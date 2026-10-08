package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/venomimonstro/vpnx3/internal/store"
)

func (s *Server) handleClientDevices(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,16<<10));if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req signedDeviceSequenceRequest
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0{writeError(w,http.StatusBadRequest,"invalid_request");return}
	deviceID,ok:=s.verifyDeviceJSONRequest(w,r,body,req.Sequence);if !ok{return}
	devices,err:=s.store.ClientDevices(r.Context(),deviceID)
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,map[string]any{"devices":devices})
}

func (s *Server) handleClientRevokeDevice(w http.ResponseWriter,r *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,16<<10));if err!=nil{writeError(w,http.StatusBadRequest,"invalid_body");return}
	var req struct{
		Sequence int64 `json:"sequence"`
		DeviceID string `json:"device_id"`
	}
	dec:=json.NewDecoder(bytes.NewReader(body));dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil||req.Sequence<=0||strings.TrimSpace(req.DeviceID)==""{
		writeError(w,http.StatusBadRequest,"invalid_request");return
	}
	currentID,ok:=s.verifyDeviceJSONRequest(w,r,body,req.Sequence);if !ok{return}
	targetID:=strings.TrimSpace(req.DeviceID)
	if err:=s.store.RevokeOwnPeerDevice(r.Context(),currentID,targetID);err!=nil{
		switch{
		case err==store.ErrNotFound:
			writeError(w,http.StatusNotFound,"device_not_found")
		case strings.Contains(err.Error(),"current device"):
			writeError(w,http.StatusConflict,"cannot_revoke_current_device")
		case strings.Contains(err.Error(),"not active"):
			writeError(w,http.StatusConflict,"device_not_active")
		default:s.internalError(w,r,err)
		}
		return
	}
	_ = s.store.WriteAudit(
		r.Context(),"device",currentID,"device.self_service.revoke","device",targetID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success",
	)
	w.WriteHeader(http.StatusNoContent)
}
