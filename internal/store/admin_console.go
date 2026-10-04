package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
	RegistrationBlocks24h int64 `json:"registration_blocks_24h"`
	PaymentBlocks24h int64 `json:"payment_blocks_24h"`
	BuildQueued int64 `json:"build_queued"`
	BuildRunning int64 `json:"build_running"`
	BuildFailed int64 `json:"build_failed"`
	BuildWorkersActive int64 `json:"build_workers_active"`
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
		  (
		    SELECT COALESCE(sum(p.amount_minor),0)-
		           COALESCE((SELECT sum(r.amount_minor) FROM refunds r WHERE r.status='succeeded' AND r.refunded_at>=now()-interval '30 days'),0)
		    FROM payments p
		    WHERE p.status IN ('succeeded','refunded') AND p.paid_at>=now()-interval '30 days'
		  ),
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
	if d.RegistrationBlocks24h,err=s.SecurityCounter24h(ctx,"registration_rate_limited");err!=nil{
		return DashboardSummary{},fmt.Errorf("dashboard registration abuse counter: %w",err)
	}
	if d.PaymentBlocks24h,err=s.SecurityCounter24h(ctx,"payment_rate_limited");err!=nil{
		return DashboardSummary{},fmt.Errorf("dashboard payment abuse counter: %w",err)
	}
	if err:=s.DB.QueryRow(ctx,`SELECT
		count(*) FILTER (WHERE status='queued')::bigint,
		count(*) FILTER (WHERE status='running')::bigint,
		count(*) FILTER (WHERE status='failed')::bigint
		FROM build_jobs`).Scan(&d.BuildQueued,&d.BuildRunning,&d.BuildFailed);err!=nil{
		return DashboardSummary{},fmt.Errorf("dashboard build jobs: %w",err)
	}
	if err:=s.DB.QueryRow(ctx,`SELECT count(*)::bigint FROM nodes WHERE role='build_worker' AND status='active'`).Scan(&d.BuildWorkersActive);err!=nil{
		return DashboardSummary{},fmt.Errorf("dashboard build workers: %w",err)
	}
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

func (s *Store) ListUsers(ctx context.Context,query,status string,limit,offset int) ([]AdminUserRow,error) {
	if limit<=0 || limit>500 { limit=100 }
	if offset<0 { offset=0 }
	query=strings.TrimSpace(query)
	status=strings.TrimSpace(strings.ToLower(status))
	if status!="" && status!="active" && status!="disabled" && status!="blocked" {
		return nil,fmt.Errorf("invalid user status filter")
	}
	pattern:="%"+query+"%"
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
		WHERE ($1='' OR
		       u.id::text ILIKE $3 OR
		       COALESCE(u.email,'') ILIKE $3 OR
		       COALESCE(u.phone,'') ILIKE $3 OR
		       EXISTS (
		         SELECT 1 FROM devices d
		         WHERE d.user_id=u.id
		           AND (d.id::text ILIKE $3 OR d.display_name ILIKE $3)
		       ))
		  AND ($2='' OR u.status=$2)
		ORDER BY u.created_at DESC
		LIMIT $4 OFFSET $5
	`,query,status,pattern,limit,offset)
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

func (s *Store) ListPayments(ctx context.Context,query,status,provider string,limit,offset int) ([]AdminPaymentRow,error) {
	if limit<=0 || limit>500 { limit=100 }
	if offset<0 { offset=0 }
	query=strings.TrimSpace(query)
	status=strings.TrimSpace(strings.ToLower(status))
	provider=strings.TrimSpace(strings.ToLower(provider))
	pattern:="%"+query+"%"
	rows,err:=s.DB.Query(ctx,`
		SELECT pay.id::text,pay.user_id::text,p.code,p.version,pay.provider,pay.provider_payment_id,
		       pay.status,pay.amount_minor,pay.currency,pay.paid_at,pay.created_at
		FROM payments pay JOIN plans p ON p.id=pay.plan_id
		WHERE ($1='' OR pay.id::text ILIKE $4 OR pay.user_id::text ILIKE $4 OR
		       pay.provider_payment_id ILIKE $4 OR p.code ILIKE $4)
		  AND ($2='' OR pay.status=$2)
		  AND ($3='' OR pay.provider=$3)
		ORDER BY pay.created_at DESC LIMIT $5 OFFSET $6
	`,query,status,provider,pattern,limit,offset)
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


type AdminSubscriptionDetail struct {
	ID string `json:"id"`
	PlanCode string `json:"plan_code"`
	PlanName string `json:"plan_name"`
	PlanVersion int `json:"plan_version"`
	Status string `json:"status"`
	StartsAt time.Time `json:"starts_at"`
	ExpiresAt time.Time `json:"expires_at"`
	GraceUntil *time.Time `json:"grace_until,omitempty"`
	AutoRenew bool `json:"auto_renew"`
	DeviceLimit int `json:"device_limit"`
}

type AdminUserDetail struct {
	ID string `json:"id"`
	Email *string `json:"email,omitempty"`
	Phone *string `json:"phone,omitempty"`
	Status string `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	DeviceCount int `json:"device_count"`
	ActiveDeviceCount int `json:"active_device_count"`
	TrialExpiresAt *time.Time `json:"trial_expires_at,omitempty"`
	Subscription *AdminSubscriptionDetail `json:"subscription,omitempty"`
}

