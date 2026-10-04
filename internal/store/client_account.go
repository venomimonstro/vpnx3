package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ClientAccountStatus struct {
	UserID        string     `json:"user_id"`
	Entitlement   string     `json:"entitlement"`
	PlanCode      *string    `json:"plan_code,omitempty"`
	PlanName      *string    `json:"plan_name,omitempty"`
	ExpiresAt     time.Time  `json:"expires_at"`
	GraceUntil    *time.Time `json:"grace_until,omitempty"`
	DeviceLimit   int        `json:"device_limit"`
	ActiveDevices int        `json:"active_devices"`
	AutoRenew     bool       `json:"auto_renew"`
}

func (s *Store) ClientAccountStatus(ctx context.Context,deviceID string,now time.Time)(ClientAccountStatus,error){
	var userID,status string
	var trialExpires *time.Time
	if err:=s.DB.QueryRow(ctx,`
		SELECT user_id::text,status,trial_expires_at
		FROM devices WHERE id=$1
	`,deviceID).Scan(&userID,&status,&trialExpires);err!=nil{
		if errors.Is(err,pgx.ErrNoRows){return ClientAccountStatus{},ErrNotFound}
		return ClientAccountStatus{},err
	}
	if status!="active"{return ClientAccountStatus{},fmt.Errorf("device inactive")}

	out:=ClientAccountStatus{UserID:userID,DeviceLimit:1}
	var planCode,planName string
	var expires time.Time
	var grace *time.Time
	var autoRenew bool
	err:=s.DB.QueryRow(ctx,`
		SELECT p.code,p.name,s.expires_at,s.grace_until,p.device_limit,s.auto_renew
		FROM subscriptions s
		JOIN plans p ON p.id=s.plan_id
		WHERE s.user_id=$1
		  AND s.status IN ('active','grace')
		  AND COALESCE(s.grace_until,s.expires_at)>$2
		ORDER BY COALESCE(s.grace_until,s.expires_at) DESC
		LIMIT 1
	`,userID,now).Scan(&planCode,&planName,&expires,&grace,&out.DeviceLimit,&autoRenew)
	if err==nil{
		out.Entitlement=planCode
		out.PlanCode=&planCode
		out.PlanName=&planName
		out.ExpiresAt=expires
		out.GraceUntil=grace
		out.AutoRenew=autoRenew
	}else if errors.Is(err,pgx.ErrNoRows){
		if trialExpires==nil||!trialExpires.After(now){return ClientAccountStatus{},fmt.Errorf("no active entitlement")}
		out.Entitlement="trial"
		out.ExpiresAt=*trialExpires
		out.DeviceLimit=1
	}else{return ClientAccountStatus{},err}

	if err:=s.DB.QueryRow(ctx,`
		SELECT count(*)::int FROM devices WHERE user_id=$1 AND status='active'
	`,userID).Scan(&out.ActiveDevices);err!=nil{return ClientAccountStatus{},err}
	return out,nil
}

func generatePairingCode()(string,[]byte,error){
	raw:=make([]byte,8)
	if _,err:=rand.Read(raw);err!=nil{return "",nil,err}
	code:=base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	code=strings.ToUpper(code)
	sum:=sha256.Sum256([]byte(code))
	return code,sum[:],nil
}

func normalizePairingCode(code string) string {
	code=strings.ToUpper(strings.TrimSpace(code))
	code=strings.ReplaceAll(code,"-","")
	code=strings.ReplaceAll(code," ","")
	return code
}

func (s *Store) CreateDevicePairingCode(ctx context.Context,deviceID string,ttl time.Duration)(string,time.Time,error){
	if ttl<time.Minute||ttl>30*time.Minute{return "",time.Time{},fmt.Errorf("invalid pairing ttl")}

	account,err:=s.ClientAccountStatus(ctx,deviceID,time.Now().UTC())
	if err!=nil{return "",time.Time{},err}
	if account.ActiveDevices>=account.DeviceLimit{return "",time.Time{},fmt.Errorf("device limit reached")}

	tx,err:=s.DB.Begin(ctx)
	if err!=nil{return "",time.Time{},err}
	defer tx.Rollback(ctx)

	var userID,status string
	if err:=tx.QueryRow(ctx,"SELECT user_id::text,status FROM devices WHERE id=$1 FOR UPDATE",deviceID).Scan(&userID,&status);err!=nil{
		if errors.Is(err,pgx.ErrNoRows){return "",time.Time{},ErrNotFound}
		return "",time.Time{},err
	}
	if status!="active"{return "",time.Time{},fmt.Errorf("device inactive")}

	// Only one live pairing invitation per source device. Serializing on the
	// device row prevents concurrent requests from creating multiple codes.
	if _,err:=tx.Exec(ctx,`
		UPDATE device_pairing_codes
		SET used_at=now()
		WHERE created_by_device_id=$1 AND used_at IS NULL
	`,deviceID);err!=nil{return "",time.Time{},err}

	for i:=0;i<4;i++{
		code,hash,err:=generatePairingCode();if err!=nil{return "",time.Time{},err}
		expires:=time.Now().UTC().Add(ttl)
		_,err=tx.Exec(ctx,`
			INSERT INTO device_pairing_codes(user_id,created_by_device_id,code_hash,expires_at)
			VALUES($1,$2,$3,$4)
		`,userID,deviceID,hash,expires)
		if err==nil{
			if err:=tx.Commit(ctx);err!=nil{return "",time.Time{},err}
			return code,expires,nil
		}
	}
	return "",time.Time{},fmt.Errorf("unable to allocate pairing code")
}

