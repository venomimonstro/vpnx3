package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type ClientDeviceRow struct {
	ID string `json:"id"`
	Platform string `json:"platform"`
	DisplayName string `json:"display_name"`
	Status string `json:"status"`
	ClientVersion *string `json:"client_version,omitempty"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	Current bool `json:"current"`
}

func (s *Store) ClientDevices(ctx context.Context,currentDeviceID string)([]ClientDeviceRow,error){
	var userID,status string
	err:=s.DB.QueryRow(ctx,`SELECT user_id::text,status FROM devices WHERE id=$1`,currentDeviceID).Scan(&userID,&status)
	if errors.Is(err,pgx.ErrNoRows){return nil,ErrNotFound}
	if err!=nil{return nil,err}
	if status!="active"{return nil,fmt.Errorf("device inactive")}

	rows,err:=s.DB.Query(ctx,`
		SELECT id::text,platform,display_name,status,client_version,first_seen_at,last_seen_at
		FROM devices
		WHERE user_id=$1
		ORDER BY CASE WHEN id=$2 THEN 0 ELSE 1 END,last_seen_at DESC NULLS LAST,first_seen_at DESC
	`,userID,currentDeviceID)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=make([]ClientDeviceRow,0)
	for rows.Next(){
		var row ClientDeviceRow
		if err:=rows.Scan(&row.ID,&row.Platform,&row.DisplayName,&row.Status,&row.ClientVersion,&row.FirstSeenAt,&row.LastSeenAt);err!=nil{
			return nil,err
		}
		row.Current=row.ID==currentDeviceID
		out=append(out,row)
	}
	return out,rows.Err()
}

func (s *Store) RevokeOwnPeerDevice(ctx context.Context,currentDeviceID,targetDeviceID string)error{
	if currentDeviceID==targetDeviceID{return fmt.Errorf("cannot revoke current device")}
	tx,err:=s.DB.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)

	var currentUser,currentStatus string
	if err:=tx.QueryRow(ctx,`
		SELECT user_id::text,status FROM devices WHERE id=$1 FOR UPDATE
	`,currentDeviceID).Scan(&currentUser,&currentStatus);err!=nil{
		if errors.Is(err,pgx.ErrNoRows){return ErrNotFound}
		return err
	}
	if currentStatus!="active"{return fmt.Errorf("current device inactive")}

	var targetUser,targetStatus string
	err=tx.QueryRow(ctx,`
		SELECT user_id::text,status FROM devices WHERE id=$1 FOR UPDATE
	`,targetDeviceID).Scan(&targetUser,&targetStatus)
	if errors.Is(err,pgx.ErrNoRows){return ErrNotFound}
	if err!=nil{return err}
	if targetUser!=currentUser{return ErrNotFound}
	if targetStatus!="active"{return fmt.Errorf("device not active")}

	if _,err:=tx.Exec(ctx,`
		UPDATE devices
		SET status='revoked',revoked_at=now(),last_seen_at=COALESCE(last_seen_at,now())
		WHERE id=$1
	`,targetDeviceID);err!=nil{return err}

	if _,err:=tx.Exec(ctx,`
		UPDATE device_pairing_codes
		SET used_at=COALESCE(used_at,now())
		WHERE created_by_device_id=$1 AND used_at IS NULL
	`,targetDeviceID);err!=nil{return err}

	return tx.Commit(ctx)
}
