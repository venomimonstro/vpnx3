package httpapi

import (
	"errors"
	"net/http"
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
	release,err:=s.store.PublishRelease(r.Context(),r.PathValue("id"))
	if err!=nil{
		if err.Error()=="not found"{writeError(w,http.StatusNotFound,"release_not_found");return}
		if strings.Contains(err.Error(),"not ready")||strings.Contains(err.Error(),"incomplete"){
			writeJSON(w,http.StatusConflict,map[string]string{"error":"release_not_publishable","detail":err.Error()});return
		}
		s.internalError(w,r,err);return
	}
	admin,_:=adminFromContext(r.Context())
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
	if err!=nil{
		s.logger.Error("admin artifact download failed","artifact_id",artifact.ID,"error",err)
		if !responseStarted(w){writeError(w,http.StatusConflict,"artifact_integrity_failed")}
	}
}

func responseStarted(w http.ResponseWriter) bool {
	// net/http does not expose write state. This helper intentionally stays
	// conservative; serveArtifact returns integrity errors before WriteHeader.
	return false
}
