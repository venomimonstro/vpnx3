package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/venomimonstro/vpnx3/internal/config"
)

type Server struct {
	http   *http.Server
	logger *slog.Logger
	db     *pgxpool.Pool
}

func NewServer(cfg config.Config, logger *slog.Logger, db *pgxpool.Pool) *Server {
	mux := http.NewServeMux()

	s := &Server{
		logger: logger,
		db: db,
	}

	mux.HandleFunc("GET /health/live", s.handleLive)
	mux.HandleFunc("GET /health/ready", s.handleReady)
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"name": "VPNX3 Control Plane",
			"api_version": "v1",
			"environment": cfg.Environment,
			"time": time.Now().UTC(),
		})
	})

	handler := requestLog(logger, recoverer(logger, mux))

	s.http = &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: handler,
		ReadTimeout: cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout: cfg.IdleTimeout,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"service": "control-plane",
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		s.logger.Warn("readiness database check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"database": "unavailable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
		"database": "ok",
	})
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
