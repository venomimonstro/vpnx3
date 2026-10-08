package httpapi

import (
	"errors"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"
	"strconv"
	"strings"

	"github.com/venomimonstro/vpnx3/internal/artifactstorage"
)

func (s *Server) handleReleases(w http.ResponseWriter,r *http.Request) {
	limit:=100
	if raw:=r.URL.Query().Get("limit"); raw!="" {
		if v,err:=strconv.Atoi(raw); err==nil { limit=v }
	}
	rows,err:=s.store.ListReleases(r.Context(),limit)
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"releases":rows})
}

func (s *Server) handleReleaseJobs(w http.ResponseWriter,r *http.Request) {
	rows,err:=s.store.ReleaseJobs(r.Context(),r.PathValue("id"))
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"jobs":rows})
}

func (s *Server) handleCreateRelease(w http.ResponseWriter,r *http.Request) {
	var req struct {
		Version string `json:"version"`
		SourceCommit string `json:"source_commit"`
		Notes string `json:"notes"`
		Targets []string `json:"targets"`
	}
	if err:=decodeJSON(w,r,&req); err!=nil { return }
	admin,_:=adminFromContext(r.Context())
	release,err:=s.store.CreateRelease(r.Context(),req.Version,req.SourceCommit,req.Notes,admin.ID,req.Targets)
	if err!=nil {
		if strings.Contains(err.Error(),"invalid") {
			writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_release","detail":err.Error()})
			return
		}
		s.internalError(w,r,err);return
	}
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"release.create","release",release.ID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	writeJSON(w,http.StatusCreated,release)
}

func (s *Server) handleRetryBuildJob(w http.ResponseWriter,r *http.Request) {
	releaseID,err:=s.store.RetryBuildJob(r.Context(),r.PathValue("jobId"))
	if err!=nil {
		if err.Error()=="not found" { writeError(w,http.StatusNotFound,"build_job_not_found");return }
		if strings.Contains(err.Error(),"not retryable") {
			writeError(w,http.StatusConflict,"build_job_not_retryable");return
		}
		s.internalError(w,r,err);return
	}
	admin,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"release.build.retry","release",releaseID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReleaseArtifacts(w http.ResponseWriter,r *http.Request) {
	rows,err:=s.store.ReleaseArtifacts(r.Context(),r.PathValue("id"))
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,map[string]any{"artifacts":rows})
}

func (s *Server) handlePublishRelease(w http.ResponseWriter,r *http.Request) {
	gate,gateErr:=s.releaseGate(r.Context(),r.PathValue("id"))
	if gateErr!=nil{s.internalError(w,r,gateErr);return}
	if gate.Enforced&&len(gate.Blockers)>0{
		admin,_:=adminFromContext(r.Context())
		_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"release.publish.blocked","release",r.PathValue("id"),
			requestIDFromContext(r.Context()),ipString(clientIP(r)),"blocked")
		writeJSON(w,http.StatusConflict,map[string]any{
			"error":"release_gate_failed","blockers":gate.Blockers,"warnings":gate.Warnings,
		})
		return
	}
	admin,_:=adminFromContext(r.Context())
	blockersRaw,_:=json.Marshal(gate.Blockers)
	warningsRaw,_:=json.Marshal(gate.Warnings)
	signalsRaw,_:=json.Marshal(gate.Signals)
	release,err:=s.store.PublishReleaseWithAttestation(
		r.Context(),r.PathValue("id"),admin.ID,s.cfg.Environment,gate.Status,
		blockersRaw,warningsRaw,signalsRaw,gate.CheckedAt,
	)
	if err!=nil{
		if err.Error()=="not found"{writeError(w,http.StatusNotFound,"release_not_found");return}
		if strings.Contains(err.Error(),"not ready")||strings.Contains(err.Error(),"incomplete"){
			writeJSON(w,http.StatusConflict,map[string]string{"error":"release_not_publishable","detail":err.Error()});return
		}
		s.internalError(w,r,err);return
	}
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"release.publish","release",release.ID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	writeJSON(w,http.StatusOK,release)
}

func (s *Server) handleWithdrawRelease(w http.ResponseWriter,r *http.Request) {
	release,err:=s.store.WithdrawRelease(r.Context(),r.PathValue("id"))
	if err!=nil{
		if strings.Contains(err.Error(),"not published"){writeError(w,http.StatusConflict,"release_not_published");return}
		s.internalError(w,r,err);return
	}
	admin,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"release.withdraw","release",release.ID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	writeJSON(w,http.StatusOK,release)
}

