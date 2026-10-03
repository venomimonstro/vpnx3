package httpapi

import (
	"net/http"
	"strconv"
	"strings"
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
