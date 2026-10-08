package store

import (
	"context"
	"fmt"
	"time"
)

func (s *Store) AllowClientLease(ctx context.Context,deviceID,kind string,now time.Time,limit int)(bool,int,error){
	if kind!="access"&&kind!="proxy"{return false,0,fmt.Errorf("invalid lease rate kind")}
	if limit<=0{
		if kind=="proxy"{limit=30}else{limit=12}
	}
	bucket:=now.UTC().Truncate(time.Minute)
	var attempts int
	err:=s.DB.QueryRow(ctx,`
		INSERT INTO client_lease_rate(device_id,kind,bucket_started_at,attempts,updated_at)
		VALUES($1,$2,$3,1,now())
		ON CONFLICT(device_id,kind,bucket_started_at) DO UPDATE
		SET attempts=client_lease_rate.attempts+1,updated_at=now()
		RETURNING attempts
	`,deviceID,kind,bucket).Scan(&attempts)
	if err!=nil{return false,0,err}
	return attempts<=limit,attempts,nil
}

func (s *Store) CleanupClientLeaseRate(ctx context.Context)(int64,error){
	tag,err:=s.DB.Exec(ctx,`
		DELETE FROM client_lease_rate
		WHERE updated_at < now()-interval '24 hours'
	`)
	if err!=nil{return 0,err}
	return tag.RowsAffected(),nil
}
