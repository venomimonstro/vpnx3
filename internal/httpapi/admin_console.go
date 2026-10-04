package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

func pageParams(r *http.Request) (int,int) {
	limit:=100
	offset:=0
	if raw:=r.URL.Query().Get("limit"); raw!="" {
		if v,err:=strconv.Atoi(raw); err==nil { limit=v }
	}
	if raw:=r.URL.Query().Get("offset"); raw!="" {
		if v,err:=strconv.Atoi(raw); err==nil { offset=v }
	}
	return limit,offset
}

func (s *Server) handleDashboard(w http.ResponseWriter,r *http.Request) {
	summary,err:=s.store.Dashboard(r.Context())
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,summary)
}

func (s *Server) handleUsers(w http.ResponseWriter,r *http.Request) {
	limit,offset:=pageParams(r)
	users,err:=s.store.ListUsers(r.Context(),limit,offset)
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"users":users})
}

func (s *Server) handleUserDetail(w http.ResponseWriter,r *http.Request) {
	user,err:=s.store.AdminUserDetail(r.Context(),r.PathValue("id"))
	if err==store.ErrNotFound{writeError(w,http.StatusNotFound,"user_not_found");return}
	if err!=nil{s.internalError(w,r,err);return}
	devices,err:=s.store.UserDevices(r.Context(),user.ID)
	if err!=nil{s.internalError(w,r,err);return}
	payments,err:=s.store.UserPayments(r.Context(),user.ID,20)
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,map[string]any{"user":user,"devices":devices,"payments":payments})
}

func (s *Server) handleUserDevices(w http.ResponseWriter,r *http.Request) {
	devices,err:=s.store.UserDevices(r.Context(),r.PathValue("id"))
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"devices":devices})
}

func (s *Server) handlePayments(w http.ResponseWriter,r *http.Request) {
	limit,offset:=pageParams(r)
	payments,err:=s.store.ListPayments(r.Context(),limit,offset)
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"payments":payments})
}

func (s *Server) handleAudit(w http.ResponseWriter,r *http.Request) {
	limit,offset:=pageParams(r)
	rows,err:=s.store.ListAudit(r.Context(),limit,offset)
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"events":rows})
}

func (s *Server) handleIncidents(w http.ResponseWriter,r *http.Request) {
	limit,_:=pageParams(r)
	rows,err:=s.store.ListIncidents(r.Context(),limit)
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"incidents":rows})
}

func (s *Server) handleCreateIncident(w http.ResponseWriter,r *http.Request) {
	var req struct {
		Severity string `json:"severity"`
		Title string `json:"title"`
		Summary string `json:"summary"`
	}
	if err:=decodeJSON(w,r,&req); err!=nil { return }
	req.Severity=strings.TrimSpace(strings.ToLower(req.Severity))
	req.Title=strings.TrimSpace(req.Title)
	req.Summary=strings.TrimSpace(req.Summary)
	if req.Title=="" || req.Summary=="" || len(req.Title)>200 || len(req.Summary)>4000 ||
		(req.Severity!="info" && req.Severity!="warning" && req.Severity!="critical") {
		writeError(w,http.StatusBadRequest,"invalid_incident")
		return
	}
	incident,err:=s.store.CreateIncident(r.Context(),req.Severity,req.Title,req.Summary)
	if err!=nil { s.internalError(w,r,err); return }
	admin,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"incident.create","incident",incident.ID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	writeJSON(w,http.StatusCreated,incident)
}

func (s *Server) handleResolveIncident(w http.ResponseWriter,r *http.Request) {
	var req struct{ RootCause string `json:"root_cause"` }
	if r.ContentLength!=0 {
		if err:=decodeJSON(w,r,&req); err!=nil { return }
	}
	incident,err:=s.store.ResolveIncident(r.Context(),r.PathValue("id"),strings.TrimSpace(req.RootCause))
	if err!=nil { s.internalError(w,r,err); return }
	admin,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"incident.resolve","incident",incident.ID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	writeJSON(w,http.StatusOK,incident)
}


func (s *Server) handleRevokeUserDevice(w http.ResponseWriter,r *http.Request) {
	userID:=r.PathValue("id")
	deviceID:=r.PathValue("deviceId")
	err:=s.store.RevokeUserDevice(r.Context(),userID,deviceID)
	if err!=nil{
		if err.Error()=="not found"{writeError(w,http.StatusNotFound,"device_not_found");return}
		if strings.Contains(err.Error(),"not active"){writeError(w,http.StatusConflict,"device_not_active");return}
		s.internalError(w,r,err);return
	}
	admin,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"device.revoke","device",deviceID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReactivateUserDevice(w http.ResponseWriter,r *http.Request) {
	userID:=r.PathValue("id")
	deviceID:=r.PathValue("deviceId")
	if err:=s.store.ReactivateUserDevice(r.Context(),userID,deviceID);err!=nil{
		if err==store.ErrNotFound{writeError(w,http.StatusNotFound,"device_not_found");return}
		if strings.Contains(err.Error(),"device limit reached"){writeError(w,http.StatusConflict,"device_limit_reached");return}
		if strings.Contains(err.Error(),"not revoked")||strings.Contains(err.Error(),"user is not active"){
			writeError(w,http.StatusConflict,"device_reactivation_rejected");return
		}
		s.internalError(w,r,err);return
	}
	admin,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"device.reactivate","device",deviceID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	w.WriteHeader(http.StatusNoContent)
}
