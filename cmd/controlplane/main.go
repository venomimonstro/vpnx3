package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/venomimonstro/vpnx3/internal/bootstrap"
	"github.com/venomimonstro/vpnx3/internal/config"
	"github.com/venomimonstro/vpnx3/internal/database"
	"github.com/venomimonstro/vpnx3/internal/httpapi"
	"github.com/venomimonstro/vpnx3/internal/nodemonitor"
	"github.com/venomimonstro/vpnx3/internal/loginthrottle"
	"github.com/venomimonstro/vpnx3/internal/probemonitor"
	"github.com/venomimonstro/vpnx3/internal/signing"
	"github.com/venomimonstro/vpnx3/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil { panic(err) }

	logger := slog.New(slog.NewJSONHandler(os.Stdout,&slog.HandlerOptions{Level:cfg.LogLevel}))

	startupCtx, startupCancel := context.WithTimeout(context.Background(),15*time.Second)
	defer startupCancel()

	db, err := database.Open(startupCtx,cfg.DatabaseURL)
	if err != nil {
		logger.Error("database initialization failed","error",err)
		os.Exit(1)
	}
	defer db.Close()

	if err := bootstrap.EnsureOwner(startupCtx,store.New(db),logger,cfg.BootstrapOwnerEmail,cfg.BootstrapOwnerPassword); err != nil {
		logger.Error("bootstrap initialization failed","error",err)
		os.Exit(1)
	}

	configSigner,err:=signing.FromSeedBase64(cfg.ConfigSigningKey)
	if err!=nil {
		logger.Error("config signing key initialization failed","error",err)
		os.Exit(1)
	}

	accessSigner,err:=signing.FromSeedBase64(cfg.AccessSigningKey)
	if err!=nil {
		logger.Error("access signing key initialization failed","error",err)
		os.Exit(1)
	}

	var releaseSigner *signing.Signer
	if cfg.ReleaseSigningKey!="" {
		releaseSigner,err=signing.FromSeedBase64(cfg.ReleaseSigningKey)
		if err!=nil {
			logger.Error("release signing key initialization failed","error",err)
			os.Exit(1)
		}
	}

	nodeStore := store.New(db)
	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()
	go nodemonitor.New(nodeStore,logger).Run(monitorCtx)
	go loginthrottle.New(nodeStore,logger).Run(monitorCtx)
	go probemonitor.New(nodeStore,logger).Run(monitorCtx)

	srv := httpapi.NewServer(cfg,logger,db,configSigner,accessSigner,releaseSigner)
	serverErr := make(chan error,1)
	go func() {
		logger.Info("control plane starting","addr",cfg.HTTPAddr,"env",cfg.Environment)
		serverErr <- srv.ListenAndServe()
	}()

	stop := make(chan os.Signal,1)
	signal.Notify(stop,syscall.SIGINT,syscall.SIGTERM)
	select {
	case sig := <-stop:
		logger.Info("shutdown signal received","signal",sig.String())
	case err := <-serverErr:
		if err != nil {
			logger.Error("http server stopped unexpectedly","error",err)
			os.Exit(1)
		}
		return
	}

	ctx,cancel := context.WithTimeout(context.Background(),10*time.Second)
	defer cancel()
	logger.Info("control plane shutting down")
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed","error",err)
		os.Exit(1)
	}
}
