package store

import (
	"context"
	"fmt"
	"time"
)

type SecurityExportStatus struct {
	Pending int64 `json:"pending"`
	Dead int64 `json:"dead"`
	Delivered int64 `json:"delivered"`
	OldestPendingAt *time.Time `json:"oldest_pending_at,omitempty"`
	LastDeliveredAt *time.Time `json:"last_delivered_at,omitempty"`
}

func (s *Store) SecurityExportStatus(ctx context.Context)(SecurityExportStatus,error){
	var out SecurityExportStatus
	err:=s.DB.QueryRow(ctx,`
		SELECT
		  count(*) FILTER(WHERE status='pending')::bigint,
		  count(*) FILTER(WHERE status='dead')::bigint,
		  count(*) FILTER(WHERE status='delivered')::bigint,
		  min(created_at) FILTER(WHERE status='pending'),
		  max(delivered_at) FILTER(WHERE status='delivered')
		FROM security_event_outbox
	`).Scan(&out.Pending,&out.Dead,&out.Delivered,&out.OldestPendingAt,&out.LastDeliveredAt)
	if err!=nil{return SecurityExportStatus{},err}
	return out,nil
}

func (s *Store) RequeueDeadSecurityExports(ctx context.Context,limit int)(int64,error){
	if limit<=0||limit>500{limit=100}
	tag,err:=s.DB.Exec(ctx,`
		WITH selected AS (
		  SELECT audit_id FROM security_event_outbox
		  WHERE status='dead'
		  ORDER BY audit_id
		  LIMIT $1
		  FOR UPDATE SKIP LOCKED
		)
		UPDATE security_event_outbox o
		SET status='pending',
		    attempts=0,
		    next_attempt_at=now(),
		    locked_until=NULL,
		    last_error=NULL,
		    updated_at=now()
		FROM selected s
		WHERE o.audit_id=s.audit_id
	`,limit)
	if err!=nil{return 0,fmt.Errorf("requeue security export: %w",err)}
	return tag.RowsAffected(),nil
}
