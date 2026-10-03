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
		admin, err := s.store.AdminBySessionHash(r.Context(), hash)
		if errors.Is(err, store.ErrNotFound) { writeError(w, http.StatusUnauthorized, "unauthorized"); return }
		if err != nil { s.internalError(w, r, err); return }
		s.store.TouchAdminSession(r.Context(), hash)
		ctx := context.WithValue(r.Context(), currentAdminKey, admin)
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
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil { return ip }
	}
	return net.ParseIP(r.RemoteAddr)
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
