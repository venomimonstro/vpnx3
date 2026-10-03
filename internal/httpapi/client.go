package httpapi

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accesslease"
	"github.com/venomimonstro/vpnx3/internal/nodeauth"
	"github.com/venomimonstro/vpnx3/internal/store"
)

func (s *Server) handleClientRegister(w http.ResponseWriter,r *http.Request) {
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,256<<10))
	if err!=nil { writeError(w,http.StatusBadRequest,"invalid_body"); return }

	var req struct {
		Platform string `json:"platform"`
		DisplayName string `json:"display_name"`
		PublicKey string `json:"public_key"`
	}
	decoder:=json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&req); err!=nil { writeError(w,http.StatusBadRequest,"invalid_json"); return }

	pub,err:=base64.RawURLEncoding.DecodeString(req.PublicKey)
	if err!=nil || len(pub)!=ed25519.PublicKeySize { writeError(w,http.StatusBadRequest,"invalid_public_key"); return }
	ts:=strings.TrimSpace(r.Header.Get("X-VPNX3-Timestamp"))
	sig:=strings.TrimSpace(r.Header.Get("X-VPNX3-Signature"))
	if ts=="" || sig=="" { writeError(w,http.StatusUnauthorized,"missing_device_signature"); return }
	if err:=nodeauth.Verify(pub,r.Method,r.URL.Path,ts,sig,body,time.Now().UTC()); err!=nil {
		writeError(w,http.StatusUnauthorized,"invalid_device_signature"); return
	}

	reg,err:=s.store.RegisterAnonymousDevice(r.Context(),req.Platform,req.DisplayName,pub,s.cfg.TrialDays)
	if err!=nil {
		if strings.Contains(err.Error(),"duplicate key") { writeError(w,http.StatusConflict,"device_already_registered"); return }
		if strings.Contains(err.Error(),"unsupported") || strings.Contains(err.Error(),"invalid") {
			writeError(w,http.StatusBadRequest,"invalid_device_data"); return
		}
		s.internalError(w,r,err); return
	}
	writeJSON(w,http.StatusCreated,map[string]any{
		"user_id":reg.UserID,
		"device_id":reg.DeviceID,
		"trial_expires_at":reg.TrialExpires,
	})
}

func (s *Server) handleClientLease(w http.ResponseWriter,r *http.Request) {
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,64<<10))
	if err!=nil { writeError(w,http.StatusBadRequest,"invalid_body"); return }
	var req struct{ Sequence int64 `json:"sequence"` }
	if err:=json.Unmarshal(body,&req); err!=nil || req.Sequence<=0 { writeError(w,http.StatusBadRequest,"invalid_json"); return }

	deviceID:=strings.TrimSpace(r.Header.Get("X-VPNX3-Device-ID"))
	ts:=strings.TrimSpace(r.Header.Get("X-VPNX3-Timestamp"))
	sig:=strings.TrimSpace(r.Header.Get("X-VPNX3-Signature"))
	if deviceID=="" || ts=="" || sig=="" { writeError(w,http.StatusUnauthorized,"missing_device_signature"); return }

	state,err:=s.store.DeviceAuthState(r.Context(),deviceID)
	if err!=nil || state.Status!="active" { writeError(w,http.StatusUnauthorized,"invalid_device"); return }
	if req.Sequence<=state.Sequence { writeError(w,http.StatusConflict,"stale_request"); return }
	if err:=nodeauth.Verify(state.PublicKey,r.Method,r.URL.Path,ts,sig,body,time.Now().UTC()); err!=nil {
		writeError(w,http.StatusUnauthorized,"invalid_device_signature"); return
	}
	if err:=s.store.AdvanceDeviceSequence(r.Context(),deviceID,req.Sequence); err!=nil {
		writeError(w,http.StatusConflict,"stale_request"); return
	}

	now:=time.Now().UTC().Truncate(time.Second)
	entitlement,err:=s.store.DeviceEntitlement(r.Context(),deviceID,now)
	if err!=nil { writeError(w,http.StatusPaymentRequired,"no_active_entitlement"); return }

	expires:=now.Add(s.cfg.AccessLeaseTTL)
	if entitlement.ExpiresAt.Before(expires) { expires=entitlement.ExpiresAt }
	leaseID:=make([]byte,16)
	if _,err:=rand.Read(leaseID); err!=nil { s.internalError(w,r,err); return }

	env,err:=accesslease.Issue(s.accessSigner,accesslease.Claims{
		SchemaVersion:1,
		LeaseID:hex.EncodeToString(leaseID),
		UserID:state.UserID,
		DeviceID:state.DeviceID,
		Entitlement:entitlement.Name,
		IssuedAt:now,
		ExpiresAt:expires,
	})
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,env)
}

func parseSequenceHeader(r *http.Request) (int64,error) {
	return strconv.ParseInt(strings.TrimSpace(r.Header.Get("X-VPNX3-Sequence")),10,64)
}
