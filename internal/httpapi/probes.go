package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/nodeauth"
	"github.com/venomimonstro/vpnx3/internal/store"
)

func (s *Server) handleProbeReport(w http.ResponseWriter,r *http.Request) {
	nodeID:=strings.TrimSpace(r.Header.Get("X-VPNX3-Node-ID"))
	timestamp:=strings.TrimSpace(r.Header.Get("X-VPNX3-Timestamp"))
	signature:=strings.TrimSpace(r.Header.Get("X-VPNX3-Signature"))
	if nodeID=="" || timestamp=="" || signature=="" {
		writeError(w,http.StatusUnauthorized,"missing_probe_signature")
		return
	}

	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,256<<10))
	if err!=nil { writeError(w,http.StatusBadRequest,"invalid_body"); return }

	state,err:=s.store.ProbeAuthState(r.Context(),nodeID)
	if err!=nil || state.Role!="probe" || state.Status=="retired" || state.Status=="destroyed" || state.Status=="quarantined" {
		writeError(w,http.StatusUnauthorized,"invalid_probe")
		return
	}
	if err:=nodeauth.Verify(state.PublicKey,r.Method,r.URL.Path,timestamp,signature,body,time.Now().UTC()); err!=nil {
		writeError(w,http.StatusUnauthorized,"invalid_probe_signature")
		return
	}

	var req struct {
		Sequence int64                    `json:"sequence"`
		Results  []store.ProbeObservation `json:"results"`
	}
	decoder:=json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&req); err!=nil || req.Sequence<=state.Sequence || len(req.Results)>500 {
		writeError(w,http.StatusBadRequest,"invalid_probe_report")
		return
	}
	if err:=s.store.RecordProbeReport(r.Context(),nodeID,req.Sequence,req.Results); err!=nil {
		if strings.Contains(err.Error(),"stale") {
			writeError(w,http.StatusConflict,"stale_probe_report")
			return
		}
		s.internalError(w,r,err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
