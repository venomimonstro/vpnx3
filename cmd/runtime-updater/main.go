package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/runtimeupdate"
)

func main(){
	logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
	cfg:=runtimeupdate.Config{
		ControlURL:strings.TrimSpace(os.Getenv("VPNX3_CONTROL_URL")),
		TrustRootPublicKey:strings.TrimSpace(os.Getenv("VPNX3_TRUST_ROOT_PUBLIC_KEY")),
		ReleaseSources:split(os.Getenv("VPNX3_RELEASE_SOURCES")),
		TargetName:strings.TrimSpace(os.Getenv("VPNX3_UPDATE_TARGET")),
		StatePath:strings.TrimSpace(os.Getenv("VPNX3_UPDATE_STATE_PATH")),
		HealthURL:strings.TrimSpace(os.Getenv("VPNX3_UPDATE_HEALTH_URL")),
	}
	updater,err:=runtimeupdate.New(cfg)
	if err!=nil{logger.Error("runtime updater configuration failed","error",err);os.Exit(2)}
	ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute)
	defer cancel()
	changed,err:=updater.Run(ctx)
	if err!=nil{logger.Error("runtime update failed","error",err);os.Exit(1)}
	logger.Info("runtime update check complete","updated",changed)
}

func split(raw string)[]string{
	out:=make([]string,0)
	for _,v:=range strings.FieldsFunc(raw,func(r rune)bool{return r==','||r==';'}){
		v=strings.TrimSpace(v)
		if v!=""{out=append(out,v)}
	}
	return out
}
