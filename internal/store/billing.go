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

type Plan struct {
	ID                string    `json:"id"`
	Code              string    `json:"code"`
	Version           int       `json:"version"`
	Name              string    `json:"name"`
	PriceMinor        int64     `json:"price_minor"`
	Currency          string    `json:"currency"`
	BillingPeriodDays int       `json:"billing_period_days"`
	DeviceLimit       int       `json:"device_limit"`
	TrialDays         int       `json:"trial_days"`
	GraceDays         int       `json:"grace_days"`
	SaleEnabled       bool      `json:"sale_enabled"`
	CreatedAt         time.Time `json:"created_at"`
}

func (s *Store) ListPlans(ctx context.Context,saleOnly bool) ([]Plan,error) {
	query:=`
		SELECT id::text,code,version,name,price_minor,currency,billing_period_days,
		       device_limit,trial_days,grace_days,sale_enabled,created_at
		FROM plans
	`
	if saleOnly { query += " WHERE sale_enabled=true" }
	query += " ORDER BY code,version DESC"

	rows,err:=s.DB.Query(ctx,query)
	if err!=nil { return nil,err }
	defer rows.Close()
	out:=make([]Plan,0)
	for rows.Next() {
		var p Plan
		if err:=rows.Scan(&p.ID,&p.Code,&p.Version,&p.Name,&p.PriceMinor,&p.Currency,
			&p.BillingPeriodDays,&p.DeviceLimit,&p.TrialDays,&p.GraceDays,&p.SaleEnabled,&p.CreatedAt); err!=nil {
			return nil,err
		}
		out=append(out,p)
	}
	return out,rows.Err()
}

type CreatePlanInput struct {
	Code              string
	Name              string
	PriceMinor        int64
	Currency          string
	BillingPeriodDays int
	DeviceLimit       int
	TrialDays         int
	GraceDays         int
}

func (s *Store) CreatePlanVersion(ctx context.Context,in CreatePlanInput) (Plan,error) {
	in.Code=strings.TrimSpace(strings.ToLower(in.Code))
	in.Name=strings.TrimSpace(in.Name)
	in.Currency=strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Code=="" || len(in.Code)>64 || in.Name=="" || len(in.Name)>120 {
		return Plan{},fmt.Errorf("invalid plan identity")
	}
	if len(in.Currency)!=3 || in.PriceMinor<0 || in.BillingPeriodDays<=0 || in.DeviceLimit<=0 ||
		in.TrialDays<0 || in.GraceDays<0 {
		return Plan{},fmt.Errorf("invalid plan values")
	}

	tx,err:=s.DB.Begin(ctx)
	if err!=nil { return Plan{},err }
	defer tx.Rollback(ctx)

	// One plan code may be versioned concurrently from several admin processes.
	// Serialize only this code, not the whole plans table.
	if _,err:=tx.Exec(ctx,"SELECT pg_advisory_xact_lock(hashtext($1))",in.Code); err!=nil {
		return Plan{},fmt.Errorf("lock plan code: %w",err)
	}

	var p Plan
	err=tx.QueryRow(ctx,`
		INSERT INTO plans(code,version,name,price_minor,currency,billing_period_days,
		                  device_limit,trial_days,grace_days,sale_enabled)
		SELECT $1,COALESCE(max(version),0)+1,$2,$3,$4,$5,$6,$7,$8,true
		FROM plans WHERE code=$1
		RETURNING id::text,code,version,name,price_minor,currency,billing_period_days,
		          device_limit,trial_days,grace_days,sale_enabled,created_at
	`,in.Code,in.Name,in.PriceMinor,in.Currency,in.BillingPeriodDays,in.DeviceLimit,in.TrialDays,in.GraceDays).
		Scan(&p.ID,&p.Code,&p.Version,&p.Name,&p.PriceMinor,&p.Currency,&p.BillingPeriodDays,
			&p.DeviceLimit,&p.TrialDays,&p.GraceDays,&p.SaleEnabled,&p.CreatedAt)
	if err!=nil { return Plan{},fmt.Errorf("create plan version: %w",err) }
	if err:=tx.Commit(ctx); err!=nil { return Plan{},err }
	return p,nil
}

type PaymentEvent struct {
	Provider          string
	ProviderEventID   string
	EventType         string
	ProviderPaymentID string
	UserID            string
	PlanID            string
	Status            string
	AmountMinor       int64
	Currency          string
	OccurredAt        time.Time
	RawPayload        []byte
}

