package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/nodeauth"
)

func (s *Server) verifyBuildWorkerRequest(w http.ResponseWriter,r *http.Request,body []byte,sequence int64) (string,bool) {
	nodeID:=strings.TrimSpace(r.Header.Get("X-VPNX3-Node-ID"))
	ts:=strings.TrimSpace(r.Header.Get("X-VPNX3-Timestamp"))
	sig:=strings.TrimSpace(r.Header.Get("X-VPNX3-Signature"))
	if nodeID=="" || ts=="" || sig=="" || sequence<=0 {
		writeError(w,http.StatusUnauthorized,"missing_build_worker_signature");return "",false
	}
	state,err:=s.store.BuildWorkerAuthState(r.Context(),nodeID)
	if err!=nil || state.Role!="build_worker" || state.Status=="quarantined" || state.Status=="retired" || state.Status=="destroyed" {
		writeError(w,http.StatusUnauthorized,"invalid_build_worker");return "",false
	}
	if sequence<=state.Sequence {
		writeError(w,http.StatusConflict,"stale_build_request");return "",false
	}
	if err:=nodeauth.Verify(state.PublicKey,r.Method,r.URL.Path,ts,sig,body,time.Now().UTC()); err!=nil {
		writeError(w,http.StatusUnauthorized,"invalid_build_worker_signature");return "",false
	}
	if err:=s.store.AdvanceBuildSequence(r.Context(),nodeID,sequence); err!=nil {
		writeError(w,http.StatusConflict,"stale_build_request");return "",false
	}
	return nodeID,true
}

func (s *Server) handleBuildClaim(w http.ResponseWriter,r *http.Request) {
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,64<<10))
	if err!=nil { writeError(w,http.StatusBadRequest,"invalid_body");return }
	var req struct {
		Sequence int64 `json:"sequence"`
		Targets []string `json:"targets"`
	}
	decoder:=json.NewDecoder(bytes.NewReader(body));decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&req); err!=nil { writeError(w,http.StatusBadRequest,"invalid_json");return }
	nodeID,ok:=s.verifyBuildWorkerRequest(w,r,body,req.Sequence);if !ok{return}
	job,err:=s.store.ClaimBuildJob(r.Context(),nodeID,req.Targets)
	if err!=nil {
		if err.Error()=="not found" { w.WriteHeader(http.StatusNoContent);return }
		if strings.Contains(err.Error(),"not active") { writeError(w,http.StatusConflict,"build_worker_not_active");return }
		s.internalError(w,r,err);return
	}
	writeJSON(w,http.StatusOK,job)
}

func (s *Server) handleBuildComplete(w http.ResponseWriter,r *http.Request) {
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,64<<10))
	if err!=nil { writeError(w,http.StatusBadRequest,"invalid_body");return }
	var req struct {
		Sequence int64 `json:"sequence"`
		Status string `json:"status"`
		ErrorSummary string `json:"error_summary"`
	}
	decoder:=json.NewDecoder(bytes.NewReader(body));decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&req); err!=nil { writeError(w,http.StatusBadRequest,"invalid_json");return }
	nodeID,ok:=s.verifyBuildWorkerRequest(w,r,body,req.Sequence);if !ok{return}
	if err:=s.store.CompleteBuildJob(r.Context(),nodeID,r.PathValue("id"),req.Status,req.ErrorSummary); err!=nil {
		if err.Error()=="not found" { writeError(w,http.StatusNotFound,"build_job_not_found");return }
		if strings.Contains(err.Error(),"invalid build result") { writeError(w,http.StatusBadRequest,"invalid_build_result");return }
		s.internalError(w,r,err);return
	}
	w.WriteHeader(http.StatusNoContent)
}
