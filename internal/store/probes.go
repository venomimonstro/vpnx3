package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type ProbeAuthState struct {
	PublicKey []byte
	Role      string
	Sequence  int64
	Status    string
}

func (s *Store) ProbeAuthState(ctx context.Context,nodeID string) (ProbeAuthState,error) {
	var state ProbeAuthState
	err:=s.DB.QueryRow(ctx,`
		SELECT identity_public_key,role::text,probe_sequence,status::text
		FROM nodes WHERE id=$1
	`,nodeID).Scan(&state.PublicKey,&state.Role,&state.Sequence,&state.Status)
	if errors.Is(err,pgx.ErrNoRows) { return ProbeAuthState{},ErrNotFound }
	if err!=nil { return ProbeAuthState{},fmt.Errorf("load probe auth state: %w",err) }
	return state,nil
}

type ProbeObservation struct {
	TargetNodeID string `json:"target_node_id"`
	EndpointKind string `json:"endpoint_kind"`
	Success      bool   `json:"success"`
	LatencyMS    int    `json:"latency_ms"`
}

func (s *Store) RecordProbeReport(ctx context.Context,probeNodeID string,sequence int64,results []ProbeObservation) error {
	tx,err:=s.DB.Begin(ctx)
	if err!=nil { return err }
	defer tx.Rollback(ctx)

	tag,err:=tx.Exec(ctx,`
		UPDATE nodes SET probe_sequence=$2,last_seen_at=now(),updated_at=now()
		WHERE id=$1 AND role='probe' AND probe_sequence < $2
	`,probeNodeID,sequence)
	if err!=nil { return err }
	if tag.RowsAffected()!=1 { return fmt.Errorf("stale probe sequence") }

	for _,result:=range results {
		if result.TargetNodeID=="" || result.EndpointKind=="" || result.LatencyMS<0 || result.LatencyMS>120000 {
			return fmt.Errorf("invalid probe result")
		}
		if _,err:=tx.Exec(ctx,`
			INSERT INTO probe_results(probe_node_id,target_node_id,endpoint_kind,success,latency_ms)
			VALUES($1,$2,$3,$4,$5)
		`,probeNodeID,result.TargetNodeID,result.EndpointKind,result.Success,result.LatencyMS); err!=nil {
			return fmt.Errorf("insert probe result: %w",err)
		}
	}
	return tx.Commit(ctx)
}

type RecentProbeResult struct {
	ProbeNodeID  string    `json:"probe_node_id"`
	TargetNodeID string    `json:"target_node_id"`
	EndpointKind string    `json:"endpoint_kind"`
	Success      bool      `json:"success"`
	LatencyMS    int       `json:"latency_ms"`
	ObservedAt   time.Time `json:"observed_at"`
}

func (s *Store) RecentProbeResults(ctx context.Context,limit int) ([]RecentProbeResult,error) {
	if limit<=0 || limit>1000 { limit=200 }
	rows,err:=s.DB.Query(ctx,`
		SELECT probe_node_id::text,target_node_id::text,endpoint_kind,success,latency_ms,observed_at
		FROM probe_results ORDER BY observed_at DESC LIMIT $1
	`,limit)
	if err!=nil { return nil,err }
	defer rows.Close()
	out:=make([]RecentProbeResult,0)
	for rows.Next() {
		var row RecentProbeResult
		if err:=rows.Scan(&row.ProbeNodeID,&row.TargetNodeID,&row.EndpointKind,&row.Success,&row.LatencyMS,&row.ObservedAt); err!=nil {
			return nil,err
		}
		out=append(out,row)
	}
	return out,rows.Err()
}

func (s *Store) RecalculateProbeHealth(ctx context.Context,window time.Duration) error {
	cutoff:=time.Now().UTC().Add(-window)
	if _,err:=s.DB.Exec(ctx,`
		UPDATE nodes n
		SET health_score=NULL,updated_at=now()
		WHERE n.role IN ('worker','ingress')
		  AND n.status IN ('active','degraded','testing')
		  AND NOT EXISTS (
		    SELECT 1 FROM probe_results p
		    WHERE p.target_node_id=n.id AND p.observed_at >= $1
		  )
	`,cutoff); err!=nil {
		return fmt.Errorf("clear stale probe health: %w",err)
	}
	_,err:=s.DB.Exec(ctx,`
		WITH scores AS (
			SELECT target_node_id,
			       round(avg(CASE WHEN success THEN 100.0 ELSE 0.0 END),2) AS score
			FROM probe_results
			WHERE observed_at >= $1
			GROUP BY target_node_id
		)
		UPDATE nodes n
		SET health_score=s.score,updated_at=now()
		FROM scores s
		WHERE n.id=s.target_node_id
		  AND n.role IN ('worker','ingress')
		  AND n.status IN ('active','degraded','testing')
	`,cutoff)
	if err!=nil { return fmt.Errorf("recalculate probe health: %w",err) }
	return nil
}
