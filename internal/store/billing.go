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
	PaymentMethodID   string
	PaymentMethodSaved bool
	AutoRenewRequested bool
	RenewalAttemptID string
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

	var duplicate int
	err=tx.QueryRow(ctx,`
		SELECT 1 FROM billing_events
		WHERE provider=$1 AND provider_event_id=$2
	`,event.Provider,event.ProviderEventID).Scan(&duplicate)
	if err==nil { return false,nil }
	if !errors.Is(err,pgx.ErrNoRows) { return false,err }

	var planPrice int64
	var planCurrency string
	var periodDays,graceDays int
	if err:=tx.QueryRow(ctx,`
		SELECT price_minor,currency,billing_period_days,grace_days
		FROM plans WHERE id=$1
	`,event.PlanID).Scan(&planPrice,&planCurrency,&periodDays,&graceDays); err!=nil {
		return false,fmt.Errorf("load event plan: %w",err)
	}
	if event.AmountMinor!=planPrice || event.Currency!=planCurrency {
		return false,fmt.Errorf("payment amount or currency does not match plan version")
	}

	var paymentID,previousStatus,existingUserID,existingPlanID,existingCurrency string
	var existingAmount int64
	err=tx.QueryRow(ctx,`
		SELECT id::text,status,user_id::text,plan_id::text,amount_minor,currency
		FROM payments
		WHERE provider=$1 AND provider_payment_id=$2
		FOR UPDATE
	`,event.Provider,event.ProviderPaymentID).
		Scan(&paymentID,&previousStatus,&existingUserID,&existingPlanID,&existingAmount,&existingCurrency)

	isNew:=errors.Is(err,pgx.ErrNoRows)
	if err!=nil && !isNew { return false,fmt.Errorf("load payment: %w",err) }

	newStatus:=event.Status
	shouldGrant:=false
	if isNew {
		err=tx.QueryRow(ctx,`
			INSERT INTO payments(user_id,plan_id,provider,provider_payment_id,status,
			                     amount_minor,currency,paid_at,auto_renew_requested)
			VALUES($1,$2,$3,$4,$5,$6,$7,CASE WHEN $5='succeeded' THEN $8 ELSE NULL END,$9)
			RETURNING id::text
		`,event.UserID,event.PlanID,event.Provider,event.ProviderPaymentID,event.Status,
			event.AmountMinor,event.Currency,event.OccurredAt,event.AutoRenewRequested).Scan(&paymentID)
		if err!=nil { return false,fmt.Errorf("insert payment: %w",err) }
		shouldGrant=event.Status=="succeeded"
	} else {
		if existingUserID!=event.UserID || existingPlanID!=event.PlanID ||
			existingAmount!=event.AmountMinor || existingCurrency!=event.Currency {
			return false,fmt.Errorf("payment event conflicts with stored payment identity")
		}

		// Final successful/refunded states must never regress because an older provider event
		// was delivered after a newer event.
		if previousStatus=="refunded" {
			newStatus="refunded"
		} else if previousStatus=="succeeded" && event.Status!="refunded" {
			newStatus="succeeded"
		}
		shouldGrant=previousStatus!="succeeded" && previousStatus!="refunded" && newStatus=="succeeded"

		if _,err:=tx.Exec(ctx,`
			UPDATE payments
			SET status=$2,
			    paid_at=CASE WHEN $2='succeeded' THEN COALESCE(paid_at,$3) ELSE paid_at END,
			    updated_at=now()
			WHERE id=$1
		`,paymentID,newStatus,event.OccurredAt); err!=nil {
			return false,fmt.Errorf("update payment: %w",err)
		}
	}

	var paymentMethodID *string
	if event.Status=="succeeded" && event.PaymentMethodSaved && strings.TrimSpace(event.PaymentMethodID)!="" {
		var id string
		err:=tx.QueryRow(ctx,`
			INSERT INTO payment_methods(user_id,provider,provider_method_id,status,last_used_at)
			VALUES($1,$2,$3,'active',$4)
			ON CONFLICT(provider,provider_method_id) DO UPDATE
			SET user_id=EXCLUDED.user_id,status='active',last_used_at=EXCLUDED.last_used_at
			RETURNING id::text
		`,event.UserID,event.Provider,strings.TrimSpace(event.PaymentMethodID),event.OccurredAt).Scan(&id)
		if err!=nil{return false,fmt.Errorf("save payment method: %w",err)}
		paymentMethodID=&id
	}

	if event.RenewalAttemptID!="" {
		attemptStatus:="pending"
		if event.Status=="succeeded"{attemptStatus="succeeded"}
		if event.Status=="failed"||event.Status=="cancelled"{attemptStatus="failed"}
		var subscriptionID string
		var failureCounted bool
		err:=tx.QueryRow(ctx,`
			UPDATE subscription_renewal_attempts
			SET provider_payment_id=$2,status=$3,updated_at=now(),
			    error_summary=CASE WHEN $3='failed' THEN COALESCE(error_summary,'provider payment failed') ELSE NULL END
			WHERE id=$1
			RETURNING subscription_id::text,failure_counted
		`,event.RenewalAttemptID,event.ProviderPaymentID,attemptStatus).Scan(&subscriptionID,&failureCounted)
		if errors.Is(err,pgx.ErrNoRows){return false,fmt.Errorf("renewal attempt not found")}
		if err!=nil{return false,fmt.Errorf("update renewal attempt: %w",err)}

		if attemptStatus=="failed"&&!failureCounted{
			if _,err:=tx.Exec(ctx,`
				UPDATE subscription_renewal_attempts
				SET failure_counted=true,updated_at=now()
				WHERE id=$1 AND failure_counted=false
			`,event.RenewalAttemptID);err!=nil{return false,err}
			if _,err:=tx.Exec(ctx,`
				UPDATE subscriptions
				SET renewal_failures=LEAST(3,renewal_failures+1),
				    auto_renew=CASE WHEN renewal_failures+1>=3 THEN false ELSE auto_renew END,
				    renewal_lock_until=NULL,
				    updated_at=now()
				WHERE id=$1
			`,subscriptionID);err!=nil{return false,err}
		}
	}

	if shouldGrant {
		var subscriptionID string
		var currentExpires time.Time
		err:=tx.QueryRow(ctx,`
			SELECT id::text,expires_at
			FROM subscriptions
			WHERE user_id=$1 AND status IN ('active','grace')
			  AND COALESCE(grace_until,expires_at) > $2
			ORDER BY expires_at DESC LIMIT 1
			FOR UPDATE
		`,event.UserID,event.OccurredAt).Scan(&subscriptionID,&currentExpires)

		start:=event.OccurredAt
		if err==nil && currentExpires.After(start) { start=currentExpires }
		expires:=start.Add(time.Duration(periodDays)*24*time.Hour)
		grace:=expires.Add(time.Duration(graceDays)*24*time.Hour)

		if errors.Is(err,pgx.ErrNoRows) {
			err=tx.QueryRow(ctx,`
				INSERT INTO subscriptions(user_id,plan_id,status,starts_at,expires_at,grace_until,auto_renew,renewal_payment_method_id,renewal_failures)
				VALUES($1,$2,'active',$3,$4,$5,$6,$7,0)
				RETURNING id::text
			`,event.UserID,event.PlanID,event.OccurredAt,expires,grace,event.AutoRenewRequested && paymentMethodID!=nil,paymentMethodID).Scan(&subscriptionID)
		} else if err==nil {
			_,err=tx.Exec(ctx,`
				UPDATE subscriptions
				SET plan_id=$2,status='active',expires_at=$3,grace_until=$4,
				    auto_renew=CASE WHEN $5 AND $6::uuid IS NOT NULL THEN true ELSE auto_renew END,
				    renewal_payment_method_id=CASE WHEN $5 AND $6::uuid IS NOT NULL THEN $6::uuid ELSE renewal_payment_method_id END,
				    renewal_failures=CASE WHEN $7<>'' THEN 0 ELSE renewal_failures END,
				    renewal_lock_until=NULL,
				    updated_at=now()
				WHERE id=$1
			`,subscriptionID,event.PlanID,expires,grace,event.AutoRenewRequested,paymentMethodID,event.RenewalAttemptID)
		}
		if err!=nil { return false,fmt.Errorf("apply subscription entitlement: %w",err) }

		creditSeconds:=int64(periodDays)*24*60*60
		if _,err:=tx.Exec(ctx,`
			INSERT INTO subscription_credits(subscription_id,payment_id,credit_seconds,source,granted_at)
			VALUES($1,$2,$3,'payment',$4)
			ON CONFLICT(payment_id) DO NOTHING
		`,subscriptionID,paymentID,creditSeconds,event.OccurredAt);err!=nil{
			return false,fmt.Errorf("record subscription credit: %w",err)
		}

		if _,err:=applyPendingReferralRewardsTx(ctx,tx,event.UserID,event.OccurredAt);err!=nil{
			return false,fmt.Errorf("apply pending referral rewards: %w",err)
		}
		if err:=qualifyReferralOnPaymentTx(ctx,tx,event.UserID,paymentID,event.OccurredAt);err!=nil{
			return false,fmt.Errorf("qualify referral: %w",err)
		}
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


type RefundEvent struct {
	Provider string
	ProviderEventID string
	ProviderRefundID string
	ProviderPaymentID string
	UserID string
	PlanID string
	Status string
	AmountMinor int64
	PaymentAmountMinor int64
	Currency string
	OccurredAt time.Time
	RawPayload []byte
}

func (s *Store) ApplyRefundEvent(ctx context.Context,event RefundEvent)(bool,error){
	event.Provider=strings.ToLower(strings.TrimSpace(event.Provider))
	event.ProviderEventID=strings.TrimSpace(event.ProviderEventID)
	event.ProviderRefundID=strings.TrimSpace(event.ProviderRefundID)
	event.ProviderPaymentID=strings.TrimSpace(event.ProviderPaymentID)
	event.Status=strings.ToLower(strings.TrimSpace(event.Status))
	event.Currency=strings.ToUpper(strings.TrimSpace(event.Currency))
	if event.Provider==""||event.ProviderEventID==""||event.ProviderRefundID==""||event.ProviderPaymentID==""||
		event.UserID==""||event.PlanID==""||event.AmountMinor<=0||event.PaymentAmountMinor<=0||len(event.Currency)!=3{
		return false,fmt.Errorf("invalid normalized refund event")
	}
	if event.Status!="succeeded"{return false,fmt.Errorf("unsupported refund status")}
	if event.OccurredAt.IsZero(){event.OccurredAt=time.Now().UTC()}
	hash:=sha256.Sum256(event.RawPayload)

	tx,err:=s.DB.Begin(ctx);if err!=nil{return false,err};defer tx.Rollback(ctx)
	var duplicate int
	err=tx.QueryRow(ctx,"SELECT 1 FROM billing_events WHERE provider=$1 AND provider_event_id=$2",
		event.Provider,event.ProviderEventID).Scan(&duplicate)
	if err==nil{return false,nil}
	if !errors.Is(err,pgx.ErrNoRows){return false,err}

	var paymentID,previousStatus,userID,planID,currency string
	var paymentAmount int64
	err=tx.QueryRow(ctx,`
		SELECT id::text,status,user_id::text,plan_id::text,amount_minor,currency
		FROM payments
		WHERE provider=$1 AND provider_payment_id=$2
		FOR UPDATE
	`,event.Provider,event.ProviderPaymentID).Scan(&paymentID,&previousStatus,&userID,&planID,&paymentAmount,&currency)
	if errors.Is(err,pgx.ErrNoRows){return false,fmt.Errorf("refund payment not found")}
	if err!=nil{return false,err}
	if userID!=event.UserID||planID!=event.PlanID||paymentAmount!=event.PaymentAmountMinor||currency!=event.Currency{
		return false,fmt.Errorf("refund conflicts with stored payment identity")
	}
	if previousStatus!="succeeded"&&previousStatus!="refunded"{
		return false,fmt.Errorf("refund requires succeeded payment")
	}
	if event.AmountMinor>paymentAmount{return false,fmt.Errorf("refund exceeds payment amount")}

	var refundID string
	err=tx.QueryRow(ctx,`
		INSERT INTO refunds(payment_id,provider,provider_refund_id,status,amount_minor,currency,refunded_at)
		VALUES($1,$2,$3,'succeeded',$4,$5,$6)
		ON CONFLICT(provider,provider_refund_id) DO NOTHING
		RETURNING id::text
	`,paymentID,event.Provider,event.ProviderRefundID,event.AmountMinor,event.Currency,event.OccurredAt).Scan(&refundID)
	if errors.Is(err,pgx.ErrNoRows){return false,nil}
	if err!=nil{return false,err}

	var totalRefunded int64
	if err:=tx.QueryRow(ctx,`
		SELECT COALESCE(sum(amount_minor),0)
		FROM refunds WHERE payment_id=$1 AND status='succeeded'
	`,paymentID).Scan(&totalRefunded);err!=nil{return false,err}
	if totalRefunded>paymentAmount{return false,fmt.Errorf("cumulative refunds exceed payment amount")}

	if totalRefunded==paymentAmount && previousStatus!="refunded" {
		if _,err:=tx.Exec(ctx,"UPDATE payments SET status='refunded',updated_at=now() WHERE id=$1",paymentID);err!=nil{return false,err}

		var subscriptionID string
		var creditSeconds int64
		err:=tx.QueryRow(ctx,`
			UPDATE subscription_credits
			SET revoked_at=$2
			WHERE payment_id=$1 AND revoked_at IS NULL
			RETURNING subscription_id::text,credit_seconds
		`,paymentID,event.OccurredAt).Scan(&subscriptionID,&creditSeconds)
		if err==nil {
			if _,err:=tx.Exec(ctx,`
				UPDATE subscriptions
				SET expires_at=GREATEST(starts_at,expires_at-($2::bigint * interval '1 second')),
				    grace_until=CASE
				      WHEN grace_until IS NULL THEN NULL
				      ELSE GREATEST(starts_at,grace_until-($2::bigint * interval '1 second'))
				    END,
				    status=CASE
				      WHEN GREATEST(starts_at,expires_at-($2::bigint * interval '1 second')) <= now() THEN 'expired'
				      ELSE status
				    END,
				    updated_at=now()
				WHERE id=$1
			`,subscriptionID,creditSeconds);err!=nil{return false,err}
		} else if !errors.Is(err,pgx.ErrNoRows) { return false,err }
		if err:=revokeReferralQualificationTx(ctx,tx,paymentID,event.OccurredAt);err!=nil{
			return false,fmt.Errorf("revoke referral qualification: %w",err)
		}
	}

	if _,err:=tx.Exec(ctx,`
		INSERT INTO billing_events(provider,provider_event_id,event_type,payload_hash,payment_id)
		VALUES($1,$2,'refund.succeeded',$3,$4)
	`,event.Provider,event.ProviderEventID,hash[:],paymentID);err!=nil{return false,err}

	if err:=tx.Commit(ctx);err!=nil{return false,err}
	return true,nil
}
