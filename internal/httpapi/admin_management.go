package httpapi

import (
	"net/http"
	"strings"

	"github.com/venomimonstro/vpnx3/internal/adminauth"
	"github.com/venomimonstro/vpnx3/internal/store"
)

func (s *Server) handleListAdmins(w http.ResponseWriter,r *http.Request) {
	rows,err:=s.store.ListAdmins(r.Context())
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,map[string]any{"admins":rows})
}

func (s *Server) handleCreateAdmin(w http.ResponseWriter,r *http.Request) {
	var req struct{
		Email string `json:"email"`
		Password string `json:"password"`
		Roles []string `json:"roles"`
	}
	if err:=decodeJSON(w,r,&req);err!=nil{return}
	hash,err:=adminauth.HashPassword(req.Password)
	if err!=nil{writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_password","detail":err.Error()});return}
	admin,err:=s.store.CreateAdmin(r.Context(),req.Email,hash,req.Roles)
	if err!=nil{
		if strings.Contains(err.Error(),"invalid")||strings.Contains(err.Error(),"unknown"){writeError(w,http.StatusBadRequest,"invalid_admin");return}
		s.internalError(w,r,err);return
	}
	actor,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(),"admin",actor.ID,"admin.create","admin_user",admin.ID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	writeJSON(w,http.StatusCreated,admin)
}

func (s *Server) handleSetAdminRoles(w http.ResponseWriter,r *http.Request) {
	var req struct{ Roles []string `json:"roles"` }
	if err:=decodeJSON(w,r,&req);err!=nil{return}
	actor,_:=adminFromContext(r.Context())
	admin,err:=s.store.SetAdminRoles(r.Context(),r.PathValue("id"),actor.ID,req.Roles)
	if err!=nil{
		if err==store.ErrNotFound{writeError(w,http.StatusNotFound,"admin_not_found");return}
		if strings.Contains(err.Error(),"cannot")||strings.Contains(err.Error(),"required")||strings.Contains(err.Error(),"unknown"){
			writeJSON(w,http.StatusConflict,map[string]string{"error":"admin_role_change_rejected","detail":err.Error()});return
		}
		s.internalError(w,r,err);return
	}
	_ = s.store.WriteAudit(r.Context(),"admin",actor.ID,"admin.roles.update","admin_user",admin.ID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	writeJSON(w,http.StatusOK,admin)
}

func (s *Server) handleSetAdminStatus(w http.ResponseWriter,r *http.Request) {
	var req struct{ Status string `json:"status"` }
	if err:=decodeJSON(w,r,&req);err!=nil{return}
	actor,_:=adminFromContext(r.Context())
	admin,err:=s.store.SetAdminStatus(r.Context(),r.PathValue("id"),actor.ID,strings.TrimSpace(strings.ToLower(req.Status)))
	if err!=nil{
		if err==store.ErrNotFound{writeError(w,http.StatusNotFound,"admin_not_found");return}
		if strings.Contains(err.Error(),"cannot")||strings.Contains(err.Error(),"invalid")||strings.Contains(err.Error(),"already"){
			writeJSON(w,http.StatusConflict,map[string]string{"error":"admin_status_change_rejected","detail":err.Error()});return
		}
		s.internalError(w,r,err);return
	}
	_ = s.store.WriteAudit(r.Context(),"admin",actor.ID,"admin.status.update","admin_user",admin.ID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	writeJSON(w,http.StatusOK,admin)
}
