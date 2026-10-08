package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	referralRewardDays = 7
	referralClaimWindow = 14 * 24 * time.Hour
)

type ReferralStatus struct {
	Code string `json:"code,omitempty"`
	Claimed30d int64 `json:"claimed_30d"`
	Qualified30d int64 `json:"qualified_30d"`
	RewardDaysGranted int64 `json:"reward_days_granted"`
	ReferredBy *string `json:"referred_by,omitempty"`
}

type ReferralClaimResult struct {
	RewardDays int `json:"reward_days"`
	RewardApplied bool `json:"reward_applied"`
	ReferrerRewardPending bool `json:"referrer_reward_pending"`
}

func referralCode() (string,error) {
	const alphabet="ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	raw:=make([]byte,10)
	if _,err:=rand.Read(raw);err!=nil{return "",err}
	out:=make([]byte,len(raw))
	for i,b:=range raw{out[i]=alphabet[int(b)%len(alphabet)]}
	return string(out),nil
}

func (s *Store) EnsureReferralCode(ctx context.Context,deviceID string)(string,error){
	var userID,status string
	if err:=s.DB.QueryRow(ctx,`SELECT user_id::text,status FROM devices WHERE id=$1`,deviceID).Scan(&userID,&status);err!=nil{
		if errors.Is(err,pgx.ErrNoRows){return "",ErrNotFound}
		return "",err
	}
	if status!="active"{return "",fmt.Errorf("device inactive")}

	var existing string
	err:=s.DB.QueryRow(ctx,`SELECT code FROM referral_codes WHERE user_id=$1 AND status='active'`,userID).Scan(&existing)
	if err==nil{return existing,nil}
	if !errors.Is(err,pgx.ErrNoRows){return "",err}

	for attempt:=0;attempt<8;attempt++{
		code,err:=referralCode();if err!=nil{return "",err}
		err=s.DB.QueryRow(ctx,`
			INSERT INTO referral_codes(user_id,code,status)
			VALUES($1,$2,'active')
			ON CONFLICT(user_id) DO UPDATE SET status='active',updated_at=now()
			RETURNING code
		`,userID,code).Scan(&existing)
		if err==nil{return existing,nil}
		// A random code collision is extremely unlikely; retry only unique-code conflicts.
		if strings.Contains(strings.ToLower(err.Error()),"referral_codes_code"){continue}
		return "",err
	}
	return "",fmt.Errorf("could not allocate referral code")
}

func (s *Store) ClaimReferralCode(ctx context.Context,deviceID,rawCode string,now time.Time)(ReferralClaimResult,error){
	code:=strings.ToUpper(strings.TrimSpace(rawCode))
	if len(code)<6||len(code)>16{return ReferralClaimResult{},fmt.Errorf("invalid referral code")}

	tx,err:=s.DB.Begin(ctx);if err!=nil{return ReferralClaimResult{},err};defer tx.Rollback(ctx)

	var referredUser,status string
	var createdAt time.Time
	err=tx.QueryRow(ctx,`
		SELECT u.id::text,d.status,u.created_at
		FROM devices d JOIN users u ON u.id=d.user_id
		WHERE d.id=$1
		FOR UPDATE OF u,d
	`,deviceID).Scan(&referredUser,&status,&createdAt)
	if errors.Is(err,pgx.ErrNoRows){return ReferralClaimResult{},ErrNotFound}
	if err!=nil{return ReferralClaimResult{},err}
	if status!="active"{return ReferralClaimResult{},fmt.Errorf("device inactive")}
	if now.Sub(createdAt)>referralClaimWindow{return ReferralClaimResult{},fmt.Errorf("referral claim window expired")}

	var paid bool
	if err:=tx.QueryRow(ctx,`
		SELECT EXISTS(SELECT 1 FROM payments WHERE user_id=$1 AND status IN ('succeeded','refunded'))
	`,referredUser).Scan(&paid);err!=nil{return ReferralClaimResult{},err}
	if paid{return ReferralClaimResult{},fmt.Errorf("referral must be claimed before first payment")}

	var already bool
	if err:=tx.QueryRow(ctx,`
		SELECT EXISTS(SELECT 1 FROM referral_redemptions WHERE referred_user_id=$1)
	`,referredUser).Scan(&already);err!=nil{return ReferralClaimResult{},err}
	if already{return ReferralClaimResult{},fmt.Errorf("referral already claimed")}

	var codeID,referrerUser string
	err=tx.QueryRow(ctx,`
		SELECT id::text,user_id::text
		FROM referral_codes
		WHERE code=$1 AND status='active'
		FOR UPDATE
	`,code).Scan(&codeID,&referrerUser)
	if errors.Is(err,pgx.ErrNoRows){return ReferralClaimResult{},fmt.Errorf("referral code not found")}
	if err!=nil{return ReferralClaimResult{},err}
	if referrerUser==referredUser{return ReferralClaimResult{},fmt.Errorf("self referral is not allowed")}

	var redemptionID string
	if err:=tx.QueryRow(ctx,`
		INSERT INTO referral_redemptions(
		  referral_code_id,referrer_user_id,referred_user_id,status,reward_days,claimed_at
		) VALUES($1,$2,$3,'claimed',$4,$5)
		RETURNING id::text
	`,codeID,referrerUser,referredUser,referralRewardDays,now).Scan(&redemptionID);err!=nil{
		return ReferralClaimResult{},err
	}
	if _,err:=tx.Exec(ctx,`
		INSERT INTO referral_rewards(redemption_id,user_id,role,reward_days,status)
		VALUES($1,$2,'referred',$3,'pending')
	`,redemptionID,referredUser,referralRewardDays);err!=nil{return ReferralClaimResult{},err}

	applied,err:=applyPendingReferralRewardsTx(ctx,tx,referredUser,now)
	if err!=nil{return ReferralClaimResult{},err}
	if err:=tx.Commit(ctx);err!=nil{return ReferralClaimResult{},err}
	return ReferralClaimResult{
		RewardDays:referralRewardDays,RewardApplied:applied,
		ReferrerRewardPending:true,
	},nil
}

