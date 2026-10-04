package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accesslease"
	"github.com/venomimonstro/vpnx3/internal/nodeauth"
)

func (s *Server) handleProbeLease(w http.ResponseWriter,r *http.Request){
	nodeID:=strings.TrimSpace(r.Header.Get("X-VPNX3-Node-ID"))
	timestamp:=strings.TrimSpace(r.Header.Get("X-VPNX3-Timestamp"))
	signature:=strings.TrimSpace(r.Header.Get("X-VPNX3-Signature"))
	if nodeID==""||timestamp==""||signature==""{
		writeError(w,http.StatusUnauthorized,"missing_probe_signature")
		return
	}

	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,64<<10))
	if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}

	var req struct{
		Sequence int64 `json:"sequence"`
		TunnelPublicKey string `json:"tunnel_public_key"`
	}
	dec:=json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0||strings.TrimSpace(req.TunnelPublicKey)==""||len(req.TunnelPublicKey)>128{
		writeError(w,http.StatusBadRequest,"invalid_probe_lease_request")
		return
	}

	state,err:=s.store.ProbeLeaseAuthState(r.Context(),nodeID)
	if err!=nil||state.Role!="probe"||state.Status!="active"{
		writeError(w,http.StatusUnauthorized,"invalid_probe")
		return
	}
	if req.Sequence<=state.Sequence{
		writeError(w,http.StatusConflict,"stale_probe_lease")
		return
	}
	if err:=nodeauth.Verify(state.PublicKey,r.Method,r.URL.Path,timestamp,signature,body,time.Now().UTC());err!=nil{
		writeError(w,http.StatusUnauthorized,"invalid_probe_signature")
		return
	}
	if err:=s.store.AdvanceProbeLeaseSequence(r.Context(),nodeID,req.Sequence);err!=nil{
		writeError(w,http.StatusConflict,"stale_probe_lease")
		return
	}

	rawID:=make([]byte,16)
	if _,err:=rand.Read(rawID);err!=nil{s.internalError(w,r,err);return}
	now:=time.Now().UTC().Truncate(time.Second)
	env,err:=accesslease.Issue(s.accessSigner,accesslease.Claims{
		SchemaVersion:1,
		LeaseID:hex.EncodeToString(rawID),
		UserID:"probe:"+nodeID,
		DeviceID:"probe:"+nodeID,
		Entitlement:"synthetic_probe",
		TunnelPublicKey:strings.TrimSpace(req.TunnelPublicKey),
		IssuedAt:now,
		ExpiresAt:now.Add(2*time.Minute),
	})
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,env)
}
