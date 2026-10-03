package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accesslease"
	"github.com/venomimonstro/vpnx3/internal/ipam"
	"github.com/venomimonstro/vpnx3/internal/sessions"
	"github.com/venomimonstro/vpnx3/internal/workerauth"
	wgadapter "github.com/venomimonstro/vpnx3/network/transport/wireguard"
)

func main() {
	logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
	publicKey:=strings.TrimSpace(os.Getenv("VPNX3_ACCESS_PUBLIC_KEY"))
	if publicKey=="" { logger.Error("VPNX3_ACCESS_PUBLIC_KEY is required"); os.Exit(1) }
	verifier,err:=accesslease.NewVerifier(publicKey)
	if err!=nil { logger.Error("access verifier initialization failed","error",err); os.Exit(1) }

	pool,err:=ipam.New(env("VPNX3_WG_POOL","10.66.0.0/24"))
	if err!=nil { logger.Error("IP pool initialization failed","error",err); os.Exit(1) }

	adapter,err:=wgadapter.New(env("VPNX3_WG_INTERFACE","wg0"),strings.TrimSpace(os.Getenv("VPNX3_WG_ENDPOINT")))
	if err!=nil { logger.Error("wireguard adapter initialization failed","error",err); os.Exit(1) }
	healthCtx,healthCancel:=context.WithTimeout(context.Background(),3*time.Second)
	if err:=adapter.Healthy(healthCtx); err!=nil {
		healthCancel()
		logger.Error("wireguard interface is not ready","error",err)
		os.Exit(1)
	}
	healthCancel()

	manager:=sessions.New(
		verifier,
		pool,
		adapter,
		env("VPNX3_WORKER_STATE_PATH","/var/lib/vpnx3-worker/sessions.json"),
	)
	restoreCtx,restoreCancel:=context.WithTimeout(context.Background(),15*time.Second)
	restored,err:=manager.Restore(restoreCtx,time.Now().UTC())
	restoreCancel()
	if err!=nil {
		logger.Error("worker session recovery failed","error",err)
		os.Exit(1)
	}
	logger.Info("worker session state restored","sessions",restored)

	addr:=env("VPNX3_WORKER_AUTH_ADDR","127.0.0.1:9090")
	srv:=workerauth.New(addr,logger,verifier,manager,adapter)

	runCtx,runCancel:=context.WithCancel(context.Background())
	defer runCancel()
	go srv.RunSweeper(runCtx)

	errCh:=make(chan error,1)
	go func(){ logger.Info("vpn worker starting","addr",addr,"transport","wireguard"); errCh<-srv.ListenAndServe() }()

	stop:=make(chan os.Signal,1)
	signal.Notify(stop,syscall.SIGINT,syscall.SIGTERM)
	select {
	case sig:=<-stop: logger.Info("shutdown signal received","signal",sig.String())
	case err:=<-errCh:
		if err!=nil { logger.Error("vpn worker failed","error",err); os.Exit(1) }
		return
	}
	runCancel()
	ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
	defer cancel()
	if err:=srv.Shutdown(ctx); err!=nil { logger.Error("worker shutdown failed","error",err); os.Exit(1) }
}

func env(key,fallback string) string {
	if v:=strings.TrimSpace(os.Getenv(key)); v!="" { return v }
	return fallback
}
