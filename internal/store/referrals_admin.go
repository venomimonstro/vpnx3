package store

import (
	"context"
	"fmt"
	"time"
)

type ReferralAdminSummary struct {
	CodesActive int64 `json:"codes_active"`
	Claims30d int64 `json:"claims_30d"`
	Qualified30d int64 `json:"qualified_30d"`
	Reversed30d int64 `json:"reversed_30d"`
	GrantedRewards30d int64 `json:"granted_rewards_30d"`
	GrantedDays30d int64 `json:"granted_days_30d"`
	UniqueReferrers30d int64 `json:"unique_referrers_30d"`
	QualificationRate float64 `json:"qualification_rate"`
	ReversalRate float64 `json:"reversal_rate"`
}

type ReferralAdminRow struct {
	ID string `json:"id"`
	Code string `json:"code"`
	ReferrerUserID string `json:"referrer_user_id"`
	ReferredUserID string `json:"referred_user_id"`
	Status string `json:"status"`
	RewardDays int `json:"reward_days"`
	ClaimedAt time.Time `json:"claimed_at"`
	QualifiedAt *time.Time `json:"qualified_at,omitempty"`
	ReversedAt *time.Time `json:"reversed_at,omitempty"`
}

func (s *Store) ReferralAdminAnalytics(ctx context.Context,limit int)(ReferralAdminSummary,[]ReferralAdminRow,error){
	if limit<=0||limit>500{limit=100}
	var summary ReferralAdminSummary
	err:=s.DB.QueryRow(ctx,`
		SELECT
		  (SELECT count(*)::bigint FROM referral_codes WHERE status='active'),
		  count(*) FILTER (WHERE rr.claimed_at>=now()-interval '30 days')::bigint,
		  count(*) FILTER (WHERE rr.qualified_at>=now()-interval '30 days')::bigint,
		  count(*) FILTER (WHERE rr.reversed_at>=now()-interval '30 days')::bigint,
		  (SELECT count(*)::bigint FROM referral_rewards rw WHERE rw.status='granted' AND rw.granted_at>=now()-interval '30 days'),
		  (SELECT COALESCE(sum(reward_days),0)::bigint FROM referral_rewards rw WHERE rw.status='granted' AND rw.granted_at>=now()-interval '30 days'),
		  count(DISTINCT rr.referrer_user_id) FILTER (WHERE rr.claimed_at>=now()-interval '30 days')::bigint
		FROM referral_redemptions rr
	`).Scan(&summary.CodesActive,&summary.Claims30d,&summary.Qualified30d,&summary.Reversed30d,
		&summary.GrantedRewards30d,&summary.GrantedDays30d,&summary.UniqueReferrers30d)
	if err!=nil{return ReferralAdminSummary{},nil,fmt.Errorf("referral summary: %w",err)}
	if summary.Claims30d>0{summary.QualificationRate=float64(summary.Qualified30d)/float64(summary.Claims30d)*100}
	if summary.Qualified30d>0{summary.ReversalRate=float64(summary.Reversed30d)/float64(summary.Qualified30d)*100}

	rows,err:=s.DB.Query(ctx,`
		SELECT rr.id::text,rc.code,rr.referrer_user_id::text,rr.referred_user_id::text,
		       rr.status,rr.reward_days,rr.claimed_at,rr.qualified_at,rr.reversed_at
		FROM referral_redemptions rr
		JOIN referral_codes rc ON rc.id=rr.referral_code_id
		ORDER BY rr.claimed_at DESC
		LIMIT $1
	`,limit)
	if err!=nil{return ReferralAdminSummary{},nil,fmt.Errorf("referral rows: %w",err)}
	defer rows.Close()
	out:=make([]ReferralAdminRow,0)
	for rows.Next(){
		var row ReferralAdminRow
		if err:=rows.Scan(&row.ID,&row.Code,&row.ReferrerUserID,&row.ReferredUserID,&row.Status,
			&row.RewardDays,&row.ClaimedAt,&row.QualifiedAt,&row.ReversedAt);err!=nil{return ReferralAdminSummary{},nil,err}
		out=append(out,row)
	}
	return summary,out,rows.Err()
}
