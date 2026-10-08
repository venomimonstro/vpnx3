package httpapi

import (
	"net/http"
	"strings"
)

func (s *Server) handleAdminReleasePolicy(w http.ResponseWriter,r *http.Request){
	target:=strings.TrimSpace(r.URL.Query().Get("target"))
	policy,err:=s.store.ReleasePolicy(r.Context(),target)
	if err!=nil{
		if strings.Contains(err.Error(),"invalid release target"){writeError(w,http.StatusBadRequest,"invalid_release_target");return}
		s.internalError(w,r,err);return
	}
	writeJSON(w,http.StatusOK,policy)
}

func (s *Server) handleSetReleasePolicy(w http.ResponseWriter,r *http.Request){
	var req struct{
		Target string `json:"target"`
		MinimumSupportedVersion string `json:"minimum_supported_version"`
		RecommendedVersion string `json:"recommended_version"`
		RolloutPercent int `json:"rollout_percent"`
		BlockedVersions []string `json:"blocked_versions"`
		Message string `json:"message"`
	}
	if err:=decodeJSON(w,r,&req);err!=nil{return}
	admin,_:=adminFromContext(r.Context())
	policy,err:=s.store.SetReleasePolicy(
		r.Context(),req.Target,req.MinimumSupportedVersion,req.RecommendedVersion,
		req.RolloutPercent,req.BlockedVersions,req.Message,admin.ID,
	)
	if err!=nil{
		writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_release_policy","detail":err.Error()})
		return
	}
	_ = s.store.WriteAudit(
		r.Context(),"admin",admin.ID,"release.policy.update","release_policy",policy.Target,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success",
	)
	writeJSON(w,http.StatusOK,policy)
}
