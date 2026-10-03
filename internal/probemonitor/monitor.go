package probemonitor

import (
	"context"
	"log/slog"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type Monitor struct {
	store *store.Store
	logger *slog.Logger
}

func New(s *store.Store,logger *slog.Logger) *Monitor {
	return &Monitor{store:s,logger:logger}
}

func (m *Monitor) Run(ctx context.Context) {
	ticker:=time.NewTicker(30*time.Second)
	defer ticker.Stop()
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
	ctx,cancel:=context.WithTimeout(parent,8*time.Second)
	defer cancel()

	if err:=m.store.RecalculateProbeHealth(ctx,5*time.Minute); err!=nil {
		m.logger.Error("probe health recalculation failed","error",err)
		return
	}
	changes,err:=m.store.EvaluateProbeCircuitBreakers(ctx,3*time.Minute)
	if err!=nil {
		m.logger.Error("probe circuit breaker evaluation failed","error",err)
		return
	}
	for _,change:=range changes {
		if change.Opened {
			m.logger.Warn("probe circuit breaker opened","node_id",change.NodeID,"health_score",change.Score)
		} else {
			m.logger.Info("probe circuit breaker recovered","node_id",change.NodeID,"health_score",change.Score)
		}
	}
}
