package nodemonitor

import (
	"context"
	"log/slog"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type Monitor struct {
	store *store.Store
	logger *slog.Logger
	interval time.Duration
	staleAfter time.Duration
	onRoutingChange func()
}

func New(s *store.Store,logger *slog.Logger,onRoutingChange ...func()) *Monitor {
	var callback func()
	if len(onRoutingChange)>0 { callback=onRoutingChange[0] }
	return &Monitor{
		store:s,
		logger:logger,
		interval:20*time.Second,
		staleAfter:90*time.Second,
		onRoutingChange:callback,
	}
}

func (m *Monitor) Run(ctx context.Context) {
	ticker:=time.NewTicker(m.interval)
	defer ticker.Stop()

	m.check(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *Monitor) check(parent context.Context) {
	ctx,cancel:=context.WithTimeout(parent,5*time.Second)
	defer cancel()
	ids,err:=m.store.DegradeStaleNodes(ctx,m.staleAfter)
	if err!=nil {
		m.logger.Error("node stale check failed","error",err)
		return
	}
	for _,id:=range ids {
		m.logger.Warn("node marked degraded after heartbeat timeout","node_id",id)
	}
	if len(ids)>0 && m.onRoutingChange!=nil {
		m.onRoutingChange()
	}
}
