package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/deviceauth"
	"github.com/venomimonstro/vpnx3/internal/store"
)

func (s *Server) handleClientTelemetry(w http.ResponseWriter,r *http.Request) {
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,32<<10))
	if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req struct{
		Sequence int64 `json:"sequence"`
		Platform string `json:"platform"`
		ClientVersion string `json:"client_version"`
		EventType string `json:"event_type"`
		ConfigVersion int64 `json:"config_version"`
		WorkerNodeID string `json:"worker_node_id"`
		NetworkType string `json:"network_type"`
		DurationMS int64 `json:"duration_ms"`
	}
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0{writeError(w,http.StatusBadRequest,"invalid_telemetry");return}

	deviceID:=strings.TrimSpace(r.Header.Get("X-VPNX3-Device-ID"))
	ts:=strings.TrimSpace(r.Header.Get("X-VPNX3-Timestamp"))
	sig:=strings.TrimSpace(r.Header.Get("X-VPNX3-Signature"))
	state,err:=s.store.DeviceAuthState(r.Context(),deviceID)
	if err!=nil||state.Status!="active"{writeError(w,http.StatusUnauthorized,"invalid_device");return}
	if req.Sequence<=state.Sequence{writeError(w,http.StatusConflict,"stale_request");return}
	if err:=deviceauth.Verify(state.IdentityAlgorithm,state.PublicKey,r.Method,r.URL.Path,ts,sig,body,time.Now().UTC());err!=nil{
		writeError(w,http.StatusUnauthorized,"invalid_device_signature");return
	}
	if err:=s.store.AdvanceDeviceSequence(r.Context(),deviceID,req.Sequence);err!=nil{
		writeError(w,http.StatusConflict,"stale_request");return
	}

	err=s.store.RecordClientTelemetry(r.Context(),store.ClientTelemetryEvent{
		Platform:req.Platform,ClientVersion:req.ClientVersion,EventType:req.EventType,
		ConfigVersion:req.ConfigVersion,WorkerNodeID:req.WorkerNodeID,NetworkType:req.NetworkType,
		DurationMS:req.DurationMS,
	})
	if err!=nil{
		if strings.Contains(err.Error(),"invalid telemetry"){writeError(w,http.StatusBadRequest,"invalid_telemetry");return}
		s.internalError(w,r,err);return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTelemetrySummary(w http.ResponseWriter,r *http.Request) {
	days:=14
	if raw:=strings.TrimSpace(r.URL.Query().Get("days"));raw!=""{
		var parsed int
		if _,err:=fmt.Sscanf(raw,"%d",&parsed);err==nil{days=parsed}
	}
	rows,err:=s.store.TelemetrySummary(r.Context(),days)
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,map[string]any{"metrics":rows})
}
