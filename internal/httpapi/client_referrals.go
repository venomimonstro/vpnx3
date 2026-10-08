package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleReferralCode(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,16<<10));if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req signedDeviceSequenceRequest
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0{writeError(w,http.StatusBadRequest,"invalid_request");return}
	deviceID,ok:=s.verifyDeviceJSONRequest(w,r,body,req.Sequence);if !ok{return}
	code,err:=s.store.EnsureReferralCode(r.Context(),deviceID)
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,map[string]any{"code":code})
}

func (s *Server) handleReferralClaim(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,16<<10));if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req struct{
		Sequence int64 `json:"sequence"`
		Code string `json:"code"`
	}
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0||strings.TrimSpace(req.Code)==""{writeError(w,http.StatusBadRequest,"invalid_request");return}
	deviceID,ok:=s.verifyDeviceJSONRequest(w,r,body,req.Sequence);if !ok{return}
	result,err:=s.store.ClaimReferralCode(r.Context(),deviceID,req.Code,time.Now().UTC())
	if err!=nil{
		msg:=err.Error()
		switch{
		case strings.Contains(msg,"not found"):writeError(w,http.StatusNotFound,"referral_code_not_found")
		case strings.Contains(msg,"already claimed"):writeError(w,http.StatusConflict,"referral_already_claimed")
		case strings.Contains(msg,"self referral"):writeError(w,http.StatusConflict,"self_referral_not_allowed")
		case strings.Contains(msg,"before first payment"):writeError(w,http.StatusConflict,"referral_after_payment_not_allowed")
		case strings.Contains(msg,"window expired"):writeError(w,http.StatusConflict,"referral_claim_window_expired")
		default:s.internalError(w,r,err)
		}
		return
	}
	writeJSON(w,http.StatusOK,result)
}

func (s *Server) handleReferralStatus(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,16<<10));if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req signedDeviceSequenceRequest
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0{writeError(w,http.StatusBadRequest,"invalid_request");return}
	deviceID,ok:=s.verifyDeviceJSONRequest(w,r,body,req.Sequence);if !ok{return}
	status,err:=s.store.ReferralStatusForDevice(r.Context(),deviceID)
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,status)
}