func (s *Store) ReferralStatusForDevice(ctx context.Context,deviceID string)(ReferralStatus,error){
	var userID,status string
	if err:=s.DB.QueryRow(ctx,`SELECT user_id::text,status FROM devices WHERE id=$1`,deviceID).Scan(&userID,&status);err!=nil{
		if errors.Is(err,pgx.ErrNoRows){return ReferralStatus{},ErrNotFound}
		return ReferralStatus{},err
	}
	if status!="active"{return ReferralStatus{},fmt.Errorf("device inactive")}

	var out ReferralStatus
	_ = s.DB.QueryRow(ctx,`SELECT code FROM referral_codes WHERE user_id=$1 AND status='active'`,userID).Scan(&out.Code)
	if err:=s.DB.QueryRow(ctx,`
		SELECT
		  count(*) FILTER (WHERE claimed_at>=now()-interval '30 days')::bigint,
		  count(*) FILTER (WHERE qualified_at>=now()-interval '30 days')::bigint
		FROM referral_redemptions WHERE referrer_user_id=$1
	`,userID).Scan(&out.Claimed30d,&out.Qualified30d);err!=nil{return ReferralStatus{},err}
	if err:=s.DB.QueryRow(ctx,`
		SELECT COALESCE(sum(reward_days),0)::bigint
		FROM referral_rewards WHERE user_id=$1 AND status='granted'
	`,userID).Scan(&out.RewardDaysGranted);err!=nil{return ReferralStatus{},err}
	var referredBy *string
	if err:=s.DB.QueryRow(ctx,`
		SELECT rc.code
		FROM referral_redemptions rr
		JOIN referral_codes rc ON rc.id=rr.referral_code_id
		WHERE rr.referred_user_id=$1 LIMIT 1
	`,userID).Scan(&referredBy);err!=nil&&!errors.Is(err,pgx.ErrNoRows){return ReferralStatus{},err}
	out.ReferredBy=referredBy
	return out,nil
}

func applyPendingReferralRewardsTx(ctx context.Context,tx pgx.Tx,userID string,now time.Time)(bool,error){
	rows,err:=tx.Query(ctx,`
		SELECT id::text,reward_days
		FROM referral_rewards
		WHERE user_id=$1 AND status='pending'
		ORDER BY created_at
		FOR UPDATE
	`,userID)
	if err!=nil{return false,err}
	type reward struct{id string;days int}
	pending:=make([]reward,0)
	total:=0
	for rows.Next(){
		var r reward
		if err:=rows.Scan(&r.id,&r.days);err!=nil{rows.Close();return false,err}
		pending=append(pending,r);total+=r.days
	}
	if err:=rows.Err();err!=nil{rows.Close();return false,err}
	rows.Close()
	if total==0{return false,nil}

	var subscriptionID string
	err=tx.QueryRow(ctx,`
		SELECT id::text FROM subscriptions
		WHERE user_id=$1 AND status IN ('active','grace')
		  AND COALESCE(grace_until,expires_at)>$2
		ORDER BY expires_at DESC LIMIT 1
		FOR UPDATE
	`,userID,now).Scan(&subscriptionID)
	if err==nil{
		seconds:=int64(total)*24*60*60
		if _,err:=tx.Exec(ctx,`
			UPDATE subscriptions
			SET expires_at=expires_at+($2::bigint*interval '1 second'),
			    grace_until=CASE WHEN grace_until IS NULL THEN NULL ELSE grace_until+($2::bigint*interval '1 second') END,
			    updated_at=now()
			WHERE id=$1
		`,subscriptionID,seconds);err!=nil{return false,err}
		if _,err:=tx.Exec(ctx,`
			UPDATE referral_rewards
			SET status='granted',target_type='subscription',subscription_id=$2,granted_at=$3
			WHERE user_id=$1 AND status='pending'
		`,userID,subscriptionID,now);err!=nil{return false,err}
		return true,nil
	}
	if !errors.Is(err,pgx.ErrNoRows){return false,err}

	seconds:=int64(total)*24*60*60
	tag,err:=tx.Exec(ctx,`
		UPDATE devices
		SET trial_expires_at=GREATEST(COALESCE(trial_expires_at,$2),$2)+($3::bigint*interval '1 second')
		WHERE user_id=$1 AND status='active'
	`,userID,now,seconds)
	if err!=nil{return false,err}
	if tag.RowsAffected()==0{return false,nil}
	if _,err:=tx.Exec(ctx,`
		UPDATE referral_rewards
		SET status='granted',target_type='trial',granted_at=$2
		WHERE user_id=$1 AND status='pending'
	`,userID,now);err!=nil{return false,err}
	return true,nil
}

