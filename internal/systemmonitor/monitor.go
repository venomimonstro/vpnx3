package systemmonitor

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type Monitor struct{
	store *store.Store
	logger *slog.Logger
	backupStatusFile string
	state map[string]*counter
}

type counter struct{bad,good int}

type check struct{
	code,severity,title,summary string
	failed bool
}

func New(s *store.Store,logger *slog.Logger,backupStatusFile string)*Monitor{
	return &Monitor{store:s,logger:logger,backupStatusFile:backupStatusFile,state:map[string]*counter{}}
}

func (m *Monitor) Run(ctx context.Context){
	t:=time.NewTicker(time.Minute);defer t.Stop()
	m.run(ctx)
	for{
		select{
		case <-ctx.Done():return
		case <-t.C:m.run(ctx)
		}
	}
}

func (m *Monitor) run(parent context.Context){
	ctx,cancel:=context.WithTimeout(parent,20*time.Second);defer cancel()
	data,err:=m.store.LaunchReadiness(ctx)
	if err!=nil{
		m.logger.Warn("system incident monitor readiness failed","error",err)
		return
	}
	checks:=[]check{
		{code:"worker_pool_empty",severity:"critical",title:"Нет маршрутизируемых VPN worker",
			summary:"Control Plane не видит ни одного active worker с полным session_api + WireGuard endpoint.",
			failed:data.RoutableWorkers<1},
		{code:"probe_visibility_lost",severity:"critical",title:"Потеря независимого наблюдения",
			summary:"Нет активных свежих независимых probe-наблюдений.",
			failed:data.ActiveProbes<1||data.FreshProbeNodes<1},
		{code:"wireguard_data_plane_unconfirmed",severity:"warning",title:"WireGuard data plane не подтверждён",
			summary:"Нет свежего успешного synthetic WireGuard data-plane observation.",
			failed:data.FreshDataPlaneWorkers<1},
		{code:"build_queue_without_worker",severity:"warning",title:"Очередь сборок без Build Worker",
			summary:"Есть queued/running build jobs, но нет active build worker.",
			failed:(data.QueuedBuildJobs>0||data.RunningBuildJobs>0)&&data.ActiveBuildWorkers<1},
		{code:"audit_chain_invalid",severity:"critical",title:"Нарушена целостность audit chain",
			summary:"verify_audit_chain() вернул false. Требуется немедленная проверка безопасности.",
			failed:!data.AuditChainValid},
	}

	if m.backupStatusFile!=""{
		failed:=true
		if stat,statErr:=os.Stat(m.backupStatusFile);statErr==nil{
			failed=time.Since(stat.ModTime().UTC())>36*time.Hour
		}
		checks=append(checks,check{
			code:"backup_stale",severity:"critical",title:"Резервная копия просрочена",
			summary:"Нет подтверждения успешного backup за допустимое окно 36 часов.",
			failed:failed,
		})
	}

	if export,exportErr:=m.store.SecurityExportHealth(ctx);exportErr==nil&&export.OldestPendingAt!=nil{
		checks=append(checks,check{
			code:"security_export_stalled",severity:"warning",title:"Задержка внешнего security-журнала",
			summary:"Самое старое недоставленное audit-событие находится в очереди более двух часов.",
			failed:time.Since(export.OldestPendingAt.UTC())>2*time.Hour,
		})
	}
	if pending,oldest,_,notifyErr:=m.store.IncidentNotificationHealth(ctx);notifyErr==nil&&pending>0&&oldest!=nil{
		checks=append(checks,check{
			code:"incident_notifications_stalled",severity:"warning",title:"Задержка операционных уведомлений",
			summary:"Самое старое уведомление об инциденте не доставлено более двух часов.",
			failed:time.Since(oldest.UTC())>2*time.Hour,
		})
	}

	for _,c:=range checks{m.observe(ctx,c)}
}

func (m *Monitor) observe(ctx context.Context,c check){
	state:=m.state[c.code]
	if state==nil{state=&counter{};m.state[c.code]=state}
	if c.failed{
		state.bad++;state.good=0
		if state.bad<3{return}
		if err:=m.store.SyncSystemIncident(ctx,c.code,c.severity,c.title,c.summary,true,time.Now().UTC());err!=nil{
			m.logger.Warn("open system incident failed","code",c.code,"error",err)
		}
		return
	}
	state.good++;state.bad=0
	if state.good<2{return}
	if err:=m.store.SyncSystemIncident(ctx,c.code,c.severity,c.title,c.summary,false,time.Now().UTC());err!=nil{
		m.logger.Warn("resolve system incident failed","code",c.code,"error",err)
	}
}
