package accountcleaner

import (
	"context"
	"log/slog"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type Cleaner struct {
	store *store.Store
	logger *slog.Logger
}

func New(s *store.Store,l *slog.Logger)*Cleaner{return &Cleaner{store:s,logger:l}}

func (c *Cleaner) Run(ctx context.Context){
	c.cleanup(ctx)
	t:=time.NewTicker(time.Hour);defer t.Stop()
	for{
		select{
		case <-ctx.Done():return
		case <-t.C:c.cleanup(ctx)
		}
	}
}

func (c *Cleaner) cleanup(parent context.Context){
	ctx,cancel:=context.WithTimeout(parent,10*time.Second);defer cancel()
	tag,err:=c.store.DB.Exec(ctx,`
		DELETE FROM device_pairing_codes
		WHERE (used_at IS NOT NULL AND used_at < now()-interval '24 hours')
		   OR (used_at IS NULL AND expires_at < now()-interval '24 hours')
	`)
	if err!=nil{
		c.logger.Warn("pairing code cleanup failed","error",err)
		return
	}
	if tag.RowsAffected()>0{
		c.logger.Info("pairing codes cleaned","count",tag.RowsAffected())
	}
	if deleted,err:=c.store.CleanupClientRegistrationRate(ctx);err!=nil{
		c.logger.Warn("registration rate cleanup failed","error",err)
	}else if deleted>0{
		c.logger.Info("registration rate buckets cleaned","count",deleted)
	}
}