func (s *Store) AdminUserDetail(ctx context.Context,userID string)(AdminUserDetail,error){
	var u AdminUserDetail
	var subID,planCode,planName,subStatus *string
	var planVersion,deviceLimit *int
	var startsAt,expiresAt,graceUntil *time.Time
	var autoRenew *bool
	err:=s.DB.QueryRow(ctx,`
		SELECT u.id::text,u.email,u.phone,u.status,u.created_at,u.updated_at,u.last_seen_at,
		       (SELECT count(*)::int FROM devices d WHERE d.user_id=u.id),
		       (SELECT count(*)::int FROM devices d WHERE d.user_id=u.id AND d.status='active'),
		       (SELECT max(d.trial_expires_at) FROM devices d WHERE d.user_id=u.id),
		       sub.id::text,p.code,p.name,p.version,sub.status,sub.starts_at,sub.expires_at,
		       sub.grace_until,sub.auto_renew,p.device_limit
		FROM users u
		LEFT JOIN LATERAL (
		  SELECT s.*
		  FROM subscriptions s
		  WHERE s.user_id=u.id
		  ORDER BY COALESCE(s.grace_until,s.expires_at) DESC,s.created_at DESC
		  LIMIT 1
		) sub ON true
		LEFT JOIN plans p ON p.id=sub.plan_id
		WHERE u.id=$1
	`,userID).Scan(
		&u.ID,&u.Email,&u.Phone,&u.Status,&u.CreatedAt,&u.UpdatedAt,&u.LastSeenAt,
		&u.DeviceCount,&u.ActiveDeviceCount,&u.TrialExpiresAt,
		&subID,&planCode,&planName,&planVersion,&subStatus,&startsAt,&expiresAt,
		&graceUntil,&autoRenew,&deviceLimit,
	)
	if errors.Is(err,pgx.ErrNoRows){return AdminUserDetail{},ErrNotFound}
	if err!=nil{return AdminUserDetail{},err}
	if subID!=nil&&planCode!=nil&&planName!=nil&&planVersion!=nil&&subStatus!=nil&&startsAt!=nil&&expiresAt!=nil&&autoRenew!=nil&&deviceLimit!=nil{
		u.Subscription=&AdminSubscriptionDetail{
			ID:*subID,PlanCode:*planCode,PlanName:*planName,PlanVersion:*planVersion,
			Status:*subStatus,StartsAt:*startsAt,ExpiresAt:*expiresAt,GraceUntil:graceUntil,
			AutoRenew:*autoRenew,DeviceLimit:*deviceLimit,
		}
	}
	return u,nil
}

func (s *Store) UserPayments(ctx context.Context,userID string,limit int)([]AdminPaymentRow,error){
	if limit<=0||limit>100{limit=20}
	rows,err:=s.DB.Query(ctx,`
		SELECT pay.id::text,pay.user_id::text,p.code,p.version,pay.provider,pay.provider_payment_id,
		       pay.status,pay.amount_minor,pay.currency,pay.paid_at,pay.created_at
		FROM payments pay
		JOIN plans p ON p.id=pay.plan_id
		WHERE pay.user_id=$1
		ORDER BY pay.created_at DESC
		LIMIT $2
	`,userID,limit)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=make([]AdminPaymentRow,0)
	for rows.Next(){
		var p AdminPaymentRow
		if err:=rows.Scan(&p.ID,&p.UserID,&p.PlanCode,&p.PlanVersion,&p.Provider,&p.ProviderPaymentID,
			&p.Status,&p.AmountMinor,&p.Currency,&p.PaidAt,&p.CreatedAt);err!=nil{return nil,err}
		out=append(out,p)
	}
	return out,rows.Err()
}
