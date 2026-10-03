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
)

const version = "0.2.0"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout,nil))
	cfg := agent.Config{
		ControlURL:strings.TrimSpace(os.Getenv("VPNX3_CONTROL_URL")),
		EnrollmentToken:strings.TrimSpace(os.Getenv("VPNX3_ENROLLMENT_TOKEN")),
		NodeName:strings.TrimSpace(os.Getenv("VPNX3_NODE_NAME")),
		Provider:strings.TrimSpace(os.Getenv("VPNX3_NODE_PROVIDER")),
		CountryCode:strings.TrimSpace(os.Getenv("VPNX3_NODE_COUNTRY")),
		PublicIP:strings.TrimSpace(os.Getenv("VPNX3_NODE_PUBLIC_IP")),
		Capacity:intEnv("VPNX3_NODE_CAPACITY",1000),
		AgentVersion:version,
		IdentityPath:env("VPNX3_AGENT_IDENTITY_PATH","/var/lib/vpnx3-agent/identity.json"),
		WorkerStatusURL:strings.TrimSpace(os.Getenv("VPNX3_WORKER_STATUS_URL")),
	}
	if cfg.ControlURL=="" || cfg.NodeName=="" {
		logger.Error("VPNX3_CONTROL_URL and VPNX3_NODE_NAME are required")
		os.Exit(1)
	}

	id,err := agent.LoadOrCreate(cfg.IdentityPath)
	if err != nil {
		logger.Error("identity initialization failed","error",err)
		os.Exit(1)
	}
	client := agent.New(cfg,id)

	startCtx,cancel := context.WithTimeout(context.Background(),30*time.Second)
	if err := client.Enroll(startCtx); err != nil {
		cancel()
		logger.Error("node enrollment failed","error",err)
		os.Exit(1)
	}
	cancel()
	logger.Info("node agent enrolled and running")

	stop := make(chan os.Signal,1)
	signal.Notify(stop,syscall.SIGINT,syscall.SIGTERM)
	ticker := time.NewTicker(30*time.Second)
	defer ticker.Stop()

	for {
		ctx,cancel := context.WithTimeout(context.Background(),15*time.Second)
		metadata:=agent.LocalMetadata()
		currentSessions:=0
		if cfg.WorkerStatusURL!="" {
			status,statusErr:=client.WorkerStatus(ctx)
			if statusErr!=nil {
				metadata["worker_status"]="unavailable"
				metadata["worker_error"]=statusErr.Error()
			} else {
				currentSessions=status.Sessions
				metadata["worker_status"]=status.Status
				metadata["worker_transport"]=status.Transport
				metadata["worker_healthy"]=status.Healthy
			}
		}
		err := client.Heartbeat(ctx,metadata,currentSessions)
		cancel()
		if err != nil { logger.Warn("heartbeat failed","error",err) }

		select {
		case <-ticker.C:
		case sig := <-stop:
			logger.Info("node agent stopping","signal",sig.String())
			return
		}
	}
}

func env(key,fallback string) string {
	if v:=strings.TrimSpace(os.Getenv(key)); v!="" { return v }
	return fallback
}
func intEnv(key string,fallback int) int {
	v,err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err!=nil || v<=0 { return fallback }
	return v
}
