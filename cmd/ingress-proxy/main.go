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
	"github.com/venomimonstro/vpnx3/internal/revocation"
	"github.com/venomimonstro/vpnx3/internal/trustbundle"
)

func main(){
	logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
	controlURL:=strings.TrimSpace(os.Getenv("VPNX3_CONTROL_URL"))
	legacyPublicKey:=strings.TrimSpace(os.Getenv("VPNX3_ACCESS_PUBLIC_KEY"))
	trustRoot:=strings.TrimSpace(os.Getenv("VPNX3_TRUST_ROOT_PUBLIC_KEY"))
	cert:=strings.TrimSpace(os.Getenv("VPNX3_INGRESS_TLS_CERT"))
	key:=strings.TrimSpace(os.Getenv("VPNX3_INGRESS_TLS_KEY"))
	if cert==""||key==""{
		logger.Error("VPNX3_INGRESS_TLS_CERT and VPNX3_INGRESS_TLS_KEY are required")
		os.Exit(1)
	}

	var verifier *proxylease.Verifier
	var revVerifier *revocation.Verifier
	var trustClient *trustbundle.Client
	var trustPayload trustbundle.Payload
	var err error
	if trustRoot!=""{
		sources:=ingressTrustSources(controlURL,strings.TrimSpace(os.Getenv("VPNX3_TRUST_SOURCES")))
		trustCtx,trustCancel:=context.WithTimeout(context.Background(),10*time.Second)
		trustClient,trustPayload,err=trustbundle.Bootstrap(
			trustCtx,sources,trustRoot,
			env("VPNX3_TRUST_STATE_PATH","/var/lib/vpnx3/ingress-trust-bundle.json"),
			time.Now().UTC(),
		)
		trustCancel()
		if err!=nil{logger.Error("runtime trust bootstrap failed","error",err);os.Exit(1)}
		keys:=trustbundle.VerificationKeys(trustPayload,"access")
		verifier,err=proxylease.NewVerifierSet(keys)
		if err==nil{err=verifier.ReplaceKeysUntil(keys,trustPayload.ExpiresAt)}
		if err!=nil{logger.Error("proxy trust keyring initialization failed","error",err);os.Exit(1)}
		revVerifier,err=revocation.NewVerifierSet(keys)
		if err==nil{err=revVerifier.ReplaceKeysUntil(keys,trustPayload.ExpiresAt)}
		if err!=nil{logger.Error("revocation trust keyring initialization failed","error",err);os.Exit(1)}
		logger.Info("root-signed ingress keyring loaded","trust_version",trustPayload.Version,"expires_at",trustPayload.ExpiresAt)
	}else{
		if legacyPublicKey==""{logger.Error("VPNX3_TRUST_ROOT_PUBLIC_KEY or VPNX3_ACCESS_PUBLIC_KEY is required");os.Exit(1)}
		verifier,err=proxylease.NewVerifier(legacyPublicKey)
		if err!=nil{logger.Error("proxy verifier failed","error",err);os.Exit(1)}
		revVerifier,err=revocation.NewVerifier(legacyPublicKey)
		if err!=nil{logger.Error("revocation verifier failed","error",err);os.Exit(1)}
		logger.Warn("legacy single access key mode enabled; configure offline-root trust bundle")
	}
	srv:=ingressproxy.New(env("VPNX3_INGRESS_ADDR",":8443"),logger,verifier)
	revocationGrace:=durationEnv("VPNX3_REVOCATION_LKG_GRACE",30*time.Minute)
	if revocationGrace<0{revocationGrace=0}
	if revocationGrace>2*time.Hour{revocationGrace=2*time.Hour}

	sources:=make([]string,0)
	if controlURL!=""{sources=append(sources,controlURL)}
	for _,source:=range strings.FieldsFunc(
		strings.TrimSpace(os.Getenv("VPNX3_REVOCATION_SOURCES")),
		func(r rune)bool{return r==','||r==';'},
	){
		if value:=strings.TrimSpace(source);value!=""{sources=append(sources,value)}
	}
	if len(sources)==0{
		logger.Error("revocation sources are required for browser ingress")
		os.Exit(1)
	}
	revClient,err:=revocation.NewClient(
		sources,revVerifier,env("VPNX3_REVOCATION_STATE_PATH","/var/lib/vpnx3/ingress-revocations.json"),
	)
	if err!=nil{logger.Error("revocation client initialization failed","error",err);os.Exit(1)}

	haveSnapshot:=false
	if stored,ok,loadErr:=revClient.LoadStored();loadErr!=nil{
		logger.Error("stored revocation snapshot invalid","error",loadErr);os.Exit(1)
	}else if ok{
		if !revocation.LKGUsable(stored,time.Now().UTC(),revocationGrace){
			logger.Warn("stored revocation snapshot is outside LKG grace","version",stored.Version,"expires_at",stored.ExpiresAt)
		}else{
			srv.ApplyRevokedDeviceHashes(stored.RevokedDeviceHashes)
			srv.SetRevocationSnapshot(stored.Version,stored.ExpiresAt,stored.ExpiresAt.Add(revocationGrace))
			haveSnapshot=true
		}
		logger.Info("stored revocation snapshot loaded","version",stored.Version,"revoked",len(stored.RevokedDeviceHashes))
	}
	initialCtx,initialCancel:=context.WithTimeout(context.Background(),10*time.Second)
	initial,fetchErr:=revClient.Fetch(initialCtx,time.Now().UTC())
	initialCancel()
	if fetchErr==nil{
		srv.ApplyRevokedDeviceHashes(initial.RevokedDeviceHashes)
		srv.SetRevocationSnapshot(initial.Version,initial.ExpiresAt,initial.ExpiresAt.Add(revocationGrace))
		haveSnapshot=true
		logger.Info("fresh revocation snapshot loaded","version",initial.Version,"revoked",len(initial.RevokedDeviceHashes))
	}else if !haveSnapshot{
		logger.Error("no trusted revocation snapshot available","error",fetchErr);os.Exit(1)
	}else{
		logger.Warn("fresh revocation snapshot unavailable; using signed LKG","error",fetchErr)
	}

	runCtx,runCancel:=context.WithCancel(context.Background())
	defer runCancel()

	if trustClient!=nil{
		go trustbundle.Poll(
			runCtx,trustClient,5*time.Minute,
			func(payload trustbundle.Payload)error{
				keys:=trustbundle.VerificationKeys(payload,"access")
				if err:=verifier.ReplaceKeysUntil(keys,payload.ExpiresAt);err!=nil{return err}
				if err:=revVerifier.ReplaceKeysUntil(keys,payload.ExpiresAt);err!=nil{return err}
				logger.Info("runtime ingress keyring refreshed","trust_version",payload.Version,"expires_at",payload.ExpiresAt)
				return nil
			},
			func(err error){logger.Warn("runtime ingress trust refresh failed","error",err)},
		)
	}

	interval:=durationEnv("VPNX3_REVOCATION_POLL_INTERVAL",time.Minute)
	if interval<15*time.Second{interval=15*time.Second}
	go func(){
		ticker:=time.NewTicker(interval);defer ticker.Stop()
		for{
			select{
			case <-runCtx.Done():return
			case <-ticker.C:
				ctx,cancel:=context.WithTimeout(runCtx,10*time.Second)
				snapshot,err:=revClient.Fetch(ctx,time.Now().UTC())
				cancel()
				if err!=nil{
					if !srv.RevocationUsable(time.Now().UTC()){
						closed:=srv.CloseAllConnections()
						logger.Error("revocation LKG hard-expired; proxy auth blocked","closed_connections",closed,"error",err)
					}else{
						logger.Warn("revocation snapshot unavailable; keeping bounded LKG deny-list","error",err)
					}
					continue
				}
				srv.SetRevocationSnapshot(snapshot.Version,snapshot.ExpiresAt,snapshot.ExpiresAt.Add(revocationGrace))
				closed:=srv.ApplyRevokedDeviceHashes(snapshot.RevokedDeviceHashes)
				if closed>0{logger.Warn("revoked browser tunnels closed","connections",closed,"version",snapshot.Version)}
			}
		}
	}()

	errCh:=make(chan error,1)
	go func(){logger.Info("browser ingress starting","addr",env("VPNX3_INGRESS_ADDR",":8443"));errCh<-srv.ListenAndServeTLS(cert,key)}()

	stop:=make(chan os.Signal,1);signal.Notify(stop,syscall.SIGINT,syscall.SIGTERM)
	select{
	case sig:=<-stop:logger.Info("shutdown signal received","signal",sig.String())
	case err:=<-errCh:
		if err!=nil{logger.Error("browser ingress stopped","error",err);os.Exit(1)}
		return
	}
	runCancel()
	ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
	if err:=srv.Shutdown(ctx);err!=nil{logger.Error("shutdown failed","error",err);os.Exit(1)}
}
func env(k,f string)string{if v:=strings.TrimSpace(os.Getenv(k));v!=""{return v};return f}

func durationEnv(key string,fallback time.Duration)time.Duration{
	raw:=strings.TrimSpace(os.Getenv(key))
	if raw==""{return fallback}
	if value,err:=time.ParseDuration(raw);err==nil{return value}
	return fallback
}


func ingressTrustSources(controlURL,raw string)[]string{
	out:=make([]string,0,8)
	seen:=map[string]struct{}{}
	add:=func(value string){
		value=strings.TrimSpace(value)
		if value==""{return}
		if _,ok:=seen[value];ok{return}
		seen[value]=struct{}{}
		out=append(out,value)
	}
	add(controlURL)
	for _,part:=range strings.FieldsFunc(raw,func(r rune)bool{return r==','||r==';'}){
		add(part)
	}
	return out
}
