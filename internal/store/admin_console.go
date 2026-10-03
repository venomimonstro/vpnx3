package store

import (
	"context"
	"fmt"
	"time"
)

type DashboardSummary struct {
	UsersTotal          int64   `json:"users_total"`
	DevicesActive       int64   `json:"devices_active"`
	SubscriptionsActive int64   `json:"subscriptions_active"`
	NodesActive         int64   `json:"nodes_active"`
	NodesDegraded       int64   `json:"nodes_degraded"`
	CurrentSessions     int64   `json:"current_sessions"`
	Revenue30dMinor     int64   `json:"revenue_30d_minor"`
	Payments30d         int64   `json:"payments_30d"`
	ProbeSuccess5m      *float64 `json:"probe_success_5m,omitempty"`
}

func (s *Store) Dashboard(ctx context.Context) (DashboardSummary,error) {
	var d DashboardSummary
	err:=s.DB.QueryRow(ctx,`
		SELECT
		  (SELECT count(*) FROM users),
		  (SELECT count(*) FROM devices WHERE status='active'),
		  (SELECT count(*) FROM subscriptions WHERE status IN ('active','grace') AND COALESCE(grace_until,expires_at)>now()),
		  (SELECT count(*) FROM nodes WHERE status='active'),
		  (SELECT count(*) FROM nodes WHERE status='degraded'),
		  (SELECT COALESCE(sum(current_sessions),0) FROM nodes WHERE role='worker'),
		  (SELECT COALESCE(sum(amount_minor),0) FROM payments WHERE status='succeeded' AND paid_at>=now()-interval '30 days'),
		  (SELECT count(*) FROM payments WHERE created_at>=now()-interval '30 days')
	`).Scan(
		&d.UsersTotal,&d.DevicesActive,&d.SubscriptionsActive,&d.NodesActive,
		&d.NodesDegraded,&d.CurrentSessions,&d.Revenue30dMinor,&d.Payments30d,
	)
	if err!=nil { return DashboardSummary{},fmt.Errorf("dashboard summary: %w",err) }

	var probe *float64
	if err:=s.DB.QueryRow(ctx,`
		SELECT avg(CASE WHEN success THEN 100.0 ELSE 0.0 END)::float8
		FROM probe_results WHERE observed_at>=now()-interval '5 minutes'
	`).Scan(&probe); err!=nil { return DashboardSummary{},fmt.Errorf("dashboard probe score: %w",err) }
	d.ProbeSuccess5m=probe
	return d,nil
}

type AdminUserRow struct {
	ID           string     `json:"id"`
	Email        *string    `json:"email,omitempty"`
	Phone        *string    `json:"phone,omitempty"`
	Status       string     `json:"status"`
	DeviceCount  int        `json:"device_count"`
	Subscription *string    `json:"subscription,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
}

func (s *Store) ListUsers(ctx context.Context,limit,offset int) ([]AdminUserRow,error) {
	if limit<=0 || limit>500 { limit=100 }
	if offset<0 { offset=0 }
	rows,err:=s.DB.Query(ctx,`
		SELECT u.id::text,u.email,u.phone,u.status,
		       (SELECT count(*)::int FROM devices d WHERE d.user_id=u.id),
		       p.code,sub.expires_at,u.created_at,u.last_seen_at
		FROM users u
		LEFT JOIN LATERAL (
		  SELECT s.plan_id,s.expires_at
		  FROM subscriptions s
		  WHERE s.user_id=u.id AND s.status IN ('active','grace')
		  ORDER BY s.expires_at DESC LIMIT 1
		) sub ON true
		LEFT JOIN plans p ON p.id=sub.plan_id
		ORDER BY u.created_at DESC
		LIMIT $1 OFFSET $2
	`,limit,offset)
	if err!=nil { return nil,fmt.Errorf("list users: %w",err) }
	defer rows.Close()
	out:=make([]AdminUserRow,0)
	for rows.Next() {
		var u AdminUserRow
		if err:=rows.Scan(&u.ID,&u.Email,&u.Phone,&u.Status,&u.DeviceCount,&u.Subscription,&u.ExpiresAt,&u.CreatedAt,&u.LastSeenAt); err!=nil {
			return nil,err
		}
		out=append(out,u)
	}
	return out,rows.Err()
}

type AdminDeviceRow struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	Platform       string     `json:"platform"`
	DisplayName    string     `json:"display_name"`
	Status         string     `json:"status"`
	ClientVersion  *string    `json:"client_version,omitempty"`
	FirstSeenAt    time.Time  `json:"first_seen_at"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
	TrialExpiresAt *time.Time `json:"trial_expires_at,omitempty"`
}

func (s *Store) UserDevices(ctx context.Context,userID string) ([]AdminDeviceRow,error) {
	rows,err:=s.DB.Query(ctx,`
		SELECT id::text,user_id::text,platform,display_name,status,client_version,
		       first_seen_at,last_seen_at,trial_expires_at
		FROM devices WHERE user_id=$1 ORDER BY first_seen_at DESC
	`,userID)
	if err!=nil { return nil,err }
	defer rows.Close()
	out:=make([]AdminDeviceRow,0)
	for rows.Next() {
		var d AdminDeviceRow
		if err:=rows.Scan(&d.ID,&d.UserID,&d.Platform,&d.DisplayName,&d.Status,&d.ClientVersion,&d.FirstSeenAt,&d.LastSeenAt,&d.TrialExpiresAt); err!=nil {
			return nil,err
		}
		out=append(out,d)
	}
	return out,rows.Err()
}