func (s *Server) handleDownloadArtifact(w http.ResponseWriter,r *http.Request) {
	artifact,err:=s.store.ReleaseArtifactByID(r.Context(),r.PathValue("id"),r.PathValue("artifactId"))
	if err!=nil{writeError(w,http.StatusNotFound,"artifact_not_found");return}
	err=s.serveArtifact(w,r,artifact.StorageKey,artifact.FileName,artifact.SHA256,false)
	if errors.Is(err,artifactstorage.ErrNotFound){
		writeError(w,http.StatusNotFound,"artifact_file_missing");return
	}
	if errors.Is(err,errArtifactStreamStarted){return}
	if err!=nil{
		s.logger.Error("admin artifact download failed","artifact_id",artifact.ID,"error",err)
		writeError(w,http.StatusConflict,"artifact_integrity_failed")
	}
}


type releaseGateResult struct {
	Status string `json:"status"`
	Enforced bool `json:"enforced"`
	Blockers []string `json:"blockers"`
	Warnings []string `json:"warnings"`
	Signals map[string]any `json:"signals"`
	CheckedAt time.Time `json:"checked_at"`
}

func (s *Server) releaseGate(ctx context.Context,releaseID string)(releaseGateResult,error){
	result:=releaseGateResult{
		Status:"ok",
		Enforced:s.cfg.Environment=="production",
		Blockers:[]string{},
		Warnings:[]string{},
		Signals:map[string]any{},
		CheckedAt:time.Now().UTC(),
	}
	add:=func(message string,critical bool){
		if critical&&result.Enforced{result.Blockers=append(result.Blockers,message)}
		else{result.Warnings=append(result.Warnings,message)}
	}

	data,err:=s.store.LaunchReadiness(ctx)
	if err!=nil{return releaseGateResult{},err}
	result.Signals=map[string]any{
		"active_workers":data.ActiveWorkers,
		"routable_workers":data.RoutableWorkers,
		"active_ingresses":data.ActiveIngresses,
		"active_probes":data.ActiveProbes,
		"fresh_probe_nodes":data.FreshProbeNodes,
		"fresh_data_plane_workers":data.FreshDataPlaneWorkers,
		"audit_chain_valid":data.AuditChainValid,
		"renewal_failed_24h":data.RenewalFailed24h,
		"renewal_disabled_failures":data.RenewalDisabledFailures,
	}

	if data.LatestManifestAt==nil||time.Since(data.LatestManifestAt.UTC())>2*time.Hour{
		add("Нет свежего подписанного Configuration Manifest",true)
	}
	if data.RoutableWorkers<1{add("Нет маршрутизируемого active VPN worker",true)}
	if data.ActiveProbes<1||data.FreshProbeNodes<1{add("Нет свежего независимого probe-наблюдения",true)}
	if data.FreshDataPlaneWorkers<1{add("Нет успешного synthetic WireGuard data-plane observation за 5 минут",true)}
	if !data.AuditChainValid{add("Нарушена хеш-цепочка audit_log",true)}
	if s.releaseSigner==nil{add("Release Signing Key не настроен",true)}
	if s.yooKassa==nil{
		add("ЮKassa не настроена — коммерческие платежи недоступны",true)
	}else{
		circuit:=s.yooKassa.CircuitSnapshot()
		result.Signals["payments_circuit"]=circuit
		switch circuit.State{
		case "open":
			add("ЮKassa circuit breaker открыт",true)
		case "half_open":
			add("ЮKassa circuit breaker находится в half-open",false)
		}
	}

	dbStats:=s.db.Stat()
	dbUtil:=float64(0)
	if dbStats.MaxConns()>0{dbUtil=float64(dbStats.AcquiredConns())/float64(dbStats.MaxConns())*100}
	result.Signals["database_pool_utilization_percent"]=dbUtil
	result.Signals["database_pool_acquired"]=dbStats.AcquiredConns()
	result.Signals["database_pool_max"]=dbStats.MaxConns()
	if dbUtil>=95{add("Primary PostgreSQL pool занят на 95% или больше",true)}
	if dbUtil>=80&&dbUtil<95{add("Primary PostgreSQL pool занят на 80% или больше",false)}

	if s.admission!=nil{
		current,limit,peak,rejected:=s.admission.snapshot()
		util:=float64(0)
		if limit>0{util=float64(current)/float64(limit)*100}
		result.Signals["http_admission"]=map[string]any{
			"current":current,"limit":limit,"peak":peak,"rejected_total":rejected,
			"utilization_percent":util,
		}
		if util>=95{add("HTTP admission занят на 95% или больше",true)}
		if util>=80&&util<95{add("HTTP admission занят на 80% или больше",false)}
	}

	var primaryRecovery bool
	var primaryLSN string
	if err:=s.db.QueryRow(ctx,`SELECT pg_is_in_recovery(),pg_current_wal_lsn()::text`).Scan(&primaryRecovery,&primaryLSN);err!=nil{
		add("Не удалось проверить PostgreSQL writer",true)
	}else if primaryRecovery{
		add("Control Plane подключён к PostgreSQL standby вместо writer",true)
	}

	if s.cfg.DatabaseHARequired{
		if s.replicaDB==nil{
			add("Strict PostgreSQL HA включён, но standby pool отсутствует",true)
		}else{
			var standbyRecovery bool
			var replayLSN string
			err:=s.replicaDB.QueryRow(ctx,`SELECT pg_is_in_recovery(),COALESCE(pg_last_wal_replay_lsn()::text,'')`).Scan(&standbyRecovery,&replayLSN)
			if err!=nil{
				add("PostgreSQL standby недоступен",true)
			}else if !standbyRecovery{
				add("Replica endpoint не является PostgreSQL standby",true)
			}else if replayLSN==""||primaryLSN==""{
				add("Не удалось определить PostgreSQL standby WAL lag",false)
			}else{
				var lagBytes float64
				if err:=s.db.QueryRow(ctx,`SELECT GREATEST(pg_wal_lsn_diff($1::pg_lsn,$2::pg_lsn),0)::float8`,primaryLSN,replayLSN).Scan(&lagBytes);err!=nil{
					add("Не удалось вычислить PostgreSQL standby WAL lag",false)
				}else{
					result.Signals["database_replica_lag_bytes"]=lagBytes
					if lagBytes>512*1024*1024{add("PostgreSQL standby WAL lag превышает 512 МБ",true)}
					if lagBytes>64*1024*1024&&lagBytes<=512*1024*1024{add("PostgreSQL standby WAL lag превышает 64 МБ",false)}
				}
			}
		}
	}

	if s.cfg.BackupStatusFile==""{
		add("Backup health-marker не настроен",true)
	}else if stat,statErr:=os.Stat(s.cfg.BackupStatusFile);statErr!=nil{
		add("Нет подтверждения успешного backup",true)
	}else if time.Since(stat.ModTime().UTC())>36*time.Hour{
		add("Последний успешный backup старше 36 часов",true)
	}

	if s.cfg.OffsiteBackupStatusFile==""{
		add("Off-site backup health-marker не настроен",true)
	}else if stat,err:=os.Stat(s.cfg.OffsiteBackupStatusFile);err!=nil{
		add("Нет подтверждения проверенного off-site backup",true)
	}else if time.Since(stat.ModTime().UTC())>36*time.Hour{
		add("Последний проверенный off-site backup старше 36 часов",true)
	}

	if s.cfg.WalOffsiteStatusFile==""{
		add("WAL off-site replication marker не настроен",true)
	}else if stat,err:=os.Stat(s.cfg.WalOffsiteStatusFile);err!=nil{
		add("Нет подтверждения off-site WAL replication",true)
	}else if time.Since(stat.ModTime().UTC())>30*time.Minute{
		add("Off-site WAL replication не подтверждалась больше 30 минут",true)
	}

	if checker,ok:=s.artifacts.(artifactstorage.ReadinessChecker);ok{
		checkCtx,cancel:=context.WithTimeout(ctx,3*time.Second)
		checkErr:=checker.Check(checkCtx)
		cancel()
		if checkErr!=nil{add("Artifact storage недоступен",true)}
	}

	jobs,err:=s.store.ReleaseJobs(ctx,releaseID)
	if err!=nil{return releaseGateResult{},err}
	needsIngress:=false
	for _,job:=range jobs{
		if job.Target=="chrome_zip"||job.Target=="firefox_zip"{needsIngress=true}
		if job.Status!="succeeded"{add("Не все build jobs завершены успешно",true);break}
	}
	if needsIngress&&data.ActiveIngresses<1{add("Browser release требует active ingress",true)}

	if data.RenewalFailed24h>0{
		add("Есть ошибки автопродления за последние 24 часа",false)
	}
	if data.RenewalDisabledFailures>0{
		add("Есть подписки, где auto-renew отключён после серии ошибок",false)
	}

	if len(result.Blockers)>0{result.Status="failed"}
	if len(result.Blockers)==0&&len(result.Warnings)>0{result.Status="warning"}
	return result,nil
}

func (s *Server) handleReleaseGate(w http.ResponseWriter,r *http.Request){
	gate,err:=s.releaseGate(r.Context(),r.PathValue("id"))
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,gate)
}
