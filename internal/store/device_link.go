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

type DeviceLinkCode struct {
	Code string
	ExpiresAt time.Time
}

func (s *Store) CreateDeviceLinkCode(ctx context.Context,deviceID string,now time.Time)(DeviceLinkCode,error){
	tx,err:=s.DB.Begin(ctx);if err!=nil{return DeviceLinkCode{},err};defer tx.Rollback(ctx)

	var userID,status string
	err=tx.QueryRow(ctx,`
		SELECT user_id::text,status FROM devices WHERE id=$1 FOR UPDATE
	`,deviceID).Scan(&userID,&status)
	if errors.Is(err,pgx.ErrNoRows){return DeviceLinkCode{},ErrNotFound}
	if err!=nil{return DeviceLinkCode{},err}
	if status!="active"{return DeviceLinkCode{},fmt.Errorf("device inactive")}

	limit,err:=activeDeviceLimit(ctx,tx,userID,now)
	if err!=nil{return DeviceLinkCode{},err}
	if limit<=1{return DeviceLinkCode{},fmt.Errorf("plan does not allow additional devices")}

	var active int
	if err:=tx.QueryRow(ctx,`
		SELECT count(*)::int FROM devices WHERE user_id=$1 AND status='active'
	`,userID).Scan(&active);err!=nil{return DeviceLinkCode{},err}
	if active>=limit{return DeviceLinkCode{},fmt.Errorf("device limit reached")}

	raw:=make([]byte,8)
	if _,err:=rand.Read(raw);err!=nil{return DeviceLinkCode{},err}
	code:=strings.TrimRight(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw),"=")
	sum:=sha256.Sum256([]byte(code))
	expires:=now.Add(10*time.Minute)

	if _,err:=tx.Exec(ctx,`
		UPDATE device_link_codes SET used_at=$2
		WHERE user_id=$1 AND used_at IS NULL AND expires_at>$2
	`,userID,now);err!=nil{return DeviceLinkCode{},err}
	if _,err:=tx.Exec(ctx,`
		INSERT INTO device_link_codes(user_id,created_by_device_id,code_hash,expires_at)
		VALUES($1,$2,$3,$4)
	`,userID,deviceID,sum[:],expires);err!=nil{return DeviceLinkCode{},err}
	if err:=tx.Commit(ctx);err!=nil{return DeviceLinkCode{},err}
	return DeviceLinkCode{Code:code,ExpiresAt:expires},nil
}

func (s *Store) RegisterLinkedDevice(
	ctx context.Context,
	code,platform,displayName,algorithm string,
	publicKey []byte,
	now time.Time,
)(DeviceRegistration,error){
	code=strings.ToUpper(strings.TrimSpace(code))
	if len(code)<10||len(code)>20{return DeviceRegistration{},fmt.Errorf("invalid link code")}
	if err:=validateDeviceInput(platform,displayName,algorithm,publicKey);err!=nil{return DeviceRegistration{},err}
	sum:=sha256.Sum256([]byte(code))

	tx,err:=s.DB.Begin(ctx);if err!=nil{return DeviceRegistration{},err};defer tx.Rollback(ctx)
	var linkID,userID string
	var expires time.Time
	var usedAt *time.Time
	err=tx.QueryRow(ctx,`
		SELECT id::text,user_id::text,expires_at,used_at
		FROM device_link_codes WHERE code_hash=$1 FOR UPDATE
	`,sum[:]).Scan(&linkID,&userID,&expires,&usedAt)
	if errors.Is(err,pgx.ErrNoRows){return DeviceRegistration{},fmt.Errorf("invalid link code")}
	if err!=nil{return DeviceRegistration{},err}
	if usedAt!=nil||!expires.After(now){return DeviceRegistration{},fmt.Errorf("link code expired or used")}

	var userStatus string
	if err:=tx.QueryRow(ctx,"SELECT status FROM users WHERE id=$1 FOR UPDATE",userID).Scan(&userStatus);err!=nil{return DeviceRegistration{},err}
	if userStatus!="active"{return DeviceRegistration{},fmt.Errorf("user inactive")}

	limit,err:=activeDeviceLimit(ctx,tx,userID,now)
	if err!=nil{return DeviceRegistration{},err}
	if limit<=1{return DeviceRegistration{},fmt.Errorf("plan does not allow additional devices")}

	var active int
	if err:=tx.QueryRow(ctx,"SELECT count(*)::int FROM devices WHERE user_id=$1 AND status='active'",userID).Scan(&active);err!=nil{return DeviceRegistration{},err}
	if active>=limit{return DeviceRegistration{},fmt.Errorf("device limit reached")}

	var deviceID string
	err=tx.QueryRow(ctx,`
		INSERT INTO devices(
		  user_id,platform,display_name,status,identity_public_key,identity_algorithm,last_seen_at
		)
		VALUES($1,$2,$3,'active',$4,$5,$6)
		RETURNING id::text
	`,userID,strings.ToLower(strings.TrimSpace(platform)),strings.TrimSpace(displayName),publicKey,algorithm,now).Scan(&deviceID)
	if err!=nil{return DeviceRegistration{},fmt.Errorf("create linked device: %w",err)}

	if _,err:=tx.Exec(ctx,"UPDATE device_link_codes SET used_at=$2 WHERE id=$1",linkID,now);err!=nil{return DeviceRegistration{},err}
	if err:=tx.Commit(ctx);err!=nil{return DeviceRegistration{},err}

	return DeviceRegistration{UserID:userID,DeviceID:deviceID},nil
}

type deviceLimitQuerier interface{
	QueryRow(context.Context,string,...any) pgx.Row
}

func activeDeviceLimit(ctx context.Context,q deviceLimitQuerier,userID string,now time.Time)(int,error){
	var limit int
	err:=q.QueryRow(ctx,`
		SELECT p.device_limit
		FROM subscriptions s
		JOIN plans p ON p.id=s.plan_id
		WHERE s.user_id=$1
		  AND s.status IN ('active','grace')
		  AND COALESCE(s.grace_until,s.expires_at)>$2
		ORDER BY COALESCE(s.grace_until,s.expires_at) DESC
		LIMIT 1
	`,userID,now).Scan(&limit)
	if errors.Is(err,pgx.ErrNoRows){return 1,nil}
	return limit,err
}

func validateDeviceInput(platform,displayName,algorithm string,publicKey []byte)error{
	switch strings.ToLower(strings.TrimSpace(platform)){
	case "android","ios","chrome","firefox","windows","macos","linux":
	default:return fmt.Errorf("unsupported platform")
	}
	displayName=strings.TrimSpace(displayName)
	if displayName==""||len(displayName)>120{return fmt.Errorf("invalid display name")}
	if len(publicKey)==0||len(publicKey)>2048{return fmt.Errorf("invalid public key")}
	if algorithm!="ed25519"&&algorithm!="ecdsa-p256-sha256"{return fmt.Errorf("unsupported identity algorithm")}
	return nil
}