func (s *Store) ClaimDevicePairingCode(ctx context.Context,deviceID,code string,now time.Time)(ClientAccountStatus,error){
	code=normalizePairingCode(code)
	if len(code)<10||len(code)>20{return ClientAccountStatus{},fmt.Errorf("invalid pairing code")}
	sum:=sha256.Sum256([]byte(code))

	tx,err:=s.DB.Begin(ctx);if err!=nil{return ClientAccountStatus{},err};defer tx.Rollback(ctx)

	var sourceUserID,status string
	err=tx.QueryRow(ctx,`
		SELECT user_id::text,status FROM devices WHERE id=$1 FOR UPDATE
	`,deviceID).Scan(&sourceUserID,&status)
	if errors.Is(err,pgx.ErrNoRows){return ClientAccountStatus{},ErrNotFound}
	if err!=nil{return ClientAccountStatus{},err}
	if status!="active"{return ClientAccountStatus{},fmt.Errorf("device inactive")}

	var targetUserID string
	var codeID string
	err=tx.QueryRow(ctx,`
		SELECT id::text,user_id::text
		FROM device_pairing_codes
		WHERE code_hash=$1 AND used_at IS NULL AND expires_at>$2
		FOR UPDATE
	`,sum[:],now).Scan(&codeID,&targetUserID)
	if errors.Is(err,pgx.ErrNoRows){return ClientAccountStatus{},fmt.Errorf("pairing code expired or invalid")}
	if err!=nil{return ClientAccountStatus{},err}
	if targetUserID==sourceUserID{return ClientAccountStatus{},fmt.Errorf("device already belongs to this account")}

	// Serialize all pairing claims into the same target account so concurrent
	// devices cannot both observe a free slot and exceed plan.device_limit.
	var targetStatus string
	if err:=tx.QueryRow(ctx,"SELECT status FROM users WHERE id=$1 FOR UPDATE",targetUserID).Scan(&targetStatus);err!=nil{
		return ClientAccountStatus{},err
	}
	if targetStatus!="active"{return ClientAccountStatus{},fmt.Errorf("target account inactive")}

	var sourceDevices,sourceSubscriptions,sourcePayments int
	if err:=tx.QueryRow(ctx,"SELECT count(*)::int FROM devices WHERE user_id=$1",sourceUserID).Scan(&sourceDevices);err!=nil{return ClientAccountStatus{},err}
	if err:=tx.QueryRow(ctx,"SELECT count(*)::int FROM subscriptions WHERE user_id=$1",sourceUserID).Scan(&sourceSubscriptions);err!=nil{return ClientAccountStatus{},err}
	if err:=tx.QueryRow(ctx,"SELECT count(*)::int FROM payments WHERE user_id=$1",sourceUserID).Scan(&sourcePayments);err!=nil{return ClientAccountStatus{},err}
	if sourceDevices!=1||sourceSubscriptions!=0||sourcePayments!=0{
		return ClientAccountStatus{},fmt.Errorf("source account cannot be merged")
	}

	var deviceLimit int
	err=tx.QueryRow(ctx,`
		SELECT p.device_limit
		FROM subscriptions s JOIN plans p ON p.id=s.plan_id
		WHERE s.user_id=$1 AND s.status IN ('active','grace')
		  AND COALESCE(s.grace_until,s.expires_at)>$2
		ORDER BY COALESCE(s.grace_until,s.expires_at) DESC LIMIT 1
	`,targetUserID,now).Scan(&deviceLimit)
	if errors.Is(err,pgx.ErrNoRows){deviceLimit=1}else if err!=nil{return ClientAccountStatus{},err}

	var activeCount int
	if err:=tx.QueryRow(ctx,`
		SELECT count(*)::int FROM devices WHERE user_id=$1 AND status='active'
	`,targetUserID).Scan(&activeCount);err!=nil{return ClientAccountStatus{},err}
	if activeCount>=deviceLimit{return ClientAccountStatus{},fmt.Errorf("device limit reached")}

	if _,err:=tx.Exec(ctx,`
		UPDATE devices
		SET user_id=$2,trial_started_at=NULL,trial_expires_at=NULL,last_seen_at=now()
		WHERE id=$1
	`,deviceID,targetUserID);err!=nil{return ClientAccountStatus{},err}
	if _,err:=tx.Exec(ctx,"UPDATE device_pairing_codes SET used_at=$2 WHERE id=$1",codeID,now);err!=nil{return ClientAccountStatus{},err}
	if _,err:=tx.Exec(ctx,"DELETE FROM users WHERE id=$1",sourceUserID);err!=nil{return ClientAccountStatus{},err}

	if err:=tx.Commit(ctx);err!=nil{return ClientAccountStatus{},err}
	return s.ClientAccountStatus(ctx,deviceID,now)
}