type AdminPaymentRow struct {
	ID                string     `json:"id"`
	UserID            string     `json:"user_id"`
	PlanCode          string     `json:"plan_code"`
	PlanVersion       int        `json:"plan_version"`
	Provider          string     `json:"provider"`
	ProviderPaymentID string     `json:"provider_payment_id"`
	Status            string     `json:"status"`
	AmountMinor       int64      `json:"amount_minor"`
	Currency          string     `json:"currency"`
	PaidAt            *time.Time `json:"paid_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

func (s *Store) ListPayments(ctx context.Context,limit,offset int) ([]AdminPaymentRow,error) {
	if limit<=0 || limit>500 { limit=100 }
	if offset<0 { offset=0 }
	rows,err:=s.DB.Query(ctx,`
		SELECT pay.id::text,pay.user_id::text,p.code,p.version,pay.provider,pay.provider_payment_id,
		       pay.status,pay.amount_minor,pay.currency,pay.paid_at,pay.created_at
		FROM payments pay JOIN plans p ON p.id=pay.plan_id
		ORDER BY pay.created_at DESC LIMIT $1 OFFSET $2
	`,limit,offset)
	if err!=nil { return nil,err }
	defer rows.Close()
	out:=make([]AdminPaymentRow,0)
	for rows.Next() {
		var p AdminPaymentRow
		if err:=rows.Scan(&p.ID,&p.UserID,&p.PlanCode,&p.PlanVersion,&p.Provider,&p.ProviderPaymentID,
			&p.Status,&p.AmountMinor,&p.Currency,&p.PaidAt,&p.CreatedAt); err!=nil { return nil,err }
		out=append(out,p)
	}
	return out,rows.Err()
}

type AuditRow struct {
	ID           int64     `json:"id"`
	ActorType    string    `json:"actor_type"`
	ActorID      *string   `json:"actor_id,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   *string   `json:"resource_id,omitempty"`
	RequestID    *string   `json:"request_id,omitempty"`
	SourceIP     *string   `json:"source_ip,omitempty"`
	Result       string    `json:"result"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) ListAudit(ctx context.Context,limit,offset int) ([]AuditRow,error) {
	if limit<=0 || limit>500 { limit=100 }
	if offset<0 { offset=0 }
	rows,err:=s.DB.Query(ctx,`
		SELECT id,actor_type,actor_id,action,resource_type,resource_id,request_id,
		       host(source_ip),result,created_at
		FROM audit_log ORDER BY id DESC LIMIT $1 OFFSET $2
	`,limit,offset)
	if err!=nil { return nil,err }
	defer rows.Close()
	out:=make([]AuditRow,0)
	for rows.Next() {
		var a AuditRow
		if err:=rows.Scan(&a.ID,&a.ActorType,&a.ActorID,&a.Action,&a.ResourceType,&a.ResourceID,
			&a.RequestID,&a.SourceIP,&a.Result,&a.CreatedAt); err!=nil { return nil,err }
		out=append(out,a)
	}
	return out,rows.Err()
}

type Incident struct {
	ID         string     `json:"id"`
	Severity   string     `json:"severity"`
	Status     string     `json:"status"`
	Title      string     `json:"title"`
	Summary    string     `json:"summary"`
	DetectedAt time.Time  `json:"detected_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	RootCause  *string    `json:"root_cause,omitempty"`
}

func (s *Store) ListIncidents(ctx context.Context,limit int) ([]Incident,error) {
	if limit<=0 || limit>500 { limit=100 }
	rows,err:=s.DB.Query(ctx,`
		SELECT id::text,severity,status,title,summary,detected_at,resolved_at,root_cause
		FROM incidents ORDER BY detected_at DESC LIMIT $1
	`,limit)
	if err!=nil { return nil,err }
	defer rows.Close()
	out:=make([]Incident,0)
	for rows.Next() {
		var i Incident
		if err:=rows.Scan(&i.ID,&i.Severity,&i.Status,&i.Title,&i.Summary,&i.DetectedAt,&i.ResolvedAt,&i.RootCause); err!=nil { return nil,err }
		out=append(out,i)
	}
	return out,rows.Err()
}

func (s *Store) CreateIncident(ctx context.Context,severity,title,summary string) (Incident,error) {
	var i Incident
	err:=s.DB.QueryRow(ctx,`
		INSERT INTO incidents(severity,status,title,summary)
		VALUES($1,'open',$2,$3)
		RETURNING id::text,severity,status,title,summary,detected_at,resolved_at,root_cause
	`,severity,title,summary).Scan(&i.ID,&i.Severity,&i.Status,&i.Title,&i.Summary,&i.DetectedAt,&i.ResolvedAt,&i.RootCause)
	if err!=nil { return Incident{},err }
	return i,nil
}

func (s *Store) ResolveIncident(ctx context.Context,id,rootCause string) (Incident,error) {
	var i Incident
	err:=s.DB.QueryRow(ctx,`
		UPDATE incidents SET status='resolved',resolved_at=now(),root_cause=NULLIF($2,'')
		WHERE id=$1
		RETURNING id::text,severity,status,title,summary,detected_at,resolved_at,root_cause
	`,id,rootCause).Scan(&i.ID,&i.Severity,&i.Status,&i.Title,&i.Summary,&i.DetectedAt,&i.ResolvedAt,&i.RootCause)
	if err!=nil { return Incident{},err }
	return i,nil
}
