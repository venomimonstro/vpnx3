package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/deviceauth"
)

func (s *Server) handleCreateDeviceLinkCode(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,8<<10))
	if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req struct{Sequence int64 `json:"sequence"`}
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0{writeError(w,http.StatusBadRequest,"invalid_json");return}

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

	link,err:=s.store.CreateDeviceLinkCode(r.Context(),deviceID,time.Now().UTC())
	if err!=nil{
		if strings.Contains(err.Error(),"limit")||strings.Contains(err.Error(),"additional"){
			writeJSON(w,http.StatusConflict,map[string]string{"error":"device_link_not_allowed","detail":err.Error()});return
		}
		s.internalError(w,r,err);return
	}
	writeJSON(w,http.StatusCreated,map[string]any{"code":link.Code,"expires_at":link.ExpiresAt})
}

func (s *Server) handleRegisterLinkedDevice(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,256<<10))
	if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req struct{
		LinkCode string `json:"link_code"`
		Platform string `json:"platform"`
		DisplayName string `json:"display_name"`
		IdentityAlgorithm string `json:"identity_algorithm"`
		PublicKey string `json:"public_key"`
	}
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil{writeError(w,http.StatusBadRequest,"invalid_json");return}
	pub,err:=base64.RawURLEncoding.DecodeString(req.PublicKey)
	if err!=nil||deviceauth.ValidatePublicKey(req.IdentityAlgorithm,pub)!=nil{
		writeError(w,http.StatusBadRequest,"invalid_public_key");return
	}
	ts:=strings.TrimSpace(r.Header.Get("X-VPNX3-Timestamp"))
	sig:=strings.TrimSpace(r.Header.Get("X-VPNX3-Signature"))
	if err:=deviceauth.Verify(req.IdentityAlgorithm,pub,r.Method,r.URL.Path,ts,sig,body,time.Now().UTC());err!=nil{
		writeError(w,http.StatusUnauthorized,"invalid_device_signature");return
	}
	reg,err:=s.store.RegisterLinkedDevice(
		r.Context(),req.LinkCode,req.Platform,req.DisplayName,req.IdentityAlgorithm,pub,time.Now().UTC(),
	)
	if err!=nil{
		if strings.Contains(err.Error(),"code")||strings.Contains(err.Error(),"limit")||strings.Contains(err.Error(),"additional"){
			writeJSON(w,http.StatusConflict,map[string]string{"error":"device_link_failed","detail":err.Error()});return
		}
		s.internalError(w,r,err);return
	}
	writeJSON(w,http.StatusCreated,map[string]any{
		"user_id":reg.UserID,"device_id":reg.DeviceID,"trial_expires_at":"",
	})
}
