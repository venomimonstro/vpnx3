package incidentautomation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type Engine struct {
	store *store.Store
	logger *slog.Logger
	securityExportConfigured bool
	interval time.Duration
}

func New(s *store.Store,logger *slog.Logger,securityExportConfigured bool,interval time.Duration)*Engine{
	if interval<15*time.Second{interval=time.Minute}
	return &Engine{
		store:s,logger:logger,securityExportConfigured:securityExportConfigured,interval:interval,
	}
}

func (e *Engine) Run(ctx context.Context){
	t:=time.NewTicker(e.interval)
	defer t.Stop()
	e.evaluate(ctx)
	for{
		select{
		case <-ctx.Done():return
		case <-t.C:e.evaluate(ctx)
		}
	}
}

type condition struct{
	code,title,detail string
	healthy bool
}

func (e *Engine) evaluate(parent context.Context){
	ctx,cancel:=context.WithTimeout(parent,15*time.Second)
	defer cancel()
	data,err:=e.store.LaunchReadiness(ctx)
	if err!=nil{
		e.logger.Error("incident automation readiness failed","error",err)
		return
	}
	conditions:=[]condition{
		{
			code:"worker_pool",
			title:"Нет маршрутизируемого VPN worker",
			detail:fmt.Sprintf("Активных worker: %d; маршрутизируемых worker: %d.",data.ActiveWorkers,data.RoutableWorkers),
			healthy:data.RoutableWorkers>0,
		},
		{
			code:"probe_freshness",
			title:"Нет свежего независимого наблюдения",
			detail:fmt.Sprintf("Активных probe: %d; свежих probe: %d.",data.ActiveProbes,data.FreshProbeNodes),
			healthy:data.ActiveProbes>0&&data.FreshProbeNodes>0,
		},
		{
			code:"wireguard_data_plane",
			title:"Synthetic WireGuard data plane не подтверждён",
			detail:fmt.Sprintf("Worker с успешным свежим data-plane probe: %d.",data.FreshDataPlaneWorkers),
			healthy:data.FreshDataPlaneWorkers>0,
		},
		{
			code:"audit_chain",
			title:"Нарушена целостность audit hash-chain",
			detail:"verify_audit_chain() вернул false.",
			healthy:data.AuditChainValid,
		},
	}
	if e.securityExportConfigured{
		externalHealthy:=data.SecurityExportDead==0
		if data.SecurityExportOldestPendingAt!=nil{
			externalHealthy=externalHealthy&&time.Since(data.SecurityExportOldestPendingAt.UTC())<=time.Hour
		}
		conditions=append(conditions,condition{
			code:"external_audit",
			title:"Деградировал внешний журнал безопасности",
			detail:fmt.Sprintf("Pending: %d; dead-letter: %d.",data.SecurityExportPending,data.SecurityExportDead),
			healthy:externalHealthy,
		})
	}
	if data.QueuedBuildJobs>0{
		conditions=append(conditions,condition{
			code:"build_factory",
			title:"Очередь сборки без активного build worker",
			detail:fmt.Sprintf("Queued: %d; active build workers: %d.",data.QueuedBuildJobs,data.ActiveBuildWorkers),
			healthy:data.ActiveBuildWorkers>0,
		})
	}

	for _,c:=range conditions{
		result,err:=e.store.EvaluateOperationalCondition(
			ctx,c.code,c.title,c.detail,c.healthy,3,3,time.Now().UTC(),
		)
		if err!=nil{
			e.logger.Error("operational condition evaluation failed","condition",c.code,"error",err)
			continue
		}
		if result.OpenedIncidentID!=""{
			_ = e.store.WriteAudit(
				ctx,"system","incident-automation","incident.auto_open","incident",
				result.OpenedIncidentID,"","", "success",
			)
			e.logger.Error("operational incident opened",
				"condition",c.code,"incident_id",result.OpenedIncidentID)
		}
		if result.ResolvedIncidentID!=""{
			_ = e.store.WriteAudit(
				ctx,"system","incident-automation","incident.auto_resolve","incident",
				result.ResolvedIncidentID,"","", "success",
			)
			e.logger.Info("operational incident resolved",
				"condition",c.code,"incident_id",result.ResolvedIncidentID)
		}
	}
}
