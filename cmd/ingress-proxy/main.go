package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/venomimonstro/vpnx3/internal/ingressproxy"
	"github.com/venomimonstro/vpnx3/internal/proxylease"
)

func main(){
	logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
	publicKey:=strings.TrimSpace(os.Getenv("VPNX3_ACCESS_PUBLIC_KEY"))
	cert:=strings.TrimSpace(os.Getenv("VPNX3_INGRESS_TLS_CERT"))
	key:=strings.TrimSpace(os.Getenv("VPNX3_INGRESS_TLS_KEY"))
	if publicKey==""||cert==""||key==""{
		logger.Error("VPNX3_ACCESS_PUBLIC_KEY, VPNX3_INGRESS_TLS_CERT and VPNX3_INGRESS_TLS_KEY are required")
		os.Exit(1)
	}
	verifier,err:=proxylease.NewVerifier(publicKey)
	if err!=nil{logger.Error("proxy verifier failed","error",err);os.Exit(1)}
	srv:=ingressproxy.New(env("VPNX3_INGRESS_ADDR",":8443"),logger,verifier)

	errCh:=make(chan error,1)
	go func(){logger.Info("browser ingress starting","addr",env("VPNX3_INGRESS_ADDR",":8443"));errCh<-srv.ListenAndServeTLS(cert,key)}()

	stop:=make(chan os.Signal,1);signal.Notify(stop,syscall.SIGINT,syscall.SIGTERM)
	select{
	case sig:=<-stop:logger.Info("shutdown signal received","signal",sig.String())
	case err:=<-errCh:
		if err!=nil{logger.Error("browser ingress stopped","error",err);os.Exit(1)}
		return
	}
	ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
	if err:=srv.Shutdown(ctx);err!=nil{logger.Error("shutdown failed","error",err);os.Exit(1)}
}
func env(k,f string)string{if v:=strings.TrimSpace(os.Getenv(k));v!=""{return v};return f}
