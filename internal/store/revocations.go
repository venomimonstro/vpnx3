package store

import (
	"context"
	"fmt"
)

type RevocationSnapshotData struct {
	Version int64
	DeviceIDs []string
}

func (s *Store) RevocationSnapshot(ctx context.Context)(RevocationSnapshotData,error){
	var out RevocationSnapshotData
	if err:=s.DB.QueryRow(ctx,`
		SELECT COALESCE(max(id),0)::bigint FROM device_revocation_events
	`).Scan(&out.Version);err!=nil{
		return RevocationSnapshotData{},fmt.Errorf("revocation version: %w",err)
	}
	rows,err:=s.DB.Query(ctx,`
		SELECT id::text
		FROM devices
		WHERE status='revoked'
		ORDER BY id
	`)
	if err!=nil{return RevocationSnapshotData{},fmt.Errorf("revoked devices: %w",err)}
	defer rows.Close()
	for rows.Next(){
		var id string
		if err:=rows.Scan(&id);err!=nil{return RevocationSnapshotData{},err}
		out.DeviceIDs=append(out.DeviceIDs,id)
	}
	if err:=rows.Err();err!=nil{return RevocationSnapshotData{},err}
	return out,nil
}
