package store

import (
	"context"
	"crypto/sha256"
	"net"
	"time"
)

func registrationSourceHash(ip net.IP) []byte {
	sum:=sha256.Sum256([]byte(ip.String()))
	return sum[:]
}

func (s *Store) AllowClientRegistration(ctx context.Context,ip net.IP,now time.Time,limit int)(bool,int,error){
	if ip==nil{return false,0,nil}
	if limit<=0{limit=30}
	bucket:=now.UTC().Truncate(time.Hour)
	var attempts int
	err:=s.DB.QueryRow(ctx,`
		INSERT INTO client_registration_rate(source_hash,bucket_started_at,attempts,updated_at)
		VALUES($1,$2,1,now())
		ON CONFLICT(source_hash,bucket_started_at) DO UPDATE
		SET attempts=client_registration_rate.attempts+1,
		    updated_at=now()
		RETURNING attempts
	`,registrationSourceHash(ip),bucket).Scan(&attempts)
	if err!=nil{return false,0,err}
	return attempts<=limit,attempts,nil
}

func (s *Store) CleanupClientRegistrationRate(ctx context.Context)(int64,error){
	tag,err:=s.DB.Exec(ctx,`
		DELETE FROM client_registration_rate
		WHERE updated_at < now()-interval '48 hours'
	`)
	if err!=nil{return 0,err}
	return tag.RowsAffected(),nil
}
