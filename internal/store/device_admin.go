package store

import (
	"context"
	"errors"
	"fmt"

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
	tag,err:=s.DB.Exec(ctx,`
		UPDATE devices
		SET status='active',revoked_at=NULL
		WHERE id=$1 AND user_id=$2 AND status='revoked'
	`,deviceID,userID)
	if err!=nil{return fmt.Errorf("reactivate device: %w",err)}
	if tag.RowsAffected()!=1{return ErrNotFound}
	return nil
}
