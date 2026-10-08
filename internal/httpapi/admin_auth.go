package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/adminauth"
	"github.com/venomimonstro/vpnx3/internal/store"
)

type adminContextKey string

const currentAdminKey adminContextKey = "current_admin"

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil { return }

	email:=strings.ToLower(strings.TrimSpace(req.Email))
	ip:=clientIP(r)
	now:=time.Now().UTC()

	blocked,_,blockErr:=s.store.AdminLoginBlocked(r.Context(),email,ip,now)
	if blockErr!=nil { s.internalError(w,r,blockErr); return }
	if blocked {
		time.Sleep(250*time.Millisecond)
		writeError(w,http.StatusTooManyRequests,"login_rate_limited")
		return
	}

	admin, err := s.store.AdminByEmail(r.Context(), email)
	valid:=err==nil && admin.Status=="active" && adminauth.VerifyPassword(admin.PasswordHash,req.Password)
	if !valid {
		_ = s.store.RecordAdminLoginFailure(r.Context(),email,ip,now)
		time.Sleep(250*time.Millisecond)
		writeError(w,http.StatusUnauthorized,"invalid_credentials")
		return
	}
	s.store.ClearAdminLoginFailures(r.Context(),email,ip)

	plain, hash, err := adminauth.NewSessionToken()
	if err != nil { s.internalError(w, r, err); return }
	expires := time.Now().UTC().Add(s.cfg.AdminSessionTTL)
	if err := s.store.CreateAdminSession(r.Context(), admin.ID, hash, ip, r.UserAgent(), expires); err != nil {
		s.internalError(w, r, err)
		return
	}
	_ = s.store.WriteAudit(r.Context(), "admin", admin.ID, "admin.login", "admin_session", "", requestIDFromContext(r.Context()), ipString(ip), "success")

	writeJSON(w, http.StatusOK, map[string]any{
		"token": plain,
		"expires_at": expires,
		"admin": map[string]any{
			"id": admin.ID,
			"email": admin.Email,
			"permissions": admin.Permissions,
		},
	})
}

func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	admin, ok := adminFromContext(r.Context())
	if !ok { writeError(w, http.StatusUnauthorized, "unauthorized"); return }
	writeJSON(w, http.StatusOK, map[string]any{
		"id": admin.ID,
		"email": admin.Email,
		"permissions": admin.Permissions,
	})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" { writeError(w, http.StatusUnauthorized, "unauthorized"); return }
	admin, _ := adminFromContext(r.Context())
	hash := adminauth.HashSessionToken(token)
	if err := s.store.RevokeAdminSession(r.Context(), hash); err != nil {
		s.internalError(w, r, err); return
	}
	ip := clientIP(r)
	_ = s.store.WriteAudit(r.Context(), "admin", admin.ID, "admin.logout", "admin_session", "", requestIDFromContext(r.Context()), ipString(ip), "success")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" { writeError(w, http.StatusUnauthorized, "unauthorized"); return }
		hash := adminauth.HashSessionToken(token)
		session,err:=s.store.AdminSessionByHash(r.Context(),hash)
		if errors.Is(err,store.ErrNotFound){writeError(w,http.StatusUnauthorized,"unauthorized");return}
		if err!=nil{s.internalError(w,r,err);return}
		if !adminSessionContextMatches(s.cfg.AdminSessionBinding,session.SourceIP,session.UserAgent,clientIP(r),r.UserAgent()){
			_ = s.store.RevokeAdminSession(r.Context(),hash)
			_ = s.store.WriteAudit(r.Context(),"admin",session.Admin.ID,"admin.session.context_mismatch","admin_session","",
				requestIDFromContext(r.Context()),ipString(clientIP(r)),"revoked")
			writeError(w,http.StatusUnauthorized,"session_context_changed")
			return
		}
		s.store.TouchAdminSession(r.Context(),hash)
		ctx:=context.WithValue(r.Context(),currentAdminKey,session.Admin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requirePermission(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admin, ok := adminFromContext(r.Context())
		if !ok { writeError(w, http.StatusUnauthorized, "unauthorized"); return }
		for _, p := range admin.Permissions {
			if p == permission { next.ServeHTTP(w, r); return }
		}
		writeError(w, http.StatusForbidden, "forbidden")
	})
}

func adminFromContext(ctx context.Context) (store.Admin, bool) {
	admin, ok := ctx.Value(currentAdminKey).(store.Admin)
	return admin, ok
}

func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") { return "" }
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func clientIP(r *http.Request) net.IP {
	if ip,ok:=r.Context().Value(clientIPKey).(net.IP);ok && ip!=nil {
		return ip
	}
	return remoteIP(r.RemoteAddr)
}

func ipString(ip net.IP) string {
	if ip==nil { return "" }
	return ip.String()
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}


func adminSessionContextMatches(mode string,storedIP net.IP,storedUA string,currentIP net.IP,currentUA string) bool {
	if mode=="off"{return true}
	if strings.TrimSpace(storedUA)!=strings.TrimSpace(currentUA){return false}
	if mode=="user-agent"{return true}
	if mode!="network"{return false}
	if storedIP==nil||currentIP==nil{return false}
	return sameAdminNetwork(storedIP,currentIP)
}

func sameAdminNetwork(a,b net.IP) bool {
	if av4:=a.To4();av4!=nil{
		bv4:=b.To4()
		return bv4!=nil&&av4[0]==bv4[0]&&av4[1]==bv4[1]&&av4[2]==bv4[2]
	}
	av6:=a.To16()
	bv6:=b.To16()
	if av6==nil||bv6==nil||b.To4()!=nil{return false}
	for i:=0;i<8;i++{if av6[i]!=bv6[i]{return false}}
	return true
}
