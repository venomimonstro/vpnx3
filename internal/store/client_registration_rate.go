package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"net"
	"time"
)

func registrationSourcePrefix(ip net.IP) []byte {
	if ip==nil{return nil}
	if v4:=ip.To4();v4!=nil {
		return []byte{v4[0],v4[1],v4[2]}
	}
	v6:=ip.To16()
	if v6==nil{return nil}
	out:=make([]byte,8)
	copy(out,v6[:8])
	return out
}

func registrationSourceHash(ip net.IP,secret []byte) []byte {
	prefix:=registrationSourcePrefix(ip)
	if len(prefix)==0||len(secret)==0{return nil}
	mac:=hmac.New(sha256.New,secret)
	_,_=mac.Write([]byte("vpnx3-registration-source-v1\x00"))
	_,_=mac.Write(prefix)
	return mac.Sum(nil)
}

func (s *Store) AllowClientRegistration(ctx context.Context,ip net.IP,secret []byte,now time.Time,limit int)(bool,int,error){
	sourceHash:=registrationSourceHash(ip,secret)
	if len(sourceHash)==0{return false,0,nil}
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
	`,sourceHash,bucket).Scan(&attempts)
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
