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
			checkCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
			err:=m.store.RecalculateProbeHealth(checkCtx,5*time.Minute)
			cancel()
			if err!=nil { m.logger.Error("probe health recalculation failed","error",err) }
		}
	}
}
