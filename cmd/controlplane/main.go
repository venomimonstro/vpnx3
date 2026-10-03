package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/venomimonstro/vpnx3/internal/config"
	"github.com/venomimonstro/vpnx3/internal/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	srv := httpapi.NewServer(cfg, logger)

	go func() {
		logger.Info("control plane starting", "addr", cfg.HTTPAddr, "env", cfg.Environment)
		if err := srv.ListenAndServe(); err != nil {
			logger.Error("http server stopped", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("control plane shutting down")
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
}
