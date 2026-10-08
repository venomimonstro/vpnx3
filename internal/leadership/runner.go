package leadership

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Runner struct{
	db *pgxpool.Pool
	logger *slog.Logger
	lockID int64
	retry time.Duration
	ping time.Duration
}

func New(db *pgxpool.Pool,logger *slog.Logger,lockID int64)*Runner{
	return &Runner{db:db,logger:logger,lockID:lockID,retry:5*time.Second,ping:10*time.Second}
}

func (r *Runner) Run(ctx context.Context,run func(context.Context)){
	for{
		if ctx.Err()!=nil{return}
		conn,err:=r.db.Acquire(ctx)
		if err!=nil{
			r.logger.Warn("leadership connection acquire failed","error",err)
			if !sleep(ctx,r.retry){return}
			continue
		}

		var leader bool
		err=conn.QueryRow(ctx,`SELECT pg_try_advisory_lock($1)`,r.lockID).Scan(&leader)
		if err!=nil{
			conn.Release()
			r.logger.Warn("leadership lock check failed","error",err)
			if !sleep(ctx,r.retry){return}
			continue
		}
		if !leader{
			conn.Release()
			if !sleep(ctx,r.retry){return}
			continue
		}

		r.logger.Info("control plane leadership acquired")
		leaderCtx,cancel:=context.WithCancel(ctx)
		done:=make(chan struct{})
		go func(){defer close(done);run(leaderCtx)}()

		ticker:=time.NewTicker(r.ping)
		lost:=false
		for !lost{
			select{
			case <-ctx.Done():
				lost=true
			case <-done:
				lost=true
			case <-ticker.C:
				pingCtx,pingCancel:=context.WithTimeout(ctx,3*time.Second)
				var one int
				pingErr:=conn.QueryRow(pingCtx,`SELECT 1`).Scan(&one)
				pingCancel()
				if pingErr!=nil||one!=1{
					r.logger.Warn("control plane leadership lost","error",pingErr)
					lost=true
				}
			}
		}
		ticker.Stop()
		cancel()
		select{
		case <-done:
		case <-time.After(5*time.Second):
			r.logger.Warn("leader jobs did not stop within grace period")
		}

		unlockCtx,unlockCancel:=context.WithTimeout(context.Background(),2*time.Second)
		_,_ = conn.Exec(unlockCtx,`SELECT pg_advisory_unlock($1)`,r.lockID)
		unlockCancel()
		conn.Release()
		r.logger.Info("control plane leadership released")

		if ctx.Err()!=nil{return}
		if !sleep(ctx,r.retry){return}
	}
}

func sleep(ctx context.Context,d time.Duration)bool{
	t:=time.NewTimer(d);defer t.Stop()
	select{
	case <-ctx.Done():return false
	case <-t.C:return true
	}
}
