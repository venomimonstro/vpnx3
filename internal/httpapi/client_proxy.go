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

	"github.com/venomimonstro/vpnx3/internal/deviceauth"
	"github.com/venomimonstro/vpnx3/internal/proxylease"
)

func (s *Server) handleClientProxyLease(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,16<<10))
	if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req struct{Sequence int64 `json:"sequence"`}
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0{writeError(w,http.StatusBadRequest,"invalid_proxy_lease_request");return}

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
	now:=time.Now().UTC().Truncate(time.Second)
	allowed,_,rateErr:=s.store.AllowClientLease(r.Context(),deviceID,"proxy",now,30)
	if rateErr!=nil{s.internalError(w,r,rateErr);return}
	if !allowed{
		_ = s.store.IncrementSecurityCounter(r.Context(),"proxy_lease_rate_limited")
		w.Header().Set("Retry-After","60")
		writeError(w,http.StatusTooManyRequests,"proxy_lease_rate_limited")
		return
	}
	ent,err:=s.store.DeviceEntitlement(r.Context(),deviceID,now)
	if err!=nil{writeError(w,http.StatusPaymentRequired,"no_active_entitlement");return}
	expires:=now.Add(s.cfg.AccessLeaseTTL)
	if ent.ExpiresAt.Before(expires){expires=ent.ExpiresAt}
	id:=make([]byte,16)
	if _,err:=rand.Read(id);err!=nil{s.internalError(w,r,err);return}
	env,err:=proxylease.Issue(s.accessSigner,proxylease.Claims{
		SchemaVersion:1,LeaseID:hex.EncodeToString(id),UserID:state.UserID,DeviceID:state.DeviceID,
		Entitlement:ent.Name,IssuedAt:now,ExpiresAt:expires,
	})
	if err!=nil{s.internalError(w,r,err);return}
	credential,err:=proxylease.EncodeCredential(env)
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,map[string]any{"credential":credential,"expires_at":expires})
}

func (s *Server) handleAccessSigningKey(w http.ResponseWriter,r *http.Request){
	writeJSON(w,http.StatusOK,map[string]string{
		"key_id":s.accessSigner.KeyID(),
		"public_key":s.accessSigner.PublicKeyBase64(),
		"algorithm":"ed25519",
	})
}
