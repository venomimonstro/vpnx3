package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type EnrollNodeInput struct {
	Token        string
	Name         string
	PublicKey    []byte
	Provider     string
	CountryCode  string
	AgentVersion string
	Capacity     int
	PublicIP     string
}

func (s *Store) EnrollNode(ctx context.Context, in EnrollNodeInput) (Node, error) {
	if len(in.PublicKey) != 32 {
		return Node{}, fmt.Errorf("invalid public key length")
	}
	if strings.TrimSpace(in.Name) == "" {
		return Node{}, fmt.Errorf("node name is required")
	}
	tokenHash := sha256.Sum256([]byte(in.Token))

	tx, err := s.DB.Begin(ctx)
	if err != nil { return Node{}, err }
	defer tx.Rollback(ctx)

	var role string
	var tokenID string
	err = tx.QueryRow(ctx, `
		SELECT id::text,intended_role::text
		FROM enrollment_tokens
		WHERE token_hash=$1
		  AND used_at IS NULL
		  AND expires_at > now()
		FOR UPDATE
	`, tokenHash[:]).Scan(&tokenID,&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Node{}, fmt.Errorf("invalid or expired enrollment token")
	}
	if err != nil {
		return Node{}, fmt.Errorf("load enrollment token: %w", err)
	}

	var nodeID string
	err = tx.QueryRow(ctx, `
		INSERT INTO nodes(
			name,role,status,country_code,provider,public_ip,agent_version,
			capacity_sessions,identity_public_key,identity_algorithm,
			enrolled_at,last_heartbeat_at
		)
		VALUES(
			$1,$2,'enrolling',NULLIF(upper($3),''),NULLIF($4,''),
			NULLIF($5,'')::inet,NULLIF($6,''),NULLIF($7,0),
			$8,'ed25519',now(),now()
		)
		RETURNING id::text
	`, strings.TrimSpace(in.Name), role, strings.TrimSpace(in.CountryCode),
		strings.TrimSpace(in.Provider), strings.TrimSpace(in.PublicIP),
		strings.TrimSpace(in.AgentVersion), in.Capacity, in.PublicKey).Scan(&nodeID)
	if err != nil {
		return Node{}, fmt.Errorf("create node: %w", err)
	}

	if _, err := tx.Exec(ctx, "UPDATE enrollment_tokens SET used_at=now() WHERE id=$1", tokenID); err != nil {
		return Node{}, fmt.Errorf("consume enrollment token: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO node_state_events(node_id,previous_status,next_status,reason,actor_type)
		VALUES($1,'new','enrolling','initial enrollment','system')
	`, nodeID); err != nil {
		return Node{}, fmt.Errorf("write enrollment event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Node{}, err
	}
	return s.NodeByID(ctx,nodeID)
}

type HeartbeatInput struct {
	NodeID          string
	Sequence        int64
	CurrentSessions int
	Capacity        int
	AgentVersion    string
	HealthScore     *float64
	Metadata        []byte
}

func (s *Store) NodePublicKey(ctx context.Context, nodeID string) ([]byte, int64, error) {
	var key []byte
	var seq int64
	err := s.DB.QueryRow(ctx, `
		SELECT identity_public_key,heartbeat_sequence
		FROM nodes
		WHERE id=$1 AND status NOT IN ('retired','destroyed','quarantined')
	`, nodeID).Scan(&key,&seq)
	if errors.Is(err,pgx.ErrNoRows) { return nil,0,ErrNotFound }
	return key,seq,err
}

func (s *Store) RecordHeartbeat(ctx context.Context, in HeartbeatInput) error {
	tag, err := s.DB.Exec(ctx, `
		UPDATE nodes
		SET heartbeat_sequence=$2,
		    current_sessions=GREATEST($3,0),
		    capacity_sessions=CASE WHEN $4 > 0 THEN $4 ELSE capacity_sessions END,
		    agent_version=CASE WHEN $5 <> '' THEN $5 ELSE agent_version END,
		    health_score=COALESCE($6,health_score),
		    metadata=CASE WHEN octet_length($7::bytea)>0 THEN $7::jsonb ELSE metadata END,
		    last_heartbeat_at=now(),
		    updated_at=now()
		WHERE id=$1 AND heartbeat_sequence < $2
	`, in.NodeID,in.Sequence,in.CurrentSessions,in.Capacity,in.AgentVersion,in.HealthScore,in.Metadata)
	if err != nil { return fmt.Errorf("record heartbeat: %w",err) }
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("stale heartbeat sequence")
	}
	return nil
}

func (s *Store) PromoteEnrolledNode(ctx context.Context, nodeID string) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _ = s.DB.Exec(ctx, `
		UPDATE nodes SET status='testing',updated_at=now()
		WHERE id=$1 AND status IN ('enrolling','provisioning')
	`,nodeID)
}
