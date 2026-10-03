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
	"github.com/venomimonstro/vpnx3/internal/workerauth"
)

func main() {
	logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
	publicKey:=strings.TrimSpace(os.Getenv("VPNX3_ACCESS_PUBLIC_KEY"))
	if publicKey=="" {
		logger.Error("VPNX3_ACCESS_PUBLIC_KEY is required")
		os.Exit(1)
	}
	verifier,err:=accesslease.NewVerifier(publicKey)
	if err!=nil {
		logger.Error("access verifier initialization failed","error",err)
		os.Exit(1)
	}
	addr:=strings.TrimSpace(os.Getenv("VPNX3_WORKER_AUTH_ADDR"))
	if addr=="" { addr="127.0.0.1:9090" }

	srv:=workerauth.New(addr,logger,verifier)
	errCh:=make(chan error,1)
	go func(){
		logger.Info("vpn worker authorization service starting","addr",addr)
		errCh<-srv.ListenAndServe()
	}()

	stop:=make(chan os.Signal,1)
	signal.Notify(stop,syscall.SIGINT,syscall.SIGTERM)
	select {
	case sig:=<-stop:
		logger.Info("shutdown signal received","signal",sig.String())
	case err:=<-errCh:
		if err!=nil { logger.Error("worker auth server failed","error",err); os.Exit(1) }
		return
	}
	ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
	defer cancel()
	if err:=srv.Shutdown(ctx); err!=nil {
		logger.Error("worker auth shutdown failed","error",err)
		os.Exit(1)
	}
}
