package httpapi

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/artifactstorage"
	"github.com/venomimonstro/vpnx3/internal/trustbundle"
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

	checks:=make([]readinessCheck,0,16)
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

	if data.RoutableConfigMirrors==0{
		add("config_mirrors","failed","Зеркала конфигурации","Нет active HTTPS config mirror; клиенты зависят от единственной точки Control Plane.")
	}else if data.RoutableConfigMirrors==1{
		add("config_mirrors","warning","Зеркала конфигурации","Доступно только одно config mirror; для устойчивости нужно минимум два независимых зеркала.")
	}else if data.RoutableConfigMirrors<data.ActiveConfigMirrors{
		add("config_mirrors","warning","Зеркала конфигурации","Не все active config mirror имеют корректный HTTPS endpoint.")
	}else{
		add("config_mirrors","ok","Зеркала конфигурации","Доступно минимум два маршрутизируемых config mirror.")
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
		if data.RenewalFailed24h>0 || data.RenewalDisabledFailures>0 {
			add("auto_renew","warning","Автопродление","Есть ошибки автопродления за 24 часа или подписки, отключённые после серии ошибок.")
		}else if data.AutoRenewActive>0 {
			add("auto_renew","ok","Автопродление","Активные автопродления работают без зафиксированных ошибок за 24 часа.")
		}else{
			add("auto_renew","ok","Автопродление","Механизм готов; активных подписок с автопродлением пока нет.")
		}
	}

	if s.cfg.Environment=="production" {
		if s.trustBundle==nil {
			add("trust_bundle","failed","Корень доверия","Production запущен без root-signed trust bundle.")
		}else if payload,err:=trustbundle.DecodeVerifiedEnvelope(*s.trustBundle);err!=nil{
			add("trust_bundle","failed","Корень доверия","Не удалось прочитать уже проверенный trust bundle.")
		}else{
			remaining:=time.Until(payload.ExpiresAt)
			switch{
			case remaining<=0:
				add("trust_bundle","failed","Корень доверия","Trust bundle просрочен; безопасная ротация ключей недоступна.")
			case remaining<7*24*time.Hour:
				add("trust_bundle","warning","Корень доверия","Trust bundle истекает менее чем через 7 дней.")
			default:
				add("trust_bundle","ok","Корень доверия","Root-signed trust bundle действителен.")
			}
		}
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

	if !data.AuditChainValid {
		add("audit_chain","failed","Целостность аудита","Хеш-цепочка audit_log повреждена или не соответствует данным.")
	}else{
		add("audit_chain","ok","Целостность аудита","Append-only журнал и хеш-цепочка согласованы.")
	}

	securityExportSignals:=map[string]any{"configured":s.cfg.SecurityExportURL!=""}
	if s.cfg.SecurityExportURL=="" {
		add("security_export","warning","Внешний журнал безопасности","Внешний HTTPS/WORM/SIEM экспорт не настроен.")
	}else{
		health,healthErr:=s.store.SecurityExportHealth(r.Context())
		if healthErr!=nil{
			add("security_export","failed","Внешний журнал безопасности","Не удалось проверить очередь внешнего аудита.")
		}else{
			securityExportSignals["pending"]=health.Pending
			securityExportSignals["dead"]=health.Dead
			securityExportSignals["max_attempts"]=health.MaxAttempts
			securityExportSignals["oldest_pending_at"]=health.OldestPendingAt
			if health.Dead>0{
				add("security_export","failed","Внешний журнал безопасности","Есть dead-letter события внешнего аудита.")
			}else if health.OldestPendingAt==nil{
				add("security_export","ok","Внешний журнал безопасности","Очередь внешнего аудита пуста.")
			}else{
				age:=time.Since(health.OldestPendingAt.UTC())
				switch{
				case age>2*time.Hour:
					add("security_export","failed","Внешний журнал безопасности","Самое старое недоставленное событие ждёт больше 2 часов.")
				case age>15*time.Minute||health.MaxAttempts>=8:
					add("security_export","warning","Внешний журнал безопасности","Есть заметная задержка или многократные ошибки доставки событий.")
				default:
					add("security_export","ok","Внешний журнал безопасности","Очередь доставки находится в допустимом окне.")
				}
			}
		}
	}

	if s.cfg.BackupStatusFile=="" {
		add("backup","warning","Резервное копирование","Backup health-marker не настроен.")
	}else if stat,err:=os.Stat(s.cfg.BackupStatusFile);err!=nil {
		add("backup","failed","Резервное копирование","Нет подтверждения успешной резервной копии.")
	}else{
		age:=time.Since(stat.ModTime().UTC())
		switch{
		case age<=30*time.Hour:
			add("backup","ok","Резервное копирование","Есть свежая успешная резервная копия.")
		case age<=36*time.Hour:
			add("backup","warning","Резервное копирование","Последний успешный backup старше 30 часов.")
		default:
			add("backup","failed","Резервное копирование","Последний успешный backup старше 36 часов.")
		}
	}

	if s.cfg.OffsiteBackupStatusFile=="" {
		add("backup_offsite","warning","Внешняя резервная копия","Off-site backup marker не настроен; отказ основного хоста/аккаунта остаётся единичной точкой потери.")
	}else if stat,err:=os.Stat(s.cfg.OffsiteBackupStatusFile);err!=nil {
		add("backup_offsite","failed","Внешняя резервная копия","Нет подтверждения проверенной копии у независимого storage-провайдера.")
	}else{
		age:=time.Since(stat.ModTime().UTC())
		switch{
		case age<=30*time.Hour:
			add("backup_offsite","ok","Внешняя резервная копия","Свежий зашифрованный backup проверен после загрузки во внешнее хранилище.")
		case age<=36*time.Hour:
			add("backup_offsite","warning","Внешняя резервная копия","Последняя проверенная off-site копия старше 30 часов.")
		default:
			add("backup_offsite","failed","Внешняя резервная копия","Последняя проверенная off-site копия старше 36 часов.")
		}
	}

	if s.cfg.WalOffsiteStatusFile=="" {
		add("pitr_wal","warning","Непрерывное восстановление БД","WAL off-site marker не настроен; RPO ограничен периодом полного backup.")
	}else if stat,err:=os.Stat(s.cfg.WalOffsiteStatusFile);err!=nil {
		add("pitr_wal","failed","Непрерывное восстановление БД","Нет подтверждения внешней репликации зашифрованного WAL.")
	}else{
		age:=time.Since(stat.ModTime().UTC())
		backlog:=-1
		if raw,err:=os.ReadFile(s.cfg.WalOffsiteStatusFile);err==nil{
			for _,line:=range strings.Split(string(raw),"\n"){
				if strings.HasPrefix(line,"backlog_files="){
					if n,err:=strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line,"backlog_files=")));err==nil{backlog=n}
				}
			}
		}
		switch{
		case age>30*time.Minute:
			add("pitr_wal","failed","Непрерывное восстановление БД","WAL off-site replication не подтверждалась больше 30 минут.")
		case backlog>128:
			add("pitr_wal","failed","Непрерывное восстановление БД","Очередь WAL превышает 128 сегментов; RPO быстро ухудшается.")
		case age>10*time.Minute||backlog>16:
			add("pitr_wal","warning","Непрерывное восстановление БД","Есть задержка WAL replication или заметная очередь сегментов.")
		case backlog<0:
			add("pitr_wal","warning","Непрерывное восстановление БД","WAL marker свежий, но backlog не удалось прочитать.")
		default:
			add("pitr_wal","ok","Непрерывное восстановление БД","Зашифрованный WAL регулярно реплицируется во внешнее хранилище.")
		}
	}

	alertSignals:=map[string]any{"configured":s.cfg.AlertWebhookURL!=""}
	if s.cfg.AlertWebhookURL==""{
		add("incident_notifications","warning","Операционные уведомления","HTTPS webhook для инцидентов не настроен.")
	}else{
		pending,oldest,maxAttempts,alertErr:=s.store.IncidentNotificationHealth(r.Context())
		if alertErr!=nil{
			add("incident_notifications","failed","Операционные уведомления","Не удалось проверить очередь уведомлений.")
		}else{
			alertSignals["pending"]=pending
			alertSignals["oldest_pending_at"]=oldest
			alertSignals["max_attempts"]=maxAttempts
			if oldest==nil{
				add("incident_notifications","ok","Операционные уведомления","Очередь уведомлений пуста.")
			}else{
				age:=time.Since(oldest.UTC())
				if age>2*time.Hour{
					add("incident_notifications","failed","Операционные уведомления","Уведомление не доставляется больше 2 часов.")
				}else if age>15*time.Minute||maxAttempts>=8{
					add("incident_notifications","warning","Операционные уведомления","Есть задержка или повторные ошибки доставки.")
				}else{
					add("incident_notifications","ok","Операционные уведомления","Очередь находится в допустимом окне.")
				}
			}
		}
	}

	risk,riskErr:=s.store.NetworkRisk(r.Context())
	if riskErr!=nil{
		add("network_capacity","failed","Ёмкость и распределение сети","Не удалось рассчитать ёмкость active worker.")
	}else{
		if risk.ActiveWorkers>0 && risk.ConfiguredCapacity<=0{
			add("network_capacity","warning","Ёмкость и распределение сети","Для active worker не настроена capacity_sessions.")
		}else if risk.UtilizationPercent>=85{
			add("network_capacity","failed","Ёмкость и распределение сети","Используется 85% или больше настроенной ёмкости.")
		}else if risk.UtilizationPercent>=70||risk.WorkersWithoutCapacity>0{
			add("network_capacity","warning","Ёмкость и распределение сети","Запас сети сокращён или часть worker не имеет capacity.")
		}else{
			add("network_capacity","ok","Ёмкость и распределение сети","Запас worker pool находится в рабочем диапазоне.")
		}
		if risk.MaxProviderShare>=70{
			add("provider_concentration","failed","Концентрация провайдера","70% или больше worker capacity зависит от одного провайдера.")
		}else if risk.MaxProviderShare>=50{
			add("provider_concentration","warning","Концентрация провайдера","50% или больше worker capacity зависит от одного провайдера.")
		}else{
			add("provider_concentration","ok","Концентрация провайдера","Нет критической зависимости от одного провайдера по настроенной capacity.")
		}
	}

	primaryInRecovery:=false
	primaryLSN:=""
	if err:=s.db.QueryRow(r.Context(),`SELECT pg_is_in_recovery(),pg_current_wal_lsn()::text`).Scan(&primaryInRecovery,&primaryLSN);err!=nil{
		add("database_primary","failed","Основная база данных","Не удалось проверить PostgreSQL writer.")
	}else if primaryInRecovery{
		add("database_primary","failed","Основная база данных","Control Plane подключён к standby вместо writer.")
	}else{
		add("database_primary","ok","Основная база данных","PostgreSQL writer доступен.")
	}

	if s.replicaDB==nil{
		if s.cfg.DatabaseHARequired{
			add("database_replica","failed","Резервная база данных","HA обязателен, но standby PostgreSQL не подключён.")
		}else{
			add("database_replica","warning","Резервная база данных","Standby PostgreSQL не настроен; база остаётся единичной точкой отказа.")
		}
	}else{
		replicaRecovery:=false
		replayLSN:=""
		if err:=s.replicaDB.QueryRow(r.Context(),`SELECT pg_is_in_recovery(),COALESCE(pg_last_wal_replay_lsn()::text,'')`).Scan(&replicaRecovery,&replayLSN);err!=nil{
			add("database_replica","failed","Резервная база данных","Standby PostgreSQL недоступен.")
		}else if !replicaRecovery{
			add("database_replica","failed","Резервная база данных","Replica endpoint подключён не к standby PostgreSQL.")
		}else if replayLSN==""||primaryLSN==""{
			add("database_replica","warning","Резервная база данных","Standby доступен, но WAL replay LSN пока недоступен.")
		}else{
			var lagBytes float64
			if err:=s.db.QueryRow(r.Context(),`SELECT GREATEST(pg_wal_lsn_diff($1::pg_lsn,$2::pg_lsn),0)::float8`,primaryLSN,replayLSN).Scan(&lagBytes);err!=nil{
				add("database_replica","warning","Резервная база данных","Standby доступен, но не удалось вычислить WAL lag.")
			}else{
				switch{
				case lagBytes>512*1024*1024:
					add("database_replica","failed","Резервная база данных","WAL lag standby превышает 512 МБ.")
				case lagBytes>64*1024*1024:
					add("database_replica","warning","Резервная база данных","WAL lag standby превышает 64 МБ.")
				default:
					add("database_replica","ok","Резервная база данных","Standby PostgreSQL доступен и WAL lag находится в рабочем диапазоне.")
				}
			}
		}
	}

	if data.LeadershipHolder==nil||data.LeadershipHeartbeatAt==nil{
		add("control_plane_leader","warning","Лидер фоновых задач","Ни один экземпляр Control Plane не подтверждает лидерство singleton-задач.")
	}else{
		age:=time.Since(data.LeadershipHeartbeatAt.UTC())
		if age>30*time.Second{
			add("control_plane_leader","failed","Лидер фоновых задач","Heartbeat лидера старше 30 секунд; singleton-мониторы могут не выполняться.")
		}else{
			add("control_plane_leader","ok","Лидер фоновых задач","Singleton-задачи имеют активного HA-лидера с живым PostgreSQL advisory lease.")
		}
	}

	if data.RuntimeManagedNodes>0 {
		switch {
		case data.RuntimeUpdaterUnhealthyNodes>0:
			add("runtime_updater","warning","Обновление серверных компонентов","Есть active-ноды, где последняя подписанная проверка обновлений завершилась ошибкой.")
		case data.RuntimeUpdaterReportedNodes<data.RuntimeManagedNodes:
			add("runtime_updater","warning","Обновление серверных компонентов","Не все active runtime-ноды публикуют состояние защищённого updater.")
		default:
			add("runtime_updater","ok","Обновление серверных компонентов","Все active runtime-ноды публикуют успешное состояние подписанного updater.")
		}
	}

	if data.LocalHealthDegradedNodes>0{
		add("fleet_local_health","warning","Здоровье хостов","Есть ноды, автоматически выведенные из маршрутизации после устойчивого локального отказа.")
	}else if data.LocalHealthWarningNodes>0{
		add("fleet_local_health","warning","Здоровье хостов","Есть active-ноды с двумя подряд плохими локальными наблюдениями; следующий плохой heartbeat переведёт их в degraded.")
	}else{
		add("fleet_local_health","ok","Здоровье хостов","Устойчивых локальных отказов active runtime-нод не обнаружено.")
	}
	if data.NodesMissingDiskTelemetry>0{
		add("fleet_disk_telemetry","warning","Контроль диска","Не все active runtime-ноды публикуют данные о свободном месте.")
	}else if data.RuntimeManagedNodes>0{
		add("fleet_disk_telemetry","ok","Контроль диска","Все active runtime-ноды публикуют данные о диске.")
	}

	if data.ActiveIngresses<1{
		add("browser_ingress","warning","Browser ingress","Нет active ingress; браузерные расширения не смогут подключаться.")
	}else{
		add("browser_ingress","ok","Browser ingress","Есть active HTTPS ingress.")
	}

	admissionSignals:=map[string]any{}
	if s.admission!=nil{
		current,limit,peak,rejected:=s.admission.snapshot()
		admissionSignals["current"]=current
		admissionSignals["limit"]=limit
		admissionSignals["peak"]=peak
		admissionSignals["rejected_total"]=rejected
		utilization:=float64(0)
		if limit>0{utilization=float64(current)/float64(limit)*100}
		admissionSignals["utilization_percent"]=utilization
		switch{
		case utilization>=95:
			add("http_admission","warning","HTTP backpressure","Текущая загрузка достигла 95% лимита одновременных запросов.")
		case utilization>=80:
			add("http_admission","warning","HTTP backpressure","Текущая загрузка превышает 80% лимита одновременных запросов.")
		default:
			add("http_admission","ok","HTTP backpressure","Admission control работает в допустимом диапазоне.")
		}
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
			"active_config_mirrors":data.ActiveConfigMirrors,
			"routable_config_mirrors":data.RoutableConfigMirrors,
			"active_probes":data.ActiveProbes,
			"fresh_probe_nodes":data.FreshProbeNodes,
			"fresh_data_plane_workers":data.FreshDataPlaneWorkers,
			"queued_build_jobs":data.QueuedBuildJobs,
			"running_build_jobs":data.RunningBuildJobs,
			"active_build_workers":data.ActiveBuildWorkers,
			"published_releases":data.PublishedReleases,
			"auto_renew_active":data.AutoRenewActive,
			"renewal_pending":data.RenewalPending,
			"renewal_failed_24h":data.RenewalFailed24h,
			"renewal_disabled_failures":data.RenewalDisabledFailures,
			"audit_chain_valid":data.AuditChainValid,
			"security_export_pending":data.SecurityExportPending,
			"security_export_dead":data.SecurityExportDead,
			"security_export_oldest_pending_at":data.SecurityExportOldestPendingAt,
			"security_export_last_delivered_at":data.SecurityExportLastDeliveredAt,
			"security_export":securityExportSignals,
			"incident_notifications":alertSignals,
			"network_risk":risk,
			"runtime_managed_nodes":data.RuntimeManagedNodes,
			"runtime_updater_reported_nodes":data.RuntimeUpdaterReportedNodes,
			"runtime_updater_unhealthy_nodes":data.RuntimeUpdaterUnhealthyNodes,
			"local_health_warning_nodes":data.LocalHealthWarningNodes,
			"local_health_degraded_nodes":data.LocalHealthDegradedNodes,
			"nodes_missing_disk_telemetry":data.NodesMissingDiskTelemetry,
			"control_plane_leader":data.LeadershipHolder,
			"control_plane_leader_heartbeat_at":data.LeadershipHeartbeatAt,
			"control_plane_leader_acquired_at":data.LeadershipAcquiredAt,
			"control_plane_leadership_transitions":data.LeadershipTransitions,
			"http_admission":admissionSignals,
		},
	})
}