func qualifyReferralOnPaymentTx(ctx context.Context,tx pgx.Tx,referredUser,paymentID string,now time.Time) error {
	var redemptionID,referrerUser string
	var days int
	err:=tx.QueryRow(ctx,`
		SELECT id::text,referrer_user_id::text,reward_days
		FROM referral_redemptions
		WHERE referred_user_id=$1 AND status='claimed'
		FOR UPDATE
	`,referredUser).Scan(&redemptionID,&referrerUser,&days)
	if errors.Is(err,pgx.ErrNoRows){return nil}
	if err!=nil{return err}

	if _,err:=tx.Exec(ctx,`
		UPDATE referral_redemptions
		SET status='qualified',qualifying_payment_id=$2,qualified_at=$3,updated_at=now()
		WHERE id=$1
	`,redemptionID,paymentID,now);err!=nil{return err}
	if _,err:=tx.Exec(ctx,`
		INSERT INTO referral_rewards(redemption_id,user_id,role,reward_days,status)
		VALUES($1,$2,'referrer',$3,'pending')
		ON CONFLICT(redemption_id,role) DO NOTHING
	`,redemptionID,referrerUser,days);err!=nil{return err}
	_,err=applyPendingReferralRewardsTx(ctx,tx,referrerUser,now)
	return err
}

func revokeReferralQualificationTx(ctx context.Context,tx pgx.Tx,paymentID string,now time.Time) error {
	var redemptionID string
	err:=tx.QueryRow(ctx,`
		SELECT id::text FROM referral_redemptions
		WHERE qualifying_payment_id=$1 AND status='qualified'
		FOR UPDATE
	`,paymentID).Scan(&redemptionID)
	if errors.Is(err,pgx.ErrNoRows){return nil}
	if err!=nil{return err}

	var rewardID,userID,status,targetType string
	var subscriptionID *string
	var days int
	err=tx.QueryRow(ctx,`
		SELECT id::text,user_id::text,status,COALESCE(target_type,''),subscription_id::text,reward_days
		FROM referral_rewards
		WHERE redemption_id=$1 AND role='referrer'
		FOR UPDATE
	`,redemptionID).Scan(&rewardID,&userID,&status,&targetType,&subscriptionID,&days)
	if errors.Is(err,pgx.ErrNoRows){
		_,err=tx.Exec(ctx,`UPDATE referral_redemptions SET status='reversed',reversed_at=$2,updated_at=now() WHERE id=$1`,redemptionID,now)
		return err
	}
	if err!=nil{return err}

	seconds:=int64(days)*24*60*60
	if status=="granted"{
		switch targetType{
		case "subscription":
			if subscriptionID!=nil{
				if _,err:=tx.Exec(ctx,`
					UPDATE subscriptions
					SET expires_at=GREATEST(starts_at,expires_at-($2::bigint*interval '1 second')),
					    grace_until=CASE WHEN grace_until IS NULL THEN NULL ELSE GREATEST(starts_at,grace_until-($2::bigint*interval '1 second')) END,
					    updated_at=now()
					WHERE id=$1
				`,*subscriptionID,seconds);err!=nil{return err}
			}
		case "trial":
			if _,err:=tx.Exec(ctx,`
				UPDATE devices
				SET trial_expires_at=CASE
				  WHEN trial_expires_at IS NULL THEN NULL
				  ELSE GREATEST(first_seen_at,trial_expires_at-($2::bigint*interval '1 second'))
				END
				WHERE user_id=$1
			`,userID,seconds);err!=nil{return err}
		}
	}
	if _,err:=tx.Exec(ctx,`
		UPDATE referral_rewards SET status='revoked',revoked_at=$2 WHERE id=$1 AND status<>'revoked'
	`,rewardID,now);err!=nil{return err}
	_,err=tx.Exec(ctx,`
		UPDATE referral_redemptions SET status='reversed',reversed_at=$2,updated_at=now() WHERE id=$1
	`,redemptionID,now)
	return err
}
