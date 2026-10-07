package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type RenewalAttempt struct {
	ID string
	SubscriptionID string
	UserID string
	Plan Plan
	PaymentMethodID string
	CycleExpiresAt time.Time
	AttemptNo int
	IdempotenceKey string
}

func renewalIdempotence(subscriptionID string,cycle time.Time,attempt int) string {
	sum:=sha256.Sum256([]byte(fmt.Sprintf("vpnx3-renewal-v1\x00%s\x00%s\x00%d",
		subscriptionID,cycle.UTC().Format(time.RFC3339Nano),attempt)))
	return hex.EncodeToString(sum[:])
}

func (s *Store) ClaimDueRenewal(ctx context.Context,now time.Time)(RenewalAttempt,error){
	tx,err:=s.DB.Begin(ctx);if err!=nil{return RenewalAttempt{},err};defer tx.Rollback(ctx)

	var a RenewalAttempt
	var failureCount int
	err=tx.QueryRow(ctx,`
		SELECT s.id::text,s.user_id::text,s.expires_at,s.renewal_failures,
		       pm.provider_method_id,
		       p.id::text,p.code,p.version,p.name,p.price_minor,p.currency,p.billing_period_days,
		       p.device_limit,p.trial_days,p.grace_days,p.sale_enabled,p.created_at
		FROM subscriptions s
		JOIN plans p ON p.id=s.plan_id
		JOIN payment_methods pm ON pm.id=s.renewal_payment_method_id AND pm.status='active'
		WHERE s.auto_renew=true
		  AND s.status IN ('active','grace')
		  AND s.expires_at <= $1 + interval '12 hours'
		  AND COALESCE(s.renewal_lock_until,'-infinity'::timestamptz) < $1
		  AND s.renewal_failures < 3
		  AND NOT EXISTS (
		    SELECT 1 FROM subscription_renewal_attempts ra
		    WHERE ra.subscription_id=s.id
		      AND ra.cycle_expires_at=s.expires_at
		      AND ra.status IN ('claimed','pending','succeeded')
		  )
		ORDER BY s.expires_at
		FOR UPDATE OF s SKIP LOCKED
		LIMIT 1
	`,now).Scan(
		&a.SubscriptionID,&a.UserID,&a.CycleExpiresAt,&failureCount,&a.PaymentMethodID,
		&a.Plan.ID,&a.Plan.Code,&a.Plan.Version,&a.Plan.Name,&a.Plan.PriceMinor,&a.Plan.Currency,
		&a.Plan.BillingPeriodDays,&a.Plan.DeviceLimit,&a.Plan.TrialDays,&a.Plan.GraceDays,
		&a.Plan.SaleEnabled,&a.Plan.CreatedAt,
	)
	if errors.Is(err,pgx.ErrNoRows){return RenewalAttempt{},ErrNotFound}
	if err!=nil{return RenewalAttempt{},fmt.Errorf("claim renewal candidate: %w",err)}

	var attempts int
	var lastAttempt time.Time
	if err:=tx.QueryRow(ctx,`
		SELECT count(*)::int,COALESCE(max(updated_at),'epoch'::timestamptz)
		FROM subscription_renewal_attempts
		WHERE subscription_id=$1 AND cycle_expires_at=$2
	`,a.SubscriptionID,a.CycleExpiresAt).Scan(&attempts,&lastAttempt);err!=nil{
		return RenewalAttempt{},err
	}
	if attempts>=3{
		_,_ = tx.Exec(ctx,`UPDATE subscriptions SET auto_renew=false,renewal_lock_until=NULL,updated_at=now() WHERE id=$1`,a.SubscriptionID)
		if err:=tx.Commit(ctx);err!=nil{return RenewalAttempt{},err}
		return RenewalAttempt{},ErrNotFound
	}
	if !lastAttempt.Equal(time.Unix(0,0).UTC()) && lastAttempt.Add(30*time.Minute).After(now){
		if _,err:=tx.Exec(ctx,`
			UPDATE subscriptions SET renewal_lock_until=$2,updated_at=now() WHERE id=$1
		`,a.SubscriptionID,lastAttempt.Add(30*time.Minute));err!=nil{return RenewalAttempt{},err}
		if err:=tx.Commit(ctx);err!=nil{return RenewalAttempt{},err}
		return RenewalAttempt{},ErrNotFound
	}

	a.AttemptNo=attempts+1
	a.IdempotenceKey=renewalIdempotence(a.SubscriptionID,a.CycleExpiresAt,a.AttemptNo)
	err=tx.QueryRow(ctx,`
		INSERT INTO subscription_renewal_attempts(
		  subscription_id,cycle_expires_at,attempt_no,provider,status,idempotence_key
		) VALUES($1,$2,$3,'yookassa','claimed',$4)
		RETURNING id::text
	`,a.SubscriptionID,a.CycleExpiresAt,a.AttemptNo,a.IdempotenceKey).Scan(&a.ID)
	if err!=nil{return RenewalAttempt{},fmt.Errorf("create renewal attempt: %w",err)}

	if _,err:=tx.Exec(ctx,`
		UPDATE subscriptions
		SET renewal_lock_until=$2+interval '5 minutes',last_renewal_attempt_at=$2,updated_at=now()
		WHERE id=$1
	`,a.SubscriptionID,now);err!=nil{return RenewalAttempt{},err}

	if err:=tx.Commit(ctx);err!=nil{return RenewalAttempt{},err}
	return a,nil
}

