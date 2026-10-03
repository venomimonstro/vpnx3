package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type DeviceRegistration struct {
	UserID       string
	DeviceID     string
	TrialExpires time.Time
}

func (s *Store) RegisterAnonymousDevice(ctx context.Context,platform,displayName string,publicKey []byte,trialDays int) (DeviceRegistration,error) {
	platform=strings.TrimSpace(strings.ToLower(platform))
	switch platform {
	case "android","ios","chrome","firefox","windows","macos","linux":
	default:
		return DeviceRegistration{},fmt.Errorf("unsupported platform")
	}
	displayName=strings.TrimSpace(displayName)
	if displayName=="" || len(displayName)>120 { return DeviceRegistration{},fmt.Errorf("invalid display name") }
	if len(publicKey)!=32 { return DeviceRegistration{},fmt.Errorf("invalid public key") }
	if trialDays<0 || trialDays>30 { return DeviceRegistration{},fmt.Errorf("invalid trial duration") }

	tx,err:=s.DB.Begin(ctx)
	if err!=nil { return DeviceRegistration{},err }
	defer tx.Rollback(ctx)

	var userID string
	if err:=tx.QueryRow(ctx,`
		INSERT INTO users(status) VALUES('active') RETURNING id::text
	`).Scan(&userID); err!=nil { return DeviceRegistration{},err }

	now:=time.Now().UTC()
	trialExpires:=now.Add(time.Duration(trialDays)*24*time.Hour)
	var deviceID string
	if err:=tx.QueryRow(ctx,`
		INSERT INTO devices(
			user_id,platform,display_name,status,identity_public_key,identity_algorithm,
			trial_started_at,trial_expires_at,last_seen_at
		) VALUES($1,$2,$3,'active',$4,'ed25519',$5,$6,$5)
		RETURNING id::text
	`,userID,platform,displayName,publicKey,now,trialExpires).Scan(&deviceID); err!=nil {
		return DeviceRegistration{},fmt.Errorf("create device: %w",err)
	}
	if err:=tx.Commit(ctx); err!=nil { return DeviceRegistration{},err }
	return DeviceRegistration{UserID:userID,DeviceID:deviceID,TrialExpires:trialExpires},nil
}

type DeviceAuthState struct {
	UserID string
	DeviceID string
	PublicKey []byte
	Sequence int64
	Status string
	TrialExpires *time.Time
}

func (s *Store) DeviceAuthState(ctx context.Context,deviceID string) (DeviceAuthState,error) {
	var d DeviceAuthState
	err:=s.DB.QueryRow(ctx,`
		SELECT user_id::text,id::text,identity_public_key,request_sequence,status,trial_expires_at
		FROM devices WHERE id=$1
	`,deviceID).Scan(&d.UserID,&d.DeviceID,&d.PublicKey,&d.Sequence,&d.Status,&d.TrialExpires)
	if errors.Is(err,pgx.ErrNoRows) { return DeviceAuthState{},ErrNotFound }
	if err!=nil { return DeviceAuthState{},fmt.Errorf("load device auth state: %w",err) }
	return d,nil
}

func (s *Store) AdvanceDeviceSequence(ctx context.Context,deviceID string,newSequence int64) error {
	tag,err:=s.DB.Exec(ctx,`
		UPDATE devices
		SET request_sequence=$2,last_seen_at=now()
		WHERE id=$1 AND request_sequence < $2 AND status='active'
	`,deviceID,newSequence)
	if err!=nil { return err }
	if tag.RowsAffected()!=1 { return fmt.Errorf("stale device sequence") }
	return nil
}

type Entitlement struct {
	Name string
	ExpiresAt time.Time
}

func (s *Store) DeviceEntitlement(ctx context.Context,deviceID string,now time.Time) (Entitlement,error) {
	var trialExpires *time.Time
	var userID string
	var status string
	err:=s.DB.QueryRow(ctx,`
		SELECT user_id::text,status,trial_expires_at FROM devices WHERE id=$1
	`,deviceID).Scan(&userID,&status,&trialExpires)
	if errors.Is(err,pgx.ErrNoRows) { return Entitlement{},ErrNotFound }
	if err!=nil { return Entitlement{},err }
	if status!="active" { return Entitlement{},fmt.Errorf("device inactive") }

	var planCode string
	var entitlementExpires time.Time
	err=s.DB.QueryRow(ctx,`
		SELECT p.code,COALESCE(s.grace_until,s.expires_at)
		FROM subscriptions s
		JOIN plans p ON p.id=s.plan_id
		WHERE s.user_id=$1
		  AND s.status IN ('active','grace')
		  AND COALESCE(s.grace_until,s.expires_at) > $2
		ORDER BY COALESCE(s.grace_until,s.expires_at) DESC
		LIMIT 1
	`,userID,now).Scan(&planCode,&entitlementExpires)
	if err==nil { return Entitlement{Name:planCode,ExpiresAt:entitlementExpires},nil }
	if !errors.Is(err,pgx.ErrNoRows) { return Entitlement{},err }

	if trialExpires!=nil && trialExpires.After(now) {
		return Entitlement{Name:"trial",ExpiresAt:*trialExpires},nil
	}
	return Entitlement{},fmt.Errorf("no active entitlement")
}
