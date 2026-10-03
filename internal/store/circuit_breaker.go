package store

import (
	"context"
	"fmt"
	"time"
)

type CircuitBreakerChange struct {
	NodeID string
	Opened bool
	Score  float64
}

func (s *Store) EvaluateProbeCircuitBreakers(ctx context.Context,window time.Duration) ([]CircuitBreakerChange,error) {
	cutoff:=time.Now().UTC().Add(-window)
	tx,err:=s.DB.Begin(ctx)
	if err!=nil { return nil,err }
	defer tx.Rollback(ctx)

	rows,err:=tx.Query(ctx,`
		WITH agg AS (
			SELECT target_node_id,
			       avg(CASE WHEN success THEN 100.0 ELSE 0.0 END)::float8 AS score,
			       count(*)::int AS samples,
			       count(DISTINCT probe_node_id)::int AS probes
			FROM probe_results
			WHERE observed_at >= $1
			GROUP BY target_node_id
		)
		SELECT n.id::text,n.status::text,n.circuit_breaker_open,
		       a.score,a.samples,a.probes,
		       COALESCE(n.last_heartbeat_at > now()-interval '90 seconds',false)
		FROM nodes n
		JOIN agg a ON a.target_node_id=n.id
		WHERE n.role IN ('worker','ingress')
		  AND n.status IN ('active','degraded')
		  AND a.samples >= 4
		  AND a.probes >= 2
		FOR UPDATE OF n
	`,cutoff)
	if err!=nil { return nil,fmt.Errorf("load circuit breaker candidates: %w",err) }

	type candidate struct {
		id string
		status string
		open bool
		score float64
		samples int
		probes int
		heartbeatFresh bool
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err:=rows.Scan(&c.id,&c.status,&c.open,&c.score,&c.samples,&c.probes,&c.heartbeatFresh); err!=nil {
			rows.Close()
			return nil,err
		}
		candidates=append(candidates,c)
	}
	rows.Close()
	if err:=rows.Err(); err!=nil { return nil,err }

	changes:=make([]CircuitBreakerChange,0)
	for _,c:=range candidates {
		switch {
		case c.status=="active" && !c.open && c.score<=25:
			if _,err:=tx.Exec(ctx,`
				UPDATE nodes
				SET status='degraded',circuit_breaker_open=true,updated_at=now()
				WHERE id=$1
			`,c.id); err!=nil { return nil,err }
			if _,err:=tx.Exec(ctx,`
				INSERT INTO node_state_events(node_id,previous_status,next_status,reason,actor_type)
				VALUES($1,'active','degraded','probe circuit breaker opened','system')
			`,c.id); err!=nil { return nil,err }
			changes=append(changes,CircuitBreakerChange{NodeID:c.id,Opened:true,Score:c.score})

		case c.status=="degraded" && c.open && c.score>=80 && c.heartbeatFresh:
			if _,err:=tx.Exec(ctx,`
				UPDATE nodes
				SET status='active',circuit_breaker_open=false,updated_at=now()
				WHERE id=$1
			`,c.id); err!=nil { return nil,err }
			if _,err:=tx.Exec(ctx,`
				INSERT INTO node_state_events(node_id,previous_status,next_status,reason,actor_type)
				VALUES($1,'degraded','active','probe circuit breaker recovered','system')
			`,c.id); err!=nil { return nil,err }
			changes=append(changes,CircuitBreakerChange{NodeID:c.id,Opened:false,Score:c.score})
		}
	}

	if err:=tx.Commit(ctx); err!=nil { return nil,err }
	return changes,nil
}
