package store

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

func (s *Store) AdminLoginBlocked(ctx context.Context,email string,ip net.IP,now time.Time) (bool,time.Time,error) {
	email=strings.ToLower(strings.TrimSpace(email))
	if email=="" || ip==nil { return false,time.Time{},nil }
	var blocked *time.Time
	err:=s.DB.QueryRow(ctx,`
		SELECT blocked_until
		FROM admin_login_throttle
		WHERE email_normalized=$1 AND source_ip=$2
	`,email,ip).Scan(&blocked)
	if err!=nil {
		if strings.Contains(err.Error(),"no rows") { return false,time.Time{},nil }
		return false,time.Time{},fmt.Errorf("login throttle lookup: %w",err)
	}
	if blocked!=nil && blocked.After(now) { return true,*blocked,nil }
	return false,time.Time{},nil
}

func (s *Store) RecordAdminLoginFailure(ctx context.Context,email string,ip net.IP,now time.Time) error {
	email=strings.ToLower(strings.TrimSpace(email))
	if email=="" || ip==nil { return nil }
	_,err:=s.DB.Exec(ctx,`
		INSERT INTO admin_login_throttle(email_normalized,source_ip,window_started_at,failed_attempts,updated_at)
		VALUES($1,$2,$3,1,$3)
		ON CONFLICT(email_normalized,source_ip) DO UPDATE
		SET
		  failed_attempts = CASE
		    WHEN admin_login_throttle.window_started_at < $3 - interval '15 minutes' THEN 1
		    ELSE admin_login_throttle.failed_attempts + 1
		  END,
		  window_started_at = CASE
		    WHEN admin_login_throttle.window_started_at < $3 - interval '15 minutes' THEN $3
		    ELSE admin_login_throttle.window_started_at
		  END,
		  blocked_until = CASE
		    WHEN (
		      CASE
		        WHEN admin_login_throttle.window_started_at < $3 - interval '15 minutes' THEN 1
		        ELSE admin_login_throttle.failed_attempts + 1
		      END
		    ) >= 8 THEN $3 + interval '15 minutes'
		    ELSE NULL
		  END,
		  updated_at=$3
	`,email,ip,now)
	return err
}

func (s *Store) ClearAdminLoginFailures(ctx context.Context,email string,ip net.IP) {
	email=strings.ToLower(strings.TrimSpace(email))
	if email=="" || ip==nil{return}
	_,_=s.DB.Exec(ctx,`
		DELETE FROM admin_login_throttle
		WHERE email_normalized=$1 AND source_ip=$2
	`,email,ip)
}
