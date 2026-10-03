package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/venomimonstro/vpnx3/internal/config"
	"github.com/venomimonstro/vpnx3/internal/database"
	"github.com/venomimonstro/vpnx3/internal/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	dir := os.Getenv("VPNX3_MIGRATIONS_DIR")
	if dir == "" {
		dir = "/migrations"
	}
	if err := migrations.Run(ctx, db, dir); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations complete")
}
