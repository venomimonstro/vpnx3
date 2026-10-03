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
	"github.com/venomimonstro/vpnx3/internal/buildworker"
)

const version="0.1.0"

func main(){
	logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
	cfg:=agent.Config{
		ControlURL:strings.TrimSpace(os.Getenv("VPNX3_CONTROL_URL")),
		EnrollmentToken:strings.TrimSpace(os.Getenv("VPNX3_ENROLLMENT_TOKEN")),
		NodeName:env("VPNX3_NODE_NAME","build-worker"),
		Provider:strings.TrimSpace(os.Getenv("VPNX3_NODE_PROVIDER")),
		CountryCode:strings.TrimSpace(os.Getenv("VPNX3_NODE_COUNTRY")),
		PublicIP:strings.TrimSpace(os.Getenv("VPNX3_NODE_PUBLIC_IP")),
		Capacity:1,AgentVersion:version,
		IdentityPath:env("VPNX3_AGENT_IDENTITY_PATH","/var/lib/vpnx3-build-worker/identity.json"),
	}
	if cfg.ControlURL=="" { logger.Error("VPNX3_CONTROL_URL is required");os.Exit(1) }
	if strings.Contains(env("VPNX3_BUILD_TARGETS","android_apk,android_aab"),"android_") {
		if !strings.HasPrefix(env("VPNX3_CLIENT_CONTROL_URL",""),"https://") ||
			strings.TrimSpace(os.Getenv("VPNX3_CONFIG_PUBLIC_KEY"))=="" {
			logger.Error("Android build worker requires VPNX3_CLIENT_CONTROL_URL=https://... and VPNX3_CONFIG_PUBLIC_KEY")
			os.Exit(1)
		}
	}
	id,err:=agent.LoadOrCreate(cfg.IdentityPath);if err!=nil{logger.Error("identity failed","error",err);os.Exit(1)}
	node:=agent.New(cfg,id)
	ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second)
	if err:=node.Enroll(ctx);err!=nil{cancel();logger.Error("enrollment failed","error",err);os.Exit(1)};cancel()
	id,err=agent.LoadOrCreate(cfg.IdentityPath);if err!=nil{logger.Error("identity reload failed","error",err);os.Exit(1)}

	client:=buildworker.New(cfg.ControlURL,cfg.IdentityPath,id)
	executor:=buildworker.Executor{
		SourceRepo:env("VPNX3_SOURCE_REPO","https://github.com/venomimonstro/vpnx3.git"),
		WorkRoot:env("VPNX3_BUILD_WORK_ROOT","/var/lib/vpnx3-build-worker/work"),
		Timeout:time.Duration(intEnv("VPNX3_BUILD_TIMEOUT_MINUTES",30))*time.Minute,
	}
	targets:=splitCSV(env("VPNX3_BUILD_TARGETS","android_apk,android_aab"))

	stop:=make(chan os.Signal,1);signal.Notify(stop,syscall.SIGINT,syscall.SIGTERM)
	ticker:=time.NewTicker(15*time.Second);defer ticker.Stop()
	logger.Info("build worker running","targets",targets)

	for{
		runCtx,runCancel:=context.WithTimeout(context.Background(),45*time.Minute)
		job,claimErr:=client.Claim(runCtx,targets)
		if claimErr!=nil{logger.Warn("claim failed","error",claimErr)}
		if job!=nil{
			logger.Info("build started","job_id",job.ID,"target",job.Target,"commit",job.SourceCommit)
			result,buildErr:=executor.Run(runCtx,*job)
			if buildErr!=nil{
				logger.Error("build failed","job_id",job.ID,"error",buildErr)
				_ = client.Complete(runCtx,job.ID,"failed",buildErr.Error())
			}else{
				logger.Info("build succeeded","job_id",job.ID,"file",result.FileName,"sha256",result.SHA256,"size_bytes",result.SizeBytes)
				if uploadErr:=client.UploadArtifact(runCtx,job.ID,result.Path);uploadErr!=nil{
					logger.Error("artifact upload failed","job_id",job.ID,"error",uploadErr)
					_ = client.Complete(runCtx,job.ID,"failed","artifact upload failed: "+uploadErr.Error())
				}
			}
		}
		runCancel()
		select{case <-ticker.C:case sig:=<-stop:logger.Info("build worker stopping","signal",sig.String());return}
	}
}
func env(k,f string)string{if v:=strings.TrimSpace(os.Getenv(k));v!=""{return v};return f}
func intEnv(k string,f int)int{v,e:=strconv.Atoi(strings.TrimSpace(os.Getenv(k)));if e!=nil||v<=0{return f};return v}
func splitCSV(v string)[]string{var out []string;for _,x:=range strings.Split(v,","){if x=strings.TrimSpace(x);x!=""{out=append(out,x)}};return out}
