package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleReleasePolicy(w http.ResponseWriter,r *http.Request){
	if s.releaseSigner==nil{writeError(w,http.StatusServiceUnavailable,"release_channel_disabled");return}
	target:=strings.TrimSpace(r.URL.Query().Get("target"))
	policy,err:=s.store.ReleasePolicy(r.Context(),target)
	if err!=nil{
		if strings.Contains(err.Error(),"invalid release target"){
			writeError(w,http.StatusBadRequest,"invalid_release_target");return
		}
		s.internalError(w,r,err);return
	}
	payload:=map[string]any{
		"schema_version":1,
		"target":policy.Target,
		"minimum_supported_version":policy.MinimumSupportedVersion,
		"recommended_version":policy.RecommendedVersion,
		"rollout_percent":policy.RolloutPercent,
		"blocked_versions":policy.BlockedVersions,
		"message":policy.Message,
		"policy_updated_at":policy.UpdatedAt.UTC().Truncate(time.Second),
		"issued_at":time.Now().UTC().Truncate(time.Second),
		"expires_at":time.Now().UTC().Add(10*time.Minute).Truncate(time.Second),
	}
	raw,err:=json.Marshal(payload);if err!=nil{s.internalError(w,r,err);return}
	sig:=s.releaseSigner.Sign(raw)
	w.Header().Set("Cache-Control","public, max-age=60")
	writeJSON(w,http.StatusOK,releaseEnvelope{
		Payload:base64.RawURLEncoding.EncodeToString(raw),
		Signature:base64.RawURLEncoding.EncodeToString(sig),
		KeyID:s.releaseSigner.KeyID(),
	})
}
