package workerauth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accesslease"
	"github.com/venomimonstro/vpnx3/internal/sessions"
)

type Server struct {
	http *http.Server
	logger *slog.Logger
	verifier *accesslease.Verifier
	sessions *sessions.Manager
}

func New(addr string,logger *slog.Logger,verifier *accesslease.Verifier,manager *sessions.Manager) *Server {
	s:=&Server{logger:logger,verifier:verifier,sessions:manager}
	mux:=http.NewServeMux()
	mux.HandleFunc("GET /health/live",func(w http.ResponseWriter,r *http.Request){
		writeJSON(w,http.StatusOK,map[string]any{"status":"ok","service":"vpn-worker","sessions":manager.Count()})
	})
	mux.HandleFunc("POST /internal/v1/authorize",s.handleAuthorize)
	mux.HandleFunc("POST /internal/v1/sessions",s.handleCreateSession)
	mux.HandleFunc("DELETE /internal/v1/sessions/{id}",s.handleDeleteSession)
	s.http=&http.Server{
		Addr:addr,Handler:mux,ReadHeaderTimeout:3*time.Second,
		ReadTimeout:5*time.Second,WriteTimeout:5*time.Second,IdleTimeout:30*time.Second,
	}
	return s
}

func (s *Server) handleAuthorize(w http.ResponseWriter,r *http.Request) {
	var env accesslease.Envelope
	decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,64<<10))
	decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&env); err!=nil { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_lease"}); return }
	claims,err:=s.verifier.Verify(env,time.Now().UTC())
	if err!=nil { s.logger.Warn("access lease rejected","error",err); writeJSON(w,http.StatusUnauthorized,map[string]string{"error":"access_denied"}); return }
	writeJSON(w,http.StatusOK,map[string]any{
		"authorized":true,"user_id":claims.UserID,"device_id":claims.DeviceID,
		"entitlement":claims.Entitlement,"expires_at":claims.ExpiresAt,
	})
}

func (s *Server) handleCreateSession(w http.ResponseWriter,r *http.Request) {
	var req struct {
		Lease accesslease.Envelope `json:"lease"`
		ClientPublicKey string `json:"client_public_key"`
	}
	decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,128<<10))
	decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&req); err!=nil { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_request"}); return }
	ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second)
	defer cancel()
	session,err:=s.sessions.Start(ctx,req.Lease,req.ClientPublicKey,time.Now().UTC())
	if err!=nil {
		s.logger.Warn("vpn session rejected","error",err)
		writeJSON(w,http.StatusUnauthorized,map[string]string{"error":"session_rejected"})
		return
	}
	writeJSON(w,http.StatusCreated,session)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter,r *http.Request) {
	ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second)
	defer cancel()
	if err:=s.sessions.Close(ctx,r.PathValue("id")); err!=nil {
		s.logger.Error("close vpn session failed","error",err)
		writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"close_failed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) RunSweeper(ctx context.Context) {
	ticker:=time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done(): return
		case now:=<-ticker.C: s.sessions.Sweep(ctx,now.UTC())
		}
	}
}

func (s *Server) ListenAndServe() error {
	err:=s.http.ListenAndServe()
	if err==http.ErrServerClosed { return nil }
	return err
}
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

func writeJSON(w http.ResponseWriter,status int,payload any) {
	w.Header().Set("Content-Type","application/json; charset=utf-8")
	w.Header().Set("Cache-Control","no-store")
	w.Header().Set("X-Content-Type-Options","nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
