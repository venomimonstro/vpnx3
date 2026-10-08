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
	"github.com/venomimonstro/vpnx3/internal/revocation"
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
	revocationGrace:=durationEnv("VPNX3_REVOCATION_LKG_GRACE",30*time.Minute)
	if revocationGrace<0{revocationGrace=0}
	if revocationGrace>2*time.Hour{revocationGrace=2*time.Hour}

	runCtx,runCancel:=context.WithCancel(context.Background())
	defer runCancel()
	go srv.RunSweeper(runCtx)

	controlURL:=strings.TrimSpace(os.Getenv("VPNX3_CONTROL_URL"))
	revocationSources:=make([]string,0)
	if controlURL!=""{revocationSources=append(revocationSources,controlURL)}
	for _,source:=range strings.FieldsFunc(
		strings.TrimSpace(os.Getenv("VPNX3_REVOCATION_SOURCES")),
		func(r rune)bool{return r==','||r==';'},
	){
		if value:=strings.TrimSpace(source);value!=""{revocationSources=append(revocationSources,value)}
	}
	if len(revocationSources)>0{
		revVerifier,revErr:=revocation.NewVerifier(publicKey)
		if revErr!=nil{
			logger.Error("revocation verifier initialization failed","error",revErr)
			os.Exit(1)
		}
		revClient,revErr:=revocation.NewClient(
			revocationSources,
			revVerifier,
			env("VPNX3_REVOCATION_STATE_PATH","/var/lib/vpnx3-worker/revocations.json"),
		)
		if revErr!=nil{
			logger.Error("revocation client initialization failed","error",revErr)
			os.Exit(1)
		}

		haveSnapshot:=false
		if stored,ok,loadErr:=revClient.LoadStored();loadErr!=nil{
			logger.Error("stored revocation snapshot invalid","error",loadErr)
			os.Exit(1)
		}else if ok{
			if !revocation.LKGUsable(stored,time.Now().UTC(),revocationGrace){
				logger.Warn("stored revocation snapshot is outside LKG grace","version",stored.Version,"expires_at",stored.ExpiresAt)
			}else{
				if _,applyErr:=manager.ApplyRevokedDeviceHashes(context.Background(),stored.RevokedDeviceHashes);applyErr!=nil{
					logger.Error("apply stored revocation snapshot failed","error",applyErr)
					os.Exit(1)
				}
				haveSnapshot=true
				srv.SetRevocationSnapshot(stored.Version,stored.ExpiresAt,stored.ExpiresAt.Add(revocationGrace))
				logger.Info("stored revocation snapshot loaded","version",stored.Version,"revoked",len(stored.RevokedDeviceHashes))
			}
		}

		initialCtx,initialCancel:=context.WithTimeout(context.Background(),10*time.Second)
		initial,fetchErr:=revClient.Fetch(initialCtx,time.Now().UTC())
		initialCancel()
		if fetchErr==nil{
			if _,applyErr:=manager.ApplyRevokedDeviceHashes(context.Background(),initial.RevokedDeviceHashes);applyErr!=nil{
				logger.Error("apply initial revocation snapshot failed","error",applyErr)
				os.Exit(1)
			}
			haveSnapshot=true
			srv.SetRevocationSnapshot(initial.Version,initial.ExpiresAt,initial.ExpiresAt.Add(revocationGrace))
			logger.Info("fresh revocation snapshot loaded","version",initial.Version,"revoked",len(initial.RevokedDeviceHashes))
		}else if !haveSnapshot{
			logger.Error("no trusted revocation snapshot available","error",fetchErr)
			os.Exit(1)
		}else{
			logger.Warn("fresh revocation snapshot unavailable; using signed LKG","error",fetchErr)
		}

		interval:=durationEnv("VPNX3_REVOCATION_POLL_INTERVAL",time.Minute)
		if interval<15*time.Second{interval=15*time.Second}
		go func(){
			ticker:=time.NewTicker(interval);defer ticker.Stop()
			poll:=func(){
				ctx,cancel:=context.WithTimeout(runCtx,10*time.Second)
				defer cancel()
				snapshot,err:=revClient.Fetch(ctx,time.Now().UTC())
				if err!=nil{
					_,_,usableUntil,_,usable:=srv.RevocationStateForRuntime(time.Now().UTC())
					if !usable{
						closeCtx,closeCancel:=context.WithTimeout(runCtx,10*time.Second)
						closed,closeErr:=manager.CloseAll(closeCtx)
						closeCancel()
						if closeErr!=nil{logger.Error("close sessions after revocation hard-expiry failed","error",closeErr)}
						logger.Error("revocation LKG hard-expired; new sessions blocked","closed_sessions",closed,"usable_until",usableUntil,"error",err)
					}else{
						logger.Warn("revocation snapshot unavailable; keeping bounded LKG deny-list","usable_until",usableUntil,"error",err)
					}
					return
				}
				srv.SetRevocationSnapshot(snapshot.Version,snapshot.ExpiresAt,snapshot.ExpiresAt.Add(revocationGrace))
				closed,err:=manager.ApplyRevokedDeviceHashes(ctx,snapshot.RevokedDeviceHashes)
				if err!=nil{
					logger.Warn("apply revocation snapshot failed","error",err)
					return
				}
				if closed>0{logger.Warn("revoked device sessions closed","count",closed,"version",snapshot.Version)}
			}
			for{
				select{
				case <-runCtx.Done():return
				case <-ticker.C:poll()
				}
			}
		}()
	}else{
		logger.Error("revocation feed is required; configure VPNX3_CONTROL_URL or VPNX3_REVOCATION_SOURCES")
		os.Exit(1)
	}

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


func durationEnv(key string,fallback time.Duration) time.Duration{
	raw:=strings.TrimSpace(os.Getenv(key))
	if raw==""{return fallback}
	if value,err:=time.ParseDuration(raw);err==nil{return value}
	return fallback
}
