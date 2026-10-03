package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/deviceauth"
)

type signedDeviceSequenceRequest struct {
	Sequence int64 `json:"sequence"`
}

func (s *Server) verifyDeviceJSONRequest(w http.ResponseWriter,r *http.Request,body []byte,sequence int64)(string,bool){
	deviceID:=strings.TrimSpace(r.Header.Get("X-VPNX3-Device-ID"))
	ts:=strings.TrimSpace(r.Header.Get("X-VPNX3-Timestamp"))
	sig:=strings.TrimSpace(r.Header.Get("X-VPNX3-Signature"))
	state,err:=s.store.DeviceAuthState(r.Context(),deviceID)
	if err!=nil||state.Status!="active"{writeError(w,http.StatusUnauthorized,"invalid_device");return "",false}
	if sequence<=state.Sequence{writeError(w,http.StatusConflict,"stale_request");return "",false}
	if err:=deviceauth.Verify(state.IdentityAlgorithm,state.PublicKey,r.Method,r.URL.Path,ts,sig,body,time.Now().UTC());err!=nil{
		writeError(w,http.StatusUnauthorized,"invalid_device_signature");return "",false
	}
	if err:=s.store.AdvanceDeviceSequence(r.Context(),deviceID,sequence);err!=nil{
		writeError(w,http.StatusConflict,"stale_request");return "",false
	}
	return deviceID,true
}

func (s *Server) handleClientAccountStatus(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,16<<10));if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req signedDeviceSequenceRequest
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0{writeError(w,http.StatusBadRequest,"invalid_request");return}
	deviceID,ok:=s.verifyDeviceJSONRequest(w,r,body,req.Sequence);if !ok{return}
	status,err:=s.store.ClientAccountStatus(r.Context(),deviceID,time.Now().UTC())
	if err!=nil{
		if strings.Contains(err.Error(),"no active entitlement"){writeError(w,http.StatusPaymentRequired,"no_active_entitlement");return}
		s.internalError(w,r,err);return
	}
	writeJSON(w,http.StatusOK,status)
}

func (s *Server) handleCreatePairingCode(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,16<<10));if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req signedDeviceSequenceRequest
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0{writeError(w,http.StatusBadRequest,"invalid_request");return}
	deviceID,ok:=s.verifyDeviceJSONRequest(w,r,body,req.Sequence);if !ok{return}
	code,expires,err:=s.store.CreateDevicePairingCode(r.Context(),deviceID,10*time.Minute)
	if err!=nil{
		if strings.Contains(err.Error(),"device limit reached"){writeError(w,http.StatusConflict,"device_limit_reached");return}
		s.internalError(w,r,err);return
	}
	writeJSON(w,http.StatusCreated,map[string]any{"code":code,"expires_at":expires})
}

func (s *Server) handleClaimPairingCode(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,16<<10));if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req struct{
		Sequence int64 `json:"sequence"`
		Code string `json:"code"`
	}
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0||strings.TrimSpace(req.Code)==""{writeError(w,http.StatusBadRequest,"invalid_request");return}
	deviceID,ok:=s.verifyDeviceJSONRequest(w,r,body,req.Sequence);if !ok{return}
	status,err:=s.store.ClaimDevicePairingCode(r.Context(),deviceID,req.Code,time.Now().UTC())
	if err!=nil{
		switch{
		case strings.Contains(err.Error(),"expired or invalid"):writeError(w,http.StatusNotFound,"pairing_code_invalid")
		case strings.Contains(err.Error(),"device limit reached"):writeError(w,http.StatusConflict,"device_limit_reached")
		case strings.Contains(err.Error(),"cannot be merged"):writeError(w,http.StatusConflict,"device_account_not_mergeable")
		case strings.Contains(err.Error(),"already belongs"):writeError(w,http.StatusConflict,"already_paired")
		default:s.internalError(w,r,err)
		}
		return
	}
	writeJSON(w,http.StatusOK,status)
}
