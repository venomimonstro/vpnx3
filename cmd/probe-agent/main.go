package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/venomimonstro/vpnx3/internal/agent"
	"github.com/venomimonstro/vpnx3/internal/clientconfig"
	"github.com/venomimonstro/vpnx3/internal/probe"
)

const version="0.1.0"

func main() {
	logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
	controlURL:=strings.TrimSpace(os.Getenv("VPNX3_CONTROL_URL"))
	configPublicKey:=strings.TrimSpace(os.Getenv("VPNX3_CONFIG_PUBLIC_KEY"))
	identityPath:=env("VPNX3_AGENT_IDENTITY_PATH","/var/lib/vpnx3-probe/identity.json")
	if controlURL=="" || configPublicKey=="" {
		logger.Error("VPNX3_CONTROL_URL and VPNX3_CONFIG_PUBLIC_KEY are required")
		os.Exit(1)
	}

	cfg:=agent.Config{
		ControlURL:controlURL,
		EnrollmentToken:strings.TrimSpace(os.Getenv("VPNX3_ENROLLMENT_TOKEN")),
		NodeName:env("VPNX3_NODE_NAME","probe"),
		Provider:strings.TrimSpace(os.Getenv("VPNX3_NODE_PROVIDER")),
		CountryCode:strings.TrimSpace(os.Getenv("VPNX3_NODE_COUNTRY")),
		PublicIP:strings.TrimSpace(os.Getenv("VPNX3_NODE_PUBLIC_IP")),
		Capacity:1,
		AgentVersion:version,
		IdentityPath:identityPath,
	}
	id,err:=agent.LoadOrCreate(identityPath)
	if err!=nil { logger.Error("probe identity failed","error",err); os.Exit(1) }
	nodeClient:=agent.New(cfg,id)
	ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second)
	if err:=nodeClient.Enroll(ctx); err!=nil { cancel(); logger.Error("probe enrollment failed","error",err); os.Exit(1) }
	cancel()

	id,err=agent.LoadOrCreate(identityPath)
	if err!=nil || id.NodeID=="" { logger.Error("probe identity reload failed","error",err); os.Exit(1) }
	reporter:=probe.New(controlURL,identityPath,id)
	verifier,err:=clientconfig.NewVerifier(configPublicKey)
	if err!=nil { logger.Error("config verifier failed","error",err); os.Exit(1) }
	fetcher:=clientconfig.NewFetcher(verifier,env("VPNX3_CONFIG_CACHE_PATH","/var/lib/vpnx3-probe/config.json"))

	stop:=make(chan os.Signal,1)
	signal.Notify(stop,syscall.SIGINT,syscall.SIGTERM)
	ticker:=time.NewTicker(time.Duration(intEnv("VPNX3_PROBE_INTERVAL_SECONDS",60))*time.Second)
	defer ticker.Stop()

	for {
		runCtx,runCancel:=context.WithTimeout(context.Background(),20*time.Second)
		manifest,_,fetchErr:=fetcher.Fetch(runCtx,strings.TrimRight(controlURL,"/")+"/api/v1/config/latest",time.Now().UTC())
		if fetchErr==nil {
			results,observeErr:=probe.Observe(manifest)
			if observeErr==nil {
				if err:=reporter.Report(runCtx,results); err!=nil {
					logger.Warn("probe report failed","error",err)
				}
			} else { logger.Warn("probe observation failed","error",observeErr) }
		} else { logger.Warn("probe config fetch failed","error",fetchErr) }
		_ = nodeClient.Heartbeat(runCtx,agent.LocalMetadata(),0)
		runCancel()

		select {
		case <-ticker.C:
		case sig:=<-stop:
			logger.Info("probe stopping","signal",sig.String())
			return
		}
	}
}

func env(key,fallback string) string {
	if v:=strings.TrimSpace(os.Getenv(key)); v!="" { return v }
	return fallback
}
func intEnv(key string,fallback int) int {
	v,err:=strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err!=nil || v<=0 { return fallback }
	return v
}
