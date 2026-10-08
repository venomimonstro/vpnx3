package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type SecurityExportEvent struct {
	AuditID int64 `json:"audit_id"`
	ActorType string `json:"actor_type"`
	ActorID *string `json:"actor_id,omitempty"`
	Action string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID *string `json:"resource_id,omitempty"`
	RequestID *string `json:"request_id,omitempty"`
	SourceIP *string `json:"source_ip,omitempty"`
	BeforeState json.RawMessage `json:"before_state,omitempty"`
	AfterState json.RawMessage `json:"after_state,omitempty"`
	Result string `json:"result"`
	CreatedAt time.Time `json:"created_at"`
	PrevHash []byte `json:"-"`
	EntryHash []byte `json:"-"`
	Attempts int `json:"-"`
}

func (s *Store) ClaimSecurityExportEvents(ctx context.Context,limit int,now time.Time)([]SecurityExportEvent,error){
	if limit<=0||limit>100{limit=25}
	tx,err:=s.DB.Begin(ctx);if err!=nil{return nil,err};defer tx.Rollback(ctx)
	rows,err:=tx.Query(ctx,`
		WITH picked AS (
		  SELECT audit_id
		  FROM security_event_outbox
		  WHERE status='pending'
		    AND next_attempt_at <= $1
		    AND COALESCE(locked_until,'-infinity'::timestamptz) < $1
		  ORDER BY audit_id
		  FOR UPDATE SKIP LOCKED
		  LIMIT $2
		),
		claimed AS (
		  UPDATE security_event_outbox o
		  SET locked_until=$1+interval '45 seconds',
		      attempts=o.attempts+1,
		      updated_at=$1
		  FROM picked p
		  WHERE o.audit_id=p.audit_id
		  RETURNING o.audit_id,o.attempts
		)
		SELECT a.id,a.actor_type,a.actor_id,a.action,a.resource_type,a.resource_id,
		       a.request_id,a.source_ip,a.before_state,a.after_state,a.result,a.created_at,
		       a.prev_hash,a.entry_hash,c.attempts
		FROM claimed c
		JOIN audit_log a ON a.id=c.audit_id
		ORDER BY a.id
	`,now,limit)
	if err!=nil{return nil,fmt.Errorf("claim security outbox: %w",err)}
	defer rows.Close()
	out:=make([]SecurityExportEvent,0)
	for rows.Next(){
		var e SecurityExportEvent
		var ip *net.IP
		if err:=rows.Scan(
			&e.AuditID,&e.ActorType,&e.ActorID,&e.Action,&e.ResourceType,&e.ResourceID,
			&e.RequestID,&ip,&e.BeforeState,&e.AfterState,&e.Result,&e.CreatedAt,
			&e.PrevHash,&e.EntryHash,&e.Attempts,
		);err!=nil{return nil,err}
		if ip!=nil{v:=ip.String();e.SourceIP=&v}
		out=append(out,e)
	}
	if err:=rows.Err();err!=nil{return nil,err}
	if err:=tx.Commit(ctx);err!=nil{return nil,err}
	return out,nil
}

func (s *Store) MarkSecurityExportDelivered(ctx context.Context,auditID int64,now time.Time)error{
	_,err:=s.DB.Exec(ctx,`
		UPDATE security_event_outbox
		SET status='delivered',delivered_at=$2,locked_until=NULL,last_error=NULL,updated_at=$2
		WHERE audit_id=$1
	`,auditID,now)
	return err
}

func (s *Store) MarkSecurityExportFailed(ctx context.Context,auditID int64,next time.Time,message string)error{
	if len(message)>1000{message=message[:1000]}
	_,err:=s.DB.Exec(ctx,`
		UPDATE security_event_outbox
		SET locked_until=NULL,next_attempt_at=$2,last_error=$3,updated_at=now()
		WHERE audit_id=$1 AND status='pending'
	`,auditID,next,message)
	return err
}

type SecurityExportHealth struct {
	Pending int64
	OldestPendingAt *time.Time
	MaxAttempts int
}

func (s *Store) SecurityExportHealth(ctx context.Context)(SecurityExportHealth,error){
	var h SecurityExportHealth
	err:=s.DB.QueryRow(ctx,`
		SELECT count(*)::bigint,min(created_at),COALESCE(max(attempts),0)::int
		FROM security_event_outbox
		WHERE status='pending'
	`).Scan(&h.Pending,&h.OldestPendingAt,&h.MaxAttempts)
	return h,err
}
