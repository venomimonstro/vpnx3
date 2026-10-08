package httpapi

import (
	"net/http"
	"strconv"
)

func (s *Server) handleIncidentNotificationStatus(w http.ResponseWriter,r *http.Request){
	status,err:=s.store.IncidentNotificationStatus(r.Context())
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,map[string]any{
		"configured":s.cfg.AlertWebhookURL!="",
		"status":status,
	})
}

func (s *Server) handleRequeueIncidentNotifications(w http.ResponseWriter,r *http.Request){
	limit:=100
	if raw:=r.URL.Query().Get("limit");raw!=""{
		value,err:=strconv.Atoi(raw)
		if err!=nil||value<1||value>500{
			writeError(w,http.StatusBadRequest,"invalid_limit")
			return
		}
		limit=value
	}
	count,err:=s.store.RequeueDeadIncidentNotifications(r.Context(),limit)
	if err!=nil{s.internalError(w,r,err);return}
	admin,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(
		r.Context(),"admin",admin.ID,"incident_notification.requeue",
		"incident_notification","dead-letter",requestIDFromContext(r.Context()),
		ipString(clientIP(r)),"success",
	)
	writeJSON(w,http.StatusOK,map[string]any{"requeued":count})
}
