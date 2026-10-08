package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/venomimonstro/vpnx3/internal/revocation"
	"github.com/venomimonstro/vpnx3/internal/trustbundle"
)

type envelope struct {
	Payload string `json:"payload"`
	Signature string `json:"signature"`
	KeyID string `json:"key_id"`
}

type manifestMeta struct {
	SchemaVersion int `json:"schema_version"`
	Version int64 `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type cache struct {
	mu sync.RWMutex
	configRaw []byte
	configVersion int64
	configExpires time.Time
	revocationRaw []byte
	revocationVersion int64
	revocationExpires time.Time
	trustRaw []byte
	trustVersion int64
	trustExpires time.Time
	trustPayload trustbundle.Payload
}

func main(){
	logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
	control:=strings.TrimRight(strings.TrimSpace(os.Getenv("VPNX3_CONTROL_URL")),"/")
	trustRootRaw:=strings.TrimSpace(os.Getenv("VPNX3_TRUST_ROOT_PUBLIC_KEY"))
	addr:=env("VPNX3_MIRROR_ADDR",":8443")
	configCachePath:=env("VPNX3_MIRROR_CACHE","/var/lib/vpnx3/config-mirror/latest.json")
	revocationCachePath:=env("VPNX3_MIRROR_REVOCATION_CACHE","/var/lib/vpnx3/config-mirror/revocations.json")
	trustCachePath:=env("VPNX3_MIRROR_TRUST_CACHE","/var/lib/vpnx3/config-mirror/trust-bundle.json")
	cert:=strings.TrimSpace(os.Getenv("VPNX3_MIRROR_TLS_CERT"))
	key:=strings.TrimSpace(os.Getenv("VPNX3_MIRROR_TLS_KEY"))
	if !strings.HasPrefix(control,"https://"){logger.Error("VPNX3_CONTROL_URL must use https");os.Exit(1)}
	if cert==""||key==""{logger.Error("TLS certificate and key paths are required");os.Exit(1)}

	trustVerifier,err:=trustbundle.NewVerifier(trustRootRaw)
	if err!=nil{logger.Error("invalid pinned trust root public key","error",err);os.Exit(1)}

	if err:=os.MkdirAll(filepath.Dir(configCachePath),0700);err!=nil{logger.Error("cache directory failed","error",err);os.Exit(1)}
	if err:=os.MkdirAll(filepath.Dir(revocationCachePath),0700);err!=nil{logger.Error("revocation cache directory failed","error",err);os.Exit(1)}
	if err:=os.MkdirAll(filepath.Dir(trustCachePath),0700);err!=nil{logger.Error("trust cache directory failed","error",err);os.Exit(1)}
	state:=&cache{}

	if raw,err:=os.ReadFile(trustCachePath);err==nil{
		var env trustbundle.Envelope
		if jsonErr:=json.Unmarshal(raw,&env);jsonErr==nil{
			if payload,verifyErr:=trustVerifier.Verify(env,0,time.Now().UTC());verifyErr==nil{
				state.trustRaw=raw
				state.trustVersion=payload.Version
				state.trustExpires=payload.ExpiresAt
				state.trustPayload=payload
			}
		}
	}
	if state.trustVersion>0{
		configPublicKey,configKeyID,configErr:=activeEd25519Key(state.trustPayload,"config")
		if configErr==nil{
			if raw,err:=os.ReadFile(configCachePath);err==nil{
				if meta,verifyErr:=verifyConfigEnvelope(raw,configPublicKey,configKeyID,0,time.Now().UTC());verifyErr==nil{
					state.configRaw=raw;state.configVersion=meta.Version;state.configExpires=meta.ExpiresAt
				}
			}
		}
		if accessKey,ok:=trustbundle.ActiveKey(state.trustPayload,"access");ok{
			if revVerifier,verifyErr:=revocation.NewVerifier(accessKey.PublicKey);verifyErr==nil{
				if raw,err:=os.ReadFile(revocationCachePath);err==nil{
					var env revocation.Envelope
					if jsonErr:=json.Unmarshal(raw,&env);jsonErr==nil{
						if payload,verifyErr:=revVerifier.VerifyStored(env,0);verifyErr==nil{
							state.revocationRaw=raw
							state.revocationVersion=payload.Version
							state.revocationExpires=payload.ExpiresAt
						}
					}
				}
			}
		}
	}

	ctx,cancel:=context.WithCancel(context.Background());defer cancel()
	go refreshLoop(
		ctx,logger,state,control,configCachePath,revocationCachePath,trustCachePath,
		trustVerifier,
	)

	mux:=http.NewServeMux()
	mux.HandleFunc("GET /health/live",func(w http.ResponseWriter,_ *http.Request){w.WriteHeader(http.StatusNoContent)})
	mux.HandleFunc("GET /internal/v1/status",func(w http.ResponseWriter,_ *http.Request){
		now:=time.Now().UTC()
		state.mu.RLock()
		configOK:=len(state.configRaw)>0&&state.configExpires.After(now)
		revocationOK:=len(state.revocationRaw)>0&&state.revocationExpires.After(now)
		trustOK:=len(state.trustRaw)>0&&state.trustExpires.After(now)
		configVersion:=state.configVersion
		revocationVersion:=state.revocationVersion
		trustVersion:=state.trustVersion
		state.mu.RUnlock()
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":"ok","service":"config-mirror","sessions":0,"transport":"https",
			"healthy":configOK&&revocationOK&&trustOK,
			"config_version":configVersion,"revocation_version":revocationVersion,"trust_version":trustVersion,
		})
	})
	mux.HandleFunc("GET /health/ready",func(w http.ResponseWriter,_ *http.Request){
		now:=time.Now().UTC()
		state.mu.RLock()
		ok:=len(state.configRaw)>0&&state.configExpires.After(now)&&
			len(state.revocationRaw)>0&&state.revocationExpires.After(now)&&
			len(state.trustRaw)>0&&state.trustExpires.After(now)
		state.mu.RUnlock()
		if !ok{http.Error(w,"not ready",http.StatusServiceUnavailable);return}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/config/latest",func(w http.ResponseWriter,_ *http.Request){
		state.mu.RLock();raw:=append([]byte(nil),state.configRaw...);expires:=state.configExpires;state.mu.RUnlock()
		serveSigned(w,raw,expires)
	})
	mux.HandleFunc("GET /api/v1/access/revocations",func(w http.ResponseWriter,_ *http.Request){
		state.mu.RLock();raw:=append([]byte(nil),state.revocationRaw...);expires:=state.revocationExpires;state.mu.RUnlock()
		serveSigned(w,raw,expires)
	})
	mux.HandleFunc("GET /api/v1/trust/bundle",func(w http.ResponseWriter,_ *http.Request){
		state.mu.RLock();raw:=append([]byte(nil),state.trustRaw...);expires:=state.trustExpires;state.mu.RUnlock()
		serveSigned(w,raw,expires)
	})
	server:=&http.Server{
		Addr:addr,Handler:mux,ReadHeaderTimeout:5*time.Second,
		ReadTimeout:10*time.Second,WriteTimeout:10*time.Second,IdleTimeout:60*time.Second,
	}
	logger.Info("config mirror starting","addr",addr)
	if err:=server.ListenAndServeTLS(cert,key);err!=nil&&err!=http.ErrServerClosed{
		logger.Error("mirror stopped","error",err);os.Exit(1)
	}
}

func serveSigned(w http.ResponseWriter,raw []byte,expires time.Time){
	if len(raw)==0||!expires.After(time.Now().UTC()){
		http.Error(w,"signed data unavailable",http.StatusServiceUnavailable);return
	}
	w.Header().Set("Content-Type","application/json")
	w.Header().Set("Cache-Control","public, max-age=30")
	w.Header().Set("X-Content-Type-Options","nosniff")
	w.WriteHeader(http.StatusOK);_,_=w.Write(raw)
}

func refreshLoop(
	ctx context.Context,logger *slog.Logger,state *cache,control,configPath,revocationPath,trustPath string,
	trustVerifier *trustbundle.Verifier,
){
	client:=&http.Client{
		Timeout:10*time.Second,
		CheckRedirect:func(_ *http.Request,_ []*http.Request)error{return http.ErrUseLastResponse},
	}
	t:=time.NewTicker(time.Minute);defer t.Stop()
	for{
		if err:=refreshTrust(ctx,client,state,control,trustPath,trustVerifier);err!=nil{
			logger.Warn("trust bundle mirror refresh failed","error",err)
		}
		state.mu.RLock()
		trustPayload:=state.trustPayload
		trustValid:=state.trustVersion>0&&state.trustExpires.After(time.Now().UTC())
		state.mu.RUnlock()
		if trustValid{
			configPublic,configKeyID,err:=activeEd25519Key(trustPayload,"config")
			if err!=nil{
				logger.Warn("active config key unavailable from trust bundle","error",err)
			}else if err:=refreshConfig(ctx,client,state,control,configPath,configPublic,configKeyID);err!=nil{
				logger.Warn("config mirror refresh failed","error",err)
			}
			accessKey,ok:=trustbundle.ActiveKey(trustPayload,"access")
			if !ok{
				logger.Warn("active access key unavailable from trust bundle")
			}else{
				revVerifier,err:=revocation.NewVerifier(accessKey.PublicKey)
				if err!=nil{
					logger.Warn("access key from trust bundle invalid","error",err)
				}else if err:=refreshRevocations(ctx,client,state,control,revocationPath,revVerifier);err!=nil{
					logger.Warn("revocation mirror refresh failed","error",err)
				}
			}
		}
		select{case<-ctx.Done():return;case<-t.C:}
	}
}

func refreshTrust(
	ctx context.Context,client *http.Client,state *cache,control,cachePath string,
	verifier *trustbundle.Verifier,
)error{
	raw,err:=fetch(ctx,client,control+"/api/v1/trust/bundle",1<<20);if err!=nil{return err}
	var env trustbundle.Envelope
	if err:=json.Unmarshal(raw,&env);err!=nil{return err}
	state.mu.RLock();minimum:=state.trustVersion;state.mu.RUnlock()
	payload,err:=verifier.Verify(env,minimum,time.Now().UTC());if err!=nil{return err}
	if err:=atomicWrite(cachePath,raw);err!=nil{return err}
	state.mu.Lock()
	state.trustRaw=append(state.trustRaw[:0],raw...)
	state.trustVersion=payload.Version
	state.trustExpires=payload.ExpiresAt
	state.trustPayload=payload
	state.mu.Unlock()
	return nil
}

func refreshConfig(
	ctx context.Context,client *http.Client,state *cache,control,cachePath string,
	publicKey ed25519.PublicKey,keyID string,
)error{
	raw,err:=fetch(ctx,client,control+"/api/v1/config/latest",1<<20);if err!=nil{return err}
	state.mu.RLock();minimum:=state.configVersion;state.mu.RUnlock()
	meta,err:=verifyConfigEnvelope(raw,publicKey,keyID,minimum,time.Now().UTC());if err!=nil{return err}
	if err:=atomicWrite(cachePath,raw);err!=nil{return err}
	state.mu.Lock()
	state.configRaw=append(state.configRaw[:0],raw...)
	state.configVersion=meta.Version
	state.configExpires=meta.ExpiresAt
	state.mu.Unlock()
	return nil
}

func refreshRevocations(
	ctx context.Context,client *http.Client,state *cache,control,cachePath string,
	verifier *revocation.Verifier,
)error{
	raw,err:=fetch(ctx,client,control+"/api/v1/access/revocations",4<<20);if err!=nil{return err}
	var env revocation.Envelope
	if err:=json.Unmarshal(raw,&env);err!=nil{return err}
	state.mu.RLock();minimum:=state.revocationVersion;state.mu.RUnlock()
	payload,err:=verifier.Verify(env,minimum,time.Now().UTC());if err!=nil{return err}
	if err:=atomicWrite(cachePath,raw);err!=nil{return err}
	state.mu.Lock()
	state.revocationRaw=append(state.revocationRaw[:0],raw...)
	state.revocationVersion=payload.Version
	state.revocationExpires=payload.ExpiresAt
	state.mu.Unlock()
	return nil
}

func activeEd25519Key(p trustbundle.Payload,purpose string)(ed25519.PublicKey,string,error){
	k,ok:=trustbundle.ActiveKey(p,purpose)
	if !ok{return nil,"",fmt.Errorf("active %s key missing",purpose)}
	raw,err:=base64.RawURLEncoding.DecodeString(k.PublicKey)
	if err!=nil||len(raw)!=ed25519.PublicKeySize{return nil,"",fmt.Errorf("invalid active %s key",purpose)}
	return ed25519.PublicKey(raw),k.KeyID,nil
}

func fetch(ctx context.Context,client *http.Client,url string,limit int64)([]byte,error){
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,url,nil);if err!=nil{return nil,err}
	req.Header.Set("Accept","application/json")
	req.Header.Set("Cache-Control","no-cache")
	resp,err:=client.Do(req);if err!=nil{return nil,err}
	defer resp.Body.Close()
	raw,err:=io.ReadAll(io.LimitReader(resp.Body,limit));if err!=nil{return nil,err}
	if resp.StatusCode!=http.StatusOK{return nil,fmt.Errorf("upstream returned HTTP %d",resp.StatusCode)}
	return raw,nil
}

func atomicWrite(path string,raw []byte)error{
	tmp:=path+".tmp"
	if err:=os.WriteFile(tmp,raw,0600);err!=nil{return err}
	return os.Rename(tmp,path)
}

func verifyConfigEnvelope(
	raw []byte,publicKey ed25519.PublicKey,keyID string,minimum int64,now time.Time,
)(manifestMeta,error){
	var env envelope
	if err:=json.Unmarshal(raw,&env);err!=nil{return manifestMeta{},err}
	if env.KeyID!=keyID{return manifestMeta{},errors.New("unexpected signing key id")}
	payload,err:=base64.RawURLEncoding.DecodeString(env.Payload);if err!=nil{return manifestMeta{},err}
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature);if err!=nil{return manifestMeta{},err}
	if !ed25519.Verify(publicKey,payload,sig){return manifestMeta{},errors.New("invalid manifest signature")}
	var meta manifestMeta
	if err:=json.Unmarshal(payload,&meta);err!=nil{return manifestMeta{},err}
	if meta.SchemaVersion!=1||meta.Version<minimum{return manifestMeta{},errors.New("invalid or rolled back manifest")}
	if meta.CreatedAt.After(now.Add(5*time.Minute))||!meta.ExpiresAt.After(now){
		return manifestMeta{},errors.New("manifest outside validity window")
	}
	return meta,nil
}

func env(key,fallback string)string{
	if v:=strings.TrimSpace(os.Getenv(key));v!=""{return v}
	return fallback
}