func (s *Store) MarkRenewalFailure(ctx context.Context,attemptID,errorSummary string)error{
	if len(errorSummary)>500{errorSummary=errorSummary[:500]}
	tx,err:=s.DB.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
	var subscriptionID string
	err=tx.QueryRow(ctx,`
		UPDATE subscription_renewal_attempts
		SET status='failed',error_summary=$2,updated_at=now()
		WHERE id=$1 AND status IN ('claimed','pending')
		RETURNING subscription_id::text
	`,attemptID,errorSummary).Scan(&subscriptionID)
	if errors.Is(err,pgx.ErrNoRows){return nil}
	if err!=nil{return err}
	if _,err:=tx.Exec(ctx,`
		UPDATE subscriptions
		SET renewal_failures=renewal_failures+1,
		    renewal_lock_until=NULL,
		    auto_renew=CASE WHEN renewal_failures+1>=3 THEN false ELSE auto_renew END,
		    updated_at=now()
		WHERE id=$1
	`,subscriptionID);err!=nil{return err}
	return tx.Commit(ctx)
}

func (s *Store) SetDeviceAutoRenew(ctx context.Context,deviceID string,enabled bool)(ClientAccountStatus,error){
	tx,err:=s.DB.Begin(ctx);if err!=nil{return ClientAccountStatus{},err};defer tx.Rollback(ctx)
	var userID,status string
	if err:=tx.QueryRow(ctx,`SELECT user_id::text,status FROM devices WHERE id=$1 FOR UPDATE`,deviceID).Scan(&userID,&status);err!=nil{
		if errors.Is(err,pgx.ErrNoRows){return ClientAccountStatus{},ErrNotFound}
		return ClientAccountStatus{},err
	}
	if status!="active"{return ClientAccountStatus{},fmt.Errorf("device inactive")}

	var subscriptionID string
	if err:=tx.QueryRow(ctx,`
		SELECT id::text FROM subscriptions
		WHERE user_id=$1 AND status IN ('active','grace')
		  AND COALESCE(grace_until,expires_at)>now()
		ORDER BY expires_at DESC LIMIT 1
		FOR UPDATE
	`,userID).Scan(&subscriptionID);err!=nil{
		if errors.Is(err,pgx.ErrNoRows){return ClientAccountStatus{},fmt.Errorf("no active subscription")}
		return ClientAccountStatus{},err
	}

	if !enabled{
		if _,err:=tx.Exec(ctx,`
			UPDATE subscriptions
			SET auto_renew=false,renewal_lock_until=NULL,updated_at=now()
			WHERE id=$1
		`,subscriptionID);err!=nil{return ClientAccountStatus{},err}
	}else{
		var methodID string
		err:=tx.QueryRow(ctx,`
			SELECT id::text FROM payment_methods
			WHERE user_id=$1 AND provider='yookassa' AND status='active'
			ORDER BY COALESCE(last_used_at,created_at) DESC LIMIT 1
		`,userID).Scan(&methodID)
		if errors.Is(err,pgx.ErrNoRows){return ClientAccountStatus{},fmt.Errorf("saved payment method required")}
		if err!=nil{return ClientAccountStatus{},err}
		if _,err:=tx.Exec(ctx,`
			UPDATE subscriptions
			SET auto_renew=true,renewal_payment_method_id=$2,renewal_failures=0,renewal_lock_until=NULL,updated_at=now()
			WHERE id=$1
		`,subscriptionID,methodID);err!=nil{return ClientAccountStatus{},err}
	}
	if err:=tx.Commit(ctx);err!=nil{return ClientAccountStatus{},err}
	return s.ClientAccountStatus(ctx,deviceID,time.Now().UTC())
}


func (s *Store) RecoverStaleRenewals(ctx context.Context,now time.Time)(int64,error){
	tag,err:=s.DB.Exec(ctx,`
		WITH stale AS (
		  UPDATE subscription_renewal_attempts
		  SET status='failed',
		      error_summary=COALESCE(error_summary,'renewal confirmation timeout'),
		      updated_at=$1
		  WHERE status IN ('claimed','pending')
		    AND updated_at < $1-interval '2 hours'
		  RETURNING subscription_id
		),
		counts AS (
		  SELECT subscription_id,count(*)::int AS failures
		  FROM stale GROUP BY subscription_id
		)
		UPDATE subscriptions s
		SET renewal_failures=LEAST(3,s.renewal_failures+c.failures),
		    auto_renew=CASE WHEN s.renewal_failures+c.failures>=3 THEN false ELSE s.auto_renew END,
		    renewal_lock_until=NULL,
		    updated_at=$1
		FROM counts c
		WHERE s.id=c.subscription_id
	`,now)
	if err!=nil{return 0,err}
	return tag.RowsAffected(),nil
}
