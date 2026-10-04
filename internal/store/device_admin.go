package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) RevokeUserDevice(ctx context.Context,userID,deviceID string) error {
	tag,err:=s.DB.Exec(ctx,`
		UPDATE devices
		SET status='revoked',revoked_at=now(),last_seen_at=COALESCE(last_seen_at,now())
		WHERE id=$1 AND user_id=$2 AND status='active'
	`,deviceID,userID)
	if err!=nil{return fmt.Errorf("revoke device: %w",err)}
	if tag.RowsAffected()!=1{
		var exists bool
		err:=s.DB.QueryRow(ctx,"SELECT true FROM devices WHERE id=$1 AND user_id=$2",deviceID,userID).Scan(&exists)
		if errors.Is(err,pgx.ErrNoRows){return ErrNotFound}
		if err!=nil{return err}
		return fmt.Errorf("device is not active")
	}
	return nil
}

func (s *Store) ReactivateUserDevice(ctx context.Context,userID,deviceID string) error {
	tx,err:=s.DB.Begin(ctx)
	if err!=nil{return err}
	defer tx.Rollback(ctx)

	var userStatus string
	if err:=tx.QueryRow(ctx,"SELECT status FROM users WHERE id=$1 FOR UPDATE",userID).Scan(&userStatus);err!=nil{
		if errors.Is(err,pgx.ErrNoRows){return ErrNotFound}
		return err
	}
	if userStatus!="active"{return fmt.Errorf("user is not active")}

	var deviceStatus string
	if err:=tx.QueryRow(ctx,"SELECT status FROM devices WHERE id=$1 AND user_id=$2 FOR UPDATE",deviceID,userID).Scan(&deviceStatus);err!=nil{
		if errors.Is(err,pgx.ErrNoRows){return ErrNotFound}
		return err
	}
	if deviceStatus!="revoked"{return fmt.Errorf("device is not revoked")}

	now:=time.Now().UTC()
	deviceLimit:=1
	var planLimit int
	err=tx.QueryRow(ctx,`
		SELECT p.device_limit
		FROM subscriptions s
		JOIN plans p ON p.id=s.plan_id
		WHERE s.user_id=$1
		  AND s.status IN ('active','grace')
		  AND COALESCE(s.grace_until,s.expires_at)>$2
		ORDER BY COALESCE(s.grace_until,s.expires_at) DESC
		LIMIT 1
	`,userID,now).Scan(&planLimit)
	if err==nil{
		deviceLimit=planLimit
	}else if !errors.Is(err,pgx.ErrNoRows){
		return err
	}

	var active int
	if err:=tx.QueryRow(ctx,"SELECT count(*)::int FROM devices WHERE user_id=$1 AND status='active'",userID).Scan(&active);err!=nil{
		return err
	}
	if active>=deviceLimit{return fmt.Errorf("device limit reached")}

	tag,err:=tx.Exec(ctx,`
		UPDATE devices
		SET status='active',revoked_at=NULL
		WHERE id=$1 AND user_id=$2 AND status='revoked'
	`,deviceID,userID)
	if err!=nil{return fmt.Errorf("reactivate device: %w",err)}
	if tag.RowsAffected()!=1{return ErrNotFound}
	return tx.Commit(ctx)
}
