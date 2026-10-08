package loginthrottle

import (
	"context"
	"log/slog"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type Cleaner struct{ store *store.Store; logger *slog.Logger }

func New(s *store.Store,l *slog.Logger)*Cleaner{return &Cleaner{store:s,logger:l}}

func (c *Cleaner) Run(ctx context.Context){
	t:=time.NewTicker(30*time.Minute);defer t.Stop()
	for{
		select{
		case <-ctx.Done():return
		case <-t.C:
			cctx,cancel:=context.WithTimeout(ctx,5*time.Second)
			_,err:=c.store.DB.Exec(cctx,`
				DELETE FROM admin_login_throttle
				WHERE updated_at < now()-interval '24 hours'
				  AND (blocked_until IS NULL OR blocked_until < now())
			`)
			if err==nil{
				_,err=c.store.CleanupClientLeaseRate(cctx)
			}
			cancel()
			if err!=nil{c.logger.Warn("rate-limit cleanup failed","error",err)}
		}
	}
}
