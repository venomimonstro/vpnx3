package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/database"
	"github.com/venomimonstro/vpnx3/internal/migrations"
)

func main() {
	databaseURL:=strings.TrimSpace(os.Getenv("VPNX3_DATABASE_URL"))
	if databaseURL=="" {
		slog.Error("VPNX3_DATABASE_URL is required")
		os.Exit(2)
	}
	dir:=strings.TrimSpace(os.Getenv("VPNX3_MIGRATIONS_DIR"))
	if dir=="" { dir="/migrations" }

	logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
	ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute)
	defer cancel()

	db,err:=database.Open(ctx,databaseURL)
	if err!=nil{
		logger.Error("database initialization failed","error",err)
		os.Exit(1)
	}
	defer db.Close()

	if err:=migrations.Run(ctx,db,dir);err!=nil{
		logger.Error("migration failed","error",err)
		os.Exit(1)
	}
	logger.Info("database migrations complete","directory",dir)
}
