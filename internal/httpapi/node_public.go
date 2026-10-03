package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/nodeauth"
	"github.com/venomimonstro/vpnx3/internal/store"
)

func (s *Server) handleNodeEnroll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token        string `json:"token"`
		Name         string `json:"name"`
		PublicKey    string `json:"public_key"`
		Provider     string `json:"provider"`
		CountryCode  string `json:"country_code"`
		AgentVersion string `json:"agent_version"`
		Capacity     int    `json:"capacity_sessions"`
		PublicIP     string `json:"public_ip"`
	}
	if err := decodeJSON(w,r,&req); err != nil { return }
	key, err := base64.RawURLEncoding.DecodeString(req.PublicKey)
	if err != nil || len(key) != 32 {
		writeError(w,http.StatusBadRequest,"invalid_public_key")
		return
	}
	node, err := s.store.EnrollNode(r.Context(),store.EnrollNodeInput{
		Token:req.Token,Name:req.Name,PublicKey:key,Provider:req.Provider,
		CountryCode:req.CountryCode,AgentVersion:req.AgentVersion,
		Capacity:req.Capacity,PublicIP:req.PublicIP,
	})
	if err != nil {
		switch {
		case strings.Contains(err.Error(),"invalid or expired"):
			writeError(w,http.StatusUnauthorized,"invalid_enrollment_token")
		case strings.Contains(err.Error(),"duplicate key"):
			writeError(w,http.StatusConflict,"node_already_exists")
		case strings.Contains(err.Error(),"country code"), strings.Contains(err.Error(),"node name"):
			writeError(w,http.StatusBadRequest,"invalid_node_data")
		default:
			s.internalError(w,r,err)
		}
		return
	}
	writeJSON(w,http.StatusCreated,map[string]any{
		"node_id":node.ID,
		"status":node.Status,
		"heartbeat_interval_seconds":30,
	})
}

func (s *Server) handleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	nodeID := strings.TrimSpace(r.Header.Get("X-VPNX3-Node-ID"))
	timestamp := strings.TrimSpace(r.Header.Get("X-VPNX3-Timestamp"))
	signature := strings.TrimSpace(r.Header.Get("X-VPNX3-Signature"))
	if nodeID == "" || timestamp == "" || signature == "" {
		writeError(w,http.StatusUnauthorized,"missing_node_signature")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w,r.Body,256<<10))
	if err != nil {
		writeError(w,http.StatusBadRequest,"invalid_body")
		return
	}
	publicKey,currentSeq,err := s.store.NodePublicKey(r.Context(),nodeID)
	if err != nil {
		writeError(w,http.StatusUnauthorized,"invalid_node")
		return
	}
	if err := nodeauth.Verify(publicKey,r.Method,r.URL.Path,timestamp,signature,body,time.Now().UTC()); err != nil {
		writeError(w,http.StatusUnauthorized,"invalid_node_signature")
		return
	}

	var req struct {
		Sequence        int64           `json:"sequence"`
		CurrentSessions int             `json:"current_sessions"`
		Capacity        int             `json:"capacity_sessions"`
		AgentVersion    string          `json:"agent_version"`
		HealthScore     *float64        `json:"health_score,omitempty"`
		Metadata        json.RawMessage `json:"metadata,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w,http.StatusBadRequest,"invalid_json")
		return
	}
	if req.Sequence <= 0 {
		writeError(w,http.StatusBadRequest,"invalid_sequence")
		return
	}
	if req.Sequence <= currentSeq {
		writeError(w,http.StatusConflict,"stale_heartbeat")
		return
	}
	if req.CurrentSessions < 0 || req.Capacity < 0 {
		writeError(w,http.StatusBadRequest,"invalid_metrics")
		return
	}
	if len(req.Metadata) == 0 { req.Metadata = json.RawMessage(`{}`) }
	if !json.Valid(req.Metadata) {
		writeError(w,http.StatusBadRequest,"invalid_metadata")
		return
	}
	if err := s.store.RecordHeartbeat(r.Context(),store.HeartbeatInput{
		NodeID:nodeID,Sequence:req.Sequence,CurrentSessions:req.CurrentSessions,
		Capacity:req.Capacity,AgentVersion:req.AgentVersion,
		HealthScore:req.HealthScore,Metadata:req.Metadata,
	}); err != nil {
		if strings.Contains(err.Error(),"stale heartbeat") {
			writeError(w,http.StatusConflict,"stale_heartbeat")
			return
		}
		s.internalError(w,r,err)
		return
	}
	if err := s.store.PromoteEnrolledNode(r.Context(),nodeID); err != nil {
		s.internalError(w,r,err)
		return
	}
	writeJSON(w,http.StatusOK,map[string]any{
		"status":"ok",
		"server_time":time.Now().UTC(),
	})
}
