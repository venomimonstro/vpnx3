package workerauth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accesslease"
)

type Server struct {
	http *http.Server
	logger *slog.Logger
	verifier *accesslease.Verifier
}

func New(addr string,logger *slog.Logger,verifier *accesslease.Verifier) *Server {
	s:=&Server{logger:logger,verifier:verifier}
	mux:=http.NewServeMux()
	mux.HandleFunc("GET /health/live",func(w http.ResponseWriter,r *http.Request){
		writeJSON(w,http.StatusOK,map[string]string{"status":"ok","service":"vpn-worker-auth"})
	})
	mux.HandleFunc("POST /internal/v1/authorize",s.handleAuthorize)
	s.http=&http.Server{
		Addr:addr,
		Handler:mux,
		ReadHeaderTimeout:3*time.Second,
		ReadTimeout:5*time.Second,
		WriteTimeout:5*time.Second,
		IdleTimeout:30*time.Second,
	}
	return s
}

func (s *Server) handleAuthorize(w http.ResponseWriter,r *http.Request) {
	var env accesslease.Envelope
	decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,64<<10))
	decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&env); err!=nil {
		writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_lease"})
		return
	}
	claims,err:=s.verifier.Verify(env,time.Now().UTC())
	if err!=nil {
		s.logger.Warn("access lease rejected","error",err)
		writeJSON(w,http.StatusUnauthorized,map[string]string{"error":"access_denied"})
		return
	}
	writeJSON(w,http.StatusOK,map[string]any{
		"authorized":true,
		"user_id":claims.UserID,
		"device_id":claims.DeviceID,
		"entitlement":claims.Entitlement,
		"expires_at":claims.ExpiresAt,
	})
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
