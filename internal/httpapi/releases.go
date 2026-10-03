package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	root,err:=filepath.Abs(s.cfg.ArtifactDir);if err!=nil{s.internalError(w,r,err);return}
	full,err:=filepath.Abs(filepath.Join(root,filepath.FromSlash(artifact.StorageKey)));if err!=nil{s.internalError(w,r,err);return}
	rel,err:=filepath.Rel(root,full)
	if err!=nil || rel==".." || strings.HasPrefix(rel,".."+string(os.PathSeparator)){
		s.internalError(w,r,fmt.Errorf("artifact path escaped storage root"));return
	}
	f,err:=os.Open(full);if err!=nil{writeError(w,http.StatusNotFound,"artifact_file_missing");return}
	defer f.Close()
	stat,err:=f.Stat();if err!=nil{s.internalError(w,r,err);return}
	h:=sha256.New()
	if _,err:=io.Copy(h,f);err!=nil{s.internalError(w,r,err);return}
	if hex.EncodeToString(h.Sum(nil))!=artifact.SHA256{
		s.logger.Error("artifact integrity mismatch","artifact_id",artifact.ID,"path",full)
		writeError(w,http.StatusConflict,"artifact_integrity_failed");return
	}
	if _,err:=f.Seek(0,io.SeekStart);err!=nil{s.internalError(w,r,err);return}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(s.cfg.ArtifactTransferTimeout))
	w.Header().Set("Content-Disposition",fmt.Sprintf("attachment; filename=%q",artifact.FileName))
	w.Header().Set("X-VPNX3-SHA256",artifact.SHA256)
	http.ServeContent(w,r,artifact.FileName,stat.ModTime(),f)
}
