package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type Node struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Role               string     `json:"role"`
	Status             string     `json:"status"`
	CountryCode        *string    `json:"country_code,omitempty"`
	Provider           *string    `json:"provider,omitempty"`
	PublicIP           *string    `json:"public_ip,omitempty"`
	AgentVersion       *string    `json:"agent_version,omitempty"`
	CapacitySessions   *int       `json:"capacity_sessions,omitempty"`
	CurrentSessions    int        `json:"current_sessions"`
	HealthScore        *float64   `json:"health_score,omitempty"`
	CircuitBreakerOpen bool       `json:"circuit_breaker_open"`
	LastHeartbeatAt    *time.Time `json:"last_heartbeat_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (s *Store) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id::text,name,role::text,status::text,country_code,provider,
		       host(public_ip),agent_version,capacity_sessions,current_sessions,
		       health_score::float8,circuit_breaker_open,last_heartbeat_at,created_at,updated_at
		FROM nodes ORDER BY created_at DESC
	`)
	if err != nil { return nil, fmt.Errorf("list nodes: %w", err) }
	defer rows.Close()

	nodes := make([]Node, 0)
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID,&n.Name,&n.Role,&n.Status,&n.CountryCode,&n.Provider,
			&n.PublicIP,&n.AgentVersion,&n.CapacitySessions,&n.CurrentSessions,
			&n.HealthScore,&n.CircuitBreakerOpen,&n.LastHeartbeatAt,&n.CreatedAt,&n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (s *Store) CreateEnrollmentToken(ctx context.Context, role, adminID string, ttl time.Duration) (plain string, expires time.Time, err error) {
	if role != "ingress" && role != "worker" && role != "probe" && role != "config_mirror" {
		return "", time.Time{}, fmt.Errorf("invalid node role")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("generate enrollment token: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(plain))
	expires = time.Now().UTC().Add(ttl)
	_, err = s.DB.Exec(ctx, `
		INSERT INTO enrollment_tokens(token_hash,intended_role,expires_at,created_by)
		VALUES($1,$2,$3,$4)
	`, hash[:], role, expires, adminID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("store enrollment token: %w", err)
	}
	return plain, expires, nil
}

var allowedNodeTransitions = map[string]map[string]bool{
	"new":         {"enrolling": true, "quarantined": true},
	"enrolling":   {"provisioning": true, "quarantined": true},
	"provisioning":{"testing": true, "quarantined": true},
	"testing":     {"draft": true, "degraded": true, "quarantined": true},
	"draft":       {"active": true, "maintenance": true, "quarantined": true, "retired": true},
	"active":      {"degraded": true, "draining": true, "maintenance": true, "quarantined": true},
	"degraded":    {"active": true, "draining": true, "maintenance": true, "quarantined": true},
	"draining":    {"maintenance": true, "retired": true, "quarantined": true, "active": true},
	"maintenance": {"testing": true, "retired": true, "quarantined": true},
	"quarantined": {"retired": true},
	"retired":     {"destroyed": true},
}

func (s *Store) TransitionNode(ctx context.Context, nodeID, target, reason, actorType, actorID string) (Node, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil { return Node{}, err }
	defer tx.Rollback(ctx)

	var current string
	if err := tx.QueryRow(ctx, "SELECT status::text FROM nodes WHERE id=$1 FOR UPDATE", nodeID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) { return Node{}, ErrNotFound }
		return Node{}, fmt.Errorf("load node state: %w", err)
	}
	if !allowedNodeTransitions[current][target] {
		return Node{}, fmt.Errorf("invalid node transition %s -> %s", current, target)
	}
	if target == "retired" {
		var sessions int
		if err := tx.QueryRow(ctx, "SELECT current_sessions FROM nodes WHERE id=$1", nodeID).Scan(&sessions); err != nil {
			return Node{}, err
		}
		if sessions > 0 {
			return Node{}, fmt.Errorf("cannot retire node with %d active sessions", sessions)
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE nodes
		SET status=$2,
		    circuit_breaker_open=false,
		    updated_at=now(),
		    published_at=CASE WHEN $2='active' AND published_at IS NULL THEN now() ELSE published_at END,
		    quarantined_at=CASE WHEN $2='quarantined' THEN now() ELSE quarantined_at END,
		    retired_at=CASE WHEN $2='retired' THEN now() ELSE retired_at END
		WHERE id=$1
	`, nodeID, target)
	if err != nil { return Node{}, fmt.Errorf("update node: %w", err) }

	if _, err := tx.Exec(ctx, `
		INSERT INTO node_state_events(node_id,previous_status,next_status,reason,actor_type,actor_id)
		VALUES($1,$2,$3,$4,$5,NULLIF($6,''))
	`, nodeID, current, target, reason, actorType, actorID); err != nil {
		return Node{}, fmt.Errorf("write state event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil { return Node{}, err }
	return s.NodeByID(ctx, nodeID)
}

func (s *Store) NodeByID(ctx context.Context, nodeID string) (Node, error) {
	var n Node
	err := s.DB.QueryRow(ctx, `
		SELECT id::text,name,role::text,status::text,country_code,provider,
		       host(public_ip),agent_version,capacity_sessions,current_sessions,
		       health_score::float8,circuit_breaker_open,last_heartbeat_at,created_at,updated_at
		FROM nodes WHERE id=$1
	`, nodeID).Scan(&n.ID,&n.Name,&n.Role,&n.Status,&n.CountryCode,&n.Provider,
		&n.PublicIP,&n.AgentVersion,&n.CapacitySessions,&n.CurrentSessions,
		&n.HealthScore,&n.CircuitBreakerOpen,&n.LastHeartbeatAt,&n.CreatedAt,&n.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) { return Node{}, ErrNotFound }
	if err != nil { return Node{}, fmt.Errorf("node by id: %w", err) }
	return n,nil
}
