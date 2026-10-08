package store

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type SecurityAuditExport struct {
	AuditID int64 `json:"audit_id"`
	PrevHash string `json:"prev_hash"`
	EntryHash string `json:"entry_hash"`
	ActorType string `json:"actor_type"`
	ActorID *string `json:"actor_id,omitempty"`
	Action string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID *string `json:"resource_id,omitempty"`
	RequestID *string `json:"request_id,omitempty"`
	Result string `json:"result"`
	CreatedAt time.Time `json:"created_at"`
	Attempt int `json:"attempt"`
}

type SecurityExportHealth struct {
	Pending int64
	Dead int64
	OldestPendingAt *time.Time
	LastDeliveredAt *time.Time
}

func (s *Store) ClaimSecurityAuditExport(ctx context.Context,now time.Time)(SecurityAuditExport,error){
	var e SecurityAuditExport
	var prev,entry []byte
	err:=s.DB.QueryRow(ctx,`
		WITH candidate AS (
		  SELECT audit_id
		  FROM security_event_outbox
		  WHERE status='pending'
		    AND next_attempt_at<=$1
		    AND COALESCE(locked_until,'-infinity'::timestamptz)<$1
		  ORDER BY audit_id
		  FOR UPDATE SKIP LOCKED
		  LIMIT 1
		),
		claimed AS (
		  UPDATE security_event_outbox o
		  SET attempts=o.attempts+1,
		      locked_until=$1+interval '2 minutes',
		      updated_at=$1
		  FROM candidate c
		  WHERE o.audit_id=c.audit_id
		  RETURNING o.audit_id,o.attempts
		)
		SELECT a.id,a.prev_hash,a.entry_hash,a.actor_type,a.actor_id,a.action,
		       a.resource_type,a.resource_id,a.request_id,a.result,a.created_at,c.attempts
		FROM claimed c
		JOIN audit_log a ON a.id=c.audit_id
	`,now).Scan(
		&e.AuditID,&prev,&entry,&e.ActorType,&e.ActorID,&e.Action,
		&e.ResourceType,&e.ResourceID,&e.RequestID,&e.Result,&e.CreatedAt,&e.Attempt,
	)
	if errors.Is(err,pgx.ErrNoRows){return SecurityAuditExport{},ErrNotFound}
	if err!=nil{return SecurityAuditExport{},fmt.Errorf("claim security audit export: %w",err)}
	e.PrevHash=hex.EncodeToString(prev)
	e.EntryHash=hex.EncodeToString(entry)
	return e,nil
}

func (s *Store) MarkSecurityAuditDelivered(ctx context.Context,auditID int64,now time.Time)error{
	tag,err:=s.DB.Exec(ctx,`
		UPDATE security_event_outbox
		SET status='delivered',delivered_at=$2,locked_until=NULL,last_error=NULL,updated_at=$2
		WHERE audit_id=$1 AND status='pending'
	`,auditID,now)
	if err!=nil{return err}
	if tag.RowsAffected()!=1{return fmt.Errorf("security audit export is not pending")}
	return nil
}

func (s *Store) MarkSecurityAuditFailed(
	ctx context.Context,
	auditID int64,
	errText string,
	nextAttempt time.Time,
	dead bool,
	now time.Time,
)error{
	if len(errText)>800{errText=errText[:800]}
	status:="pending"
	if dead{status="dead"}
	tag,err:=s.DB.Exec(ctx,`
		UPDATE security_event_outbox
		SET status=$2,next_attempt_at=$3,locked_until=NULL,last_error=$4,updated_at=$5
		WHERE audit_id=$1 AND status='pending'
	`,auditID,status,nextAttempt,errText,now)
	if err!=nil{return err}
	if tag.RowsAffected()!=1{return fmt.Errorf("security audit export is not pending")}
	return nil
}

func (s *Store) SecurityExportHealth(ctx context.Context)(SecurityExportHealth,error){
	var h SecurityExportHealth
	err:=s.DB.QueryRow(ctx,`
		SELECT
		  count(*) FILTER(WHERE status='pending')::bigint,
		  count(*) FILTER(WHERE status='dead')::bigint,
		  min(created_at) FILTER(WHERE status='pending'),
		  max(delivered_at) FILTER(WHERE status='delivered')
		FROM security_event_outbox
	`).Scan(&h.Pending,&h.Dead,&h.OldestPendingAt,&h.LastDeliveredAt)
	if err!=nil{return SecurityExportHealth{},err}
	return h,nil
}
