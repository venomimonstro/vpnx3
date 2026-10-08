package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type IncidentNotification struct {
	ID int64
	IncidentID string
	EventType string
	Payload json.RawMessage
	Attempts int
}

func (s *Store) ClaimIncidentNotifications(ctx context.Context,limit int,now time.Time)([]IncidentNotification,error){
	if limit<=0||limit>100{limit=25}
	tx,err:=s.DB.Begin(ctx);if err!=nil{return nil,err};defer tx.Rollback(ctx)
	rows,err:=tx.Query(ctx,`
		WITH picked AS (
		  SELECT id FROM incident_notification_outbox
		  WHERE status='pending'
		    AND next_attempt_at<=$1
		    AND COALESCE(locked_until,'-infinity'::timestamptz)<$1
		  ORDER BY id
		  FOR UPDATE SKIP LOCKED
		  LIMIT $2
		)
		UPDATE incident_notification_outbox o
		SET locked_until=$1+interval '45 seconds',
		    attempts=o.attempts+1,
		    updated_at=$1
		FROM picked p
		WHERE o.id=p.id
		RETURNING o.id,o.incident_id::text,o.event_type,o.payload,o.attempts
	`,now,limit)
	if err!=nil{return nil,fmt.Errorf("claim incident notifications: %w",err)}
	defer rows.Close()
	out:=make([]IncidentNotification,0)
	for rows.Next(){
		var n IncidentNotification
		if err:=rows.Scan(&n.ID,&n.IncidentID,&n.EventType,&n.Payload,&n.Attempts);err!=nil{return nil,err}
		out=append(out,n)
	}
	if err:=rows.Err();err!=nil{return nil,err}
	if err:=tx.Commit(ctx);err!=nil{return nil,err}
	return out,nil
}

func (s *Store) MarkIncidentNotificationDelivered(ctx context.Context,id int64,now time.Time)error{
	_,err:=s.DB.Exec(ctx,`
		UPDATE incident_notification_outbox
		SET status='delivered',delivered_at=$2,locked_until=NULL,last_error=NULL,updated_at=$2
		WHERE id=$1
	`,id,now)
	return err
}

func (s *Store) MarkIncidentNotificationFailed(ctx context.Context,id int64,next time.Time,message string,dead bool)error{
	if len(message)>1000{message=message[:1000]}
	status:="pending"
	if dead{status="dead"}
	_,err:=s.DB.Exec(ctx,`
		UPDATE incident_notification_outbox
		SET status=$2,locked_until=NULL,next_attempt_at=$3,last_error=$4,updated_at=now()
		WHERE id=$1 AND status='pending'
	`,id,status,next,message)
	return err
}

type IncidentNotificationStatus struct {
	Pending int64 `json:"pending"`
	Dead int64 `json:"dead"`
	Delivered int64 `json:"delivered"`
	OldestPendingAt *time.Time `json:"oldest_pending_at,omitempty"`
	MaxAttempts int `json:"max_attempts"`
	LastDeliveredAt *time.Time `json:"last_delivered_at,omitempty"`
}

func (s *Store) IncidentNotificationStatus(ctx context.Context)(IncidentNotificationStatus,error){
	var out IncidentNotificationStatus
	err:=s.DB.QueryRow(ctx,`
		SELECT
		  count(*) FILTER(WHERE status='pending')::bigint,
		  count(*) FILTER(WHERE status='dead')::bigint,
		  count(*) FILTER(WHERE status='delivered')::bigint,
		  min(created_at) FILTER(WHERE status='pending'),
		  COALESCE(max(attempts) FILTER(WHERE status='pending'),0)::int,
		  max(delivered_at) FILTER(WHERE status='delivered')
		FROM incident_notification_outbox
	`).Scan(&out.Pending,&out.Dead,&out.Delivered,&out.OldestPendingAt,&out.MaxAttempts,&out.LastDeliveredAt)
	return out,err
}

func (s *Store) IncidentNotificationHealth(ctx context.Context)(pending int64,oldest *time.Time,maxAttempts int,err error){
	status,statusErr:=s.IncidentNotificationStatus(ctx)
	if statusErr!=nil{return 0,nil,0,statusErr}
	return status.Pending,status.OldestPendingAt,status.MaxAttempts,nil
}

func (s *Store) RequeueDeadIncidentNotifications(ctx context.Context,limit int)(int64,error){
	if limit<=0||limit>500{limit=100}
	tag,err:=s.DB.Exec(ctx,`
		WITH picked AS (
		  SELECT id FROM incident_notification_outbox
		  WHERE status='dead'
		  ORDER BY id
		  LIMIT $1
		  FOR UPDATE SKIP LOCKED
		)
		UPDATE incident_notification_outbox o
		SET status='pending',attempts=0,next_attempt_at=now(),
		    locked_until=NULL,last_error=NULL,updated_at=now()
		FROM picked p WHERE o.id=p.id
	`,limit)
	if err!=nil{return 0,err}
	return tag.RowsAffected(),nil
}
