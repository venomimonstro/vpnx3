package store

import (
	"context"
	"fmt"
	"time"
)

type NodeStateEvent struct {
	ID             int64     `json:"id"`
	NodeID         string    `json:"node_id"`
	PreviousStatus *string   `json:"previous_status,omitempty"`
	NextStatus     string    `json:"next_status"`
	Reason         *string   `json:"reason,omitempty"`
	ActorType      string    `json:"actor_type"`
	ActorID        *string   `json:"actor_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func (s *Store) NodeStateEvents(ctx context.Context,nodeID string,limit int) ([]NodeStateEvent,error) {
	if limit<=0 || limit>200 { limit=100 }
	rows,err:=s.DB.Query(ctx,`
		SELECT id,node_id::text,previous_status::text,next_status::text,reason,actor_type,actor_id,created_at
		FROM node_state_events
		WHERE node_id=$1
		ORDER BY id DESC
		LIMIT $2
	`,nodeID,limit)
	if err!=nil { return nil,fmt.Errorf("list node state events: %w",err) }
	defer rows.Close()
	out:=make([]NodeStateEvent,0)
	for rows.Next() {
		var ev NodeStateEvent
		if err:=rows.Scan(&ev.ID,&ev.NodeID,&ev.PreviousStatus,&ev.NextStatus,&ev.Reason,&ev.ActorType,&ev.ActorID,&ev.CreatedAt); err!=nil {
			return nil,err
		}
		out=append(out,ev)
	}
	return out,rows.Err()
}

func (s *Store) DegradeStaleNodes(ctx context.Context,staleAfter time.Duration) ([]string,error) {
	cutoff:=time.Now().UTC().Add(-staleAfter)
	tx,err:=s.DB.Begin(ctx)
	if err!=nil { return nil,err }
	defer tx.Rollback(ctx)

	rows,err:=tx.Query(ctx,`
		SELECT id::text,status::text
		FROM nodes
		WHERE status IN ('active','testing')
		  AND (last_heartbeat_at IS NULL OR last_heartbeat_at < $1)
		FOR UPDATE
	`,cutoff)
	if err!=nil { return nil,err }

	type item struct{id,status string}
	var items []item
	for rows.Next() {
		var it item
		if err:=rows.Scan(&it.id,&it.status); err!=nil { rows.Close(); return nil,err }
		items=append(items,it)
	}
	rows.Close()
	if err:=rows.Err(); err!=nil { return nil,err }

	ids:=make([]string,0,len(items))
	for _,it:=range items {
		if _,err:=tx.Exec(ctx,`
			UPDATE nodes SET status='degraded',updated_at=now() WHERE id=$1
		`,it.id); err!=nil { return nil,err }
		if _,err:=tx.Exec(ctx,`
			INSERT INTO node_state_events(node_id,previous_status,next_status,reason,actor_type)
			VALUES($1,$2,'degraded','heartbeat timeout','system')
		`,it.id,it.status); err!=nil { return nil,err }
		ids=append(ids,it.id)
	}
	if err:=tx.Commit(ctx); err!=nil { return nil,err }
	return ids,nil
}