func (s *Store) ApplyPaymentEvent(ctx context.Context,event PaymentEvent) (bool,error) {
	event.Provider=strings.TrimSpace(strings.ToLower(event.Provider))
	event.ProviderEventID=strings.TrimSpace(event.ProviderEventID)
	event.ProviderPaymentID=strings.TrimSpace(event.ProviderPaymentID)
	event.Status=strings.TrimSpace(strings.ToLower(event.Status))
	event.Currency=strings.ToUpper(strings.TrimSpace(event.Currency))
	if event.Provider=="" || event.ProviderEventID=="" || event.ProviderPaymentID=="" ||
		event.UserID=="" || event.PlanID=="" || len(event.Currency)!=3 || event.AmountMinor<0 {
		return false,fmt.Errorf("invalid normalized payment event")
	}
	switch event.Status {
	case "pending","succeeded","failed","refunded","cancelled":
	default:
		return false,fmt.Errorf("invalid payment status")
	}
	if event.OccurredAt.IsZero() { event.OccurredAt=time.Now().UTC() }
	hash:=sha256.Sum256(event.RawPayload)

	tx,err:=s.DB.Begin(ctx)
	if err!=nil { return false,err }
	defer tx.Rollback(ctx)

	var existing int
	err=tx.QueryRow(ctx,`
		SELECT 1 FROM billing_events
		WHERE provider=$1 AND provider_event_id=$2
	`,event.Provider,event.ProviderEventID).Scan(&existing)
	if err==nil { return false,nil }
	if !errors.Is(err,pgx.ErrNoRows) { return false,err }

	var paymentID string
	err=tx.QueryRow(ctx,`
		INSERT INTO payments(user_id,plan_id,provider,provider_payment_id,status,
		                     amount_minor,currency,paid_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,CASE WHEN $5='succeeded' THEN $8 ELSE NULL END)
		ON CONFLICT(provider,provider_payment_id) DO UPDATE
		SET status=EXCLUDED.status,
		    paid_at=CASE WHEN EXCLUDED.status='succeeded' THEN COALESCE(payments.paid_at,EXCLUDED.paid_at) ELSE payments.paid_at END,
		    updated_at=now()
		RETURNING id::text
	`,event.UserID,event.PlanID,event.Provider,event.ProviderPaymentID,event.Status,
		event.AmountMinor,event.Currency,event.OccurredAt).Scan(&paymentID)
	if err!=nil { return false,fmt.Errorf("upsert payment: %w",err) }

	if event.Status=="succeeded" {
		var periodDays,graceDays int
		if err:=tx.QueryRow(ctx,`
			SELECT billing_period_days,grace_days FROM plans WHERE id=$1
		`,event.PlanID).Scan(&periodDays,&graceDays); err!=nil {
			return false,fmt.Errorf("load paid plan: %w",err)
		}

		var currentID string
		var currentExpires time.Time
		err:=tx.QueryRow(ctx,`
			SELECT id::text,expires_at
			FROM subscriptions
			WHERE user_id=$1 AND status IN ('active','grace')
			ORDER BY expires_at DESC LIMIT 1
			FOR UPDATE
		`,event.UserID).Scan(&currentID,&currentExpires)

		start:=event.OccurredAt
		if err==nil && currentExpires.After(start) { start=currentExpires }
		expires:=start.Add(time.Duration(periodDays)*24*time.Hour)
		grace:=expires.Add(time.Duration(graceDays)*24*time.Hour)

		if errors.Is(err,pgx.ErrNoRows) {
			_,err=tx.Exec(ctx,`
				INSERT INTO subscriptions(user_id,plan_id,status,starts_at,expires_at,grace_until,auto_renew)
				VALUES($1,$2,'active',$3,$4,$5,false)
			`,event.UserID,event.PlanID,event.OccurredAt,expires,grace)
		} else if err==nil {
			_,err=tx.Exec(ctx,`
				UPDATE subscriptions
				SET plan_id=$2,status='active',expires_at=$3,grace_until=$4,updated_at=now()
				WHERE id=$1
			`,currentID,event.PlanID,expires,grace)
		}
		if err!=nil { return false,fmt.Errorf("apply subscription entitlement: %w",err) }
	}

	if _,err:=tx.Exec(ctx,`
		INSERT INTO billing_events(provider,provider_event_id,event_type,payload_hash,payment_id)
		VALUES($1,$2,$3,$4,$5)
	`,event.Provider,event.ProviderEventID,event.EventType,hash[:],paymentID); err!=nil {
		return false,fmt.Errorf("record billing event: %w",err)
	}

	if err:=tx.Commit(ctx); err!=nil { return false,err }
	return true,nil
}
