package buildwatchdog

import (
	"context"
	"log/slog"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type Watchdog struct {
	store *store.Store
	logger *slog.Logger
	staleAfter time.Duration
}

func New(s *store.Store,l *slog.Logger,staleAfter time.Duration)*Watchdog{
	if staleAfter<time.Minute{staleAfter=2*time.Minute}
	return &Watchdog{store:s,logger:l,staleAfter:staleAfter}
}

func (w *Watchdog) Run(ctx context.Context){
	w.check(ctx)
	t:=time.NewTicker(time.Minute)
	defer t.Stop()

	for{
		select{
		case <-ctx.Done():
			return
		case <-t.C:
			w.check(ctx)
		}
	}
}

func (w *Watchdog) check(parent context.Context){
	ctx,cancel:=context.WithTimeout(parent,15*time.Second)
	defer cancel()

	rows,err:=w.store.RequeueStaleBuildJobs(ctx,time.Now().UTC().Add(-w.staleAfter),100)
	if err!=nil{
		w.logger.Warn("build watchdog failed","error",err)
		return
	}
	for _,row:=range rows{
		w.logger.Warn(
			"stale build job requeued",
			"job_id",row.JobID,
			"release_id",row.ReleaseID,
			"former_worker_id",row.WorkerID,
		)
	}
}
