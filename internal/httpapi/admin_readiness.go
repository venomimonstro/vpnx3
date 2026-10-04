package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/venomimonstro/vpnx3/internal/artifactstorage"
)

type readinessCheck struct {
	Code string `json:"code"`
	Status string `json:"status"`
	Title string `json:"title"`
	Detail string `json:"detail"`
}

func (s *Server) handleLaunchReadiness(w http.ResponseWriter,r *http.Request){
	data,err:=s.store.LaunchReadiness(r.Context())
	if err!=nil{s.internalError(w,r,err);return}

	checks:=make([]readinessCheck,0,10)
	add:=func(code,status,title,detail string){
		checks=append(checks,readinessCheck{Code:code,Status:status,Title:title,Detail:detail})
	}

	if data.LatestManifestAt==nil{
		add("config_manifest","failed","Подписанная конфигурация","Нет ни одного опубликованного manifest.")
	}else{
		age:=time.Since(data.LatestManifestAt.UTC())
		switch{
		case age<=30*time.Minute:
			add("config_manifest","ok","Подписанная конфигурация","Свежий manifest опубликован.")
		case age<=2*time.Hour:
			add("config_manifest","warning","Подписанная конфигурация","Manifest старше 30 минут.")
		default:
			add("config_manifest","failed","Подписанная конфигурация","Manifest старше 2 часов.")
		}
	}

	if data.RoutableWorkers<1{
		add("worker_pool","failed","VPN worker pool","Нет active worker с HTTPS session_api и UDP WireGuard endpoint.")
	}else if data.RoutableWorkers<data.ActiveWorkers{
		add("worker_pool","warning","VPN worker pool","Не все active worker имеют полный набор клиентских endpoints.")
	}else{
		add("worker_pool","ok","VPN worker pool","Есть маршрутизируемые active worker-ноды.")
	}

	if data.ActiveProbes<1||data.FreshProbeNodes<1{
		add("probes","failed","Независимые probes","Нет свежих наблюдений от active probe-ноды.")
	}else{
		add("probes","ok","Независимые probes","Свежие независимые наблюдения присутствуют.")
	}

	if data.FreshDataPlaneWorkers<1{
		add("wireguard_data_plane","warning","WireGuard data plane","Нет успешного synthetic WireGuard observation за последние 5 минут.")
	}else{
		add("wireguard_data_plane","ok","WireGuard data plane","Есть свежий успешный handshake + gateway probe.")
	}

	if data.QueuedBuildJobs>0&&data.ActiveBuildWorkers<1{
		add("build_factory","failed","Build Factory","Есть queued jobs, но нет active build worker.")
	}else if data.RunningBuildJobs>0&&data.ActiveBuildWorkers<1{
		add("build_factory","warning","Build Factory","Есть running jobs без active build worker; watchdog должен их вернуть в очередь.")
	}else{
		add("build_factory","ok","Build Factory","Очередь сборок согласована с активными worker.")
	}

	if s.releaseSigner==nil{
		add("release_signing","warning","Подписание релизов","Release signing key не настроен; публичный release channel отключён.")
	}else if data.PublishedReleases<1{
		add("release_signing","warning","Подписание релизов","Ключ настроен, но опубликованных релизов пока нет.")
	}else{
		add("release_signing","ok","Подписание релизов","Release signing и опубликованный релиз доступны.")
	}

	if s.yooKassa==nil{
		add("payments","warning","Платежи","ЮKassa не настроена; коммерческие покупки недоступны.")
	}else{
		add("payments","ok","Платежи","Платёжный адаптер настроен.")
	}

	storageStatus:="ok"
	storageDetail:="Artifact storage отвечает."
	if checker,ok:=s.artifacts.(artifactstorage.ReadinessChecker);ok{
		ctx,cancel:=context.WithTimeout(r.Context(),3*time.Second)
		err:=checker.Check(ctx)
		cancel()
		if err!=nil{
			storageStatus="failed"
			storageDetail="Artifact storage недоступен."
		}
	}
	add("artifact_storage",storageStatus,"Хранилище артефактов",storageDetail)

	if data.ActiveIngresses<1{
		add("browser_ingress","warning","Browser ingress","Нет active ingress; браузерные расширения не смогут подключаться.")
	}else{
		add("browser_ingress","ok","Browser ingress","Есть active HTTPS ingress.")
	}

	overall:="ok"
	for _,check:=range checks{
		if check.Status=="failed"{overall="failed";break}
		if check.Status=="warning"&&overall=="ok"{overall="warning"}
	}

	writeJSON(w,http.StatusOK,map[string]any{
		"status":overall,
		"checked_at":time.Now().UTC(),
		"checks":checks,
		"signals":map[string]any{
			"active_workers":data.ActiveWorkers,
			"routable_workers":data.RoutableWorkers,
			"active_ingresses":data.ActiveIngresses,
			"active_probes":data.ActiveProbes,
			"fresh_probe_nodes":data.FreshProbeNodes,
			"fresh_data_plane_workers":data.FreshDataPlaneWorkers,
			"queued_build_jobs":data.QueuedBuildJobs,
			"running_build_jobs":data.RunningBuildJobs,
			"active_build_workers":data.ActiveBuildWorkers,
			"published_releases":data.PublishedReleases,
		},
	})
}
