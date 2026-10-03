package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/venomimonstro/vpnx3/internal/config"
)

type Server struct {
	http   *http.Server
	logger *slog.Logger
}

func NewServer(cfg config.Config, logger *slog.Logger) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"service": "control-plane",
		})
	})

	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ready",
			"service": "control-plane",
		})
	})

	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"name": "VPNX3 Control Plane",
			"api_version": "v1",
			"environment": cfg.Environment,
			"time": time.Now().UTC(),
		})
	})

	handler := requestLog(logger, recoverer(logger, mux))

	return &Server{
		logger: logger,
		http: &http.Server{
			Addr: cfg.HTTPAddr,
			Handler: handler,
			ReadTimeout: cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout: cfg.IdleTimeout,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (s *Server) ListenAndServe() error {
	err := s.http.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
