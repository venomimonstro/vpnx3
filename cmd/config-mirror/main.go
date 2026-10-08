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
	raw []byte
	version int64
	expires time.Time
}

func main(){
	logger:=slog.New(slog.NewJSONHandler(os.Stdout,nil))
	control:=strings.TrimRight(strings.TrimSpace(os.Getenv("VPNX3_CONTROL_URL")),"/")
	publicKeyRaw:=strings.TrimSpace(os.Getenv("VPNX3_CONFIG_PUBLIC_KEY"))
	addr:=env("VPNX3_MIRROR_ADDR",":8443")
	cachePath:=env("VPNX3_MIRROR_CACHE","/var/lib/vpnx3/config-mirror/latest.json")
	cert:=strings.TrimSpace(os.Getenv("VPNX3_MIRROR_TLS_CERT"))
	key:=strings.TrimSpace(os.Getenv("VPNX3_MIRROR_TLS_KEY"))
	if !strings.HasPrefix(control,"https://"){logger.Error("VPNX3_CONTROL_URL must use https");os.Exit(1)}
	if cert==""||key==""{logger.Error("TLS certificate and key paths are required");os.Exit(1)}
	pub,err:=base64.RawURLEncoding.DecodeString(publicKeyRaw)
	if err!=nil||len(pub)!=ed25519.PublicKeySize{logger.Error("invalid pinned config public key");os.Exit(1)}
	publicKey:=ed25519.PublicKey(pub)
	sum:=sha256.Sum256(publicKey)
	keyID:=hex.EncodeToString(sum[:8])

	if err:=os.MkdirAll(filepath.Dir(cachePath),0700);err!=nil{logger.Error("cache directory failed","error",err);os.Exit(1)}
	state:=&cache{}
	if raw,err:=os.ReadFile(cachePath);err==nil{
		if meta,verifyErr:=verifyEnvelope(raw,publicKey,keyID,0,time.Now().UTC());verifyErr==nil{
			state.raw=raw;state.version=meta.Version;state.expires=meta.ExpiresAt
		}
	}

	ctx,cancel:=context.WithCancel(context.Background());defer cancel()
	go refreshLoop(ctx,logger,state,control,cachePath,publicKey,keyID)

	mux:=http.NewServeMux()
	mux.HandleFunc("GET /health/live",func(w http.ResponseWriter,_ *http.Request){w.WriteHeader(http.StatusNoContent)})
	mux.HandleFunc("GET /health/ready",func(w http.ResponseWriter,_ *http.Request){
		state.mu.RLock();ok:=len(state.raw)>0&&state.expires.After(time.Now().UTC());state.mu.RUnlock()
		if !ok{http.Error(w,"not ready",http.StatusServiceUnavailable);return}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/config/latest",func(w http.ResponseWriter,_ *http.Request){
		state.mu.RLock();raw:=append([]byte(nil),state.raw...);expires:=state.expires;state.mu.RUnlock()
		if len(raw)==0||!expires.After(time.Now().UTC()){http.Error(w,"configuration unavailable",http.StatusServiceUnavailable);return}
		w.Header().Set("Content-Type","application/json")
		w.Header().Set("Cache-Control","no-store")
		w.WriteHeader(http.StatusOK);_,_=w.Write(raw)
	})
	server:=&http.Server{Addr:addr,Handler:mux,ReadHeaderTimeout:5*time.Second,ReadTimeout:10*time.Second,WriteTimeout:10*time.Second,IdleTimeout:60*time.Second}
	logger.Info("config mirror starting","addr",addr)
	if err:=server.ListenAndServeTLS(cert,key);err!=nil&&err!=http.ErrServerClosed{logger.Error("mirror stopped","error",err);os.Exit(1)}
}

func refreshLoop(ctx context.Context,logger *slog.Logger,state *cache,control,cachePath string,publicKey ed25519.PublicKey,keyID string){
	client:=&http.Client{Timeout:10*time.Second}
	t:=time.NewTicker(time.Minute);defer t.Stop()
	for{
		if err:=refresh(ctx,client,state,control,cachePath,publicKey,keyID);err!=nil{
			logger.Warn("config mirror refresh failed","error",err)
		}
		select{case<-ctx.Done():return;case<-t.C:}
	}
}

func refresh(ctx context.Context,client *http.Client,state *cache,control,cachePath string,publicKey ed25519.PublicKey,keyID string)error{
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,control+"/api/v1/config/latest",nil);if err!=nil{return err}
	req.Header.Set("Accept","application/json")
	resp,err:=client.Do(req);if err!=nil{return err}
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusOK{return fmt.Errorf("control returned HTTP %d",resp.StatusCode)}
	raw,err:=io.ReadAll(io.LimitReader(resp.Body,1<<20));if err!=nil{return err}
	state.mu.RLock();minimum:=state.version;state.mu.RUnlock()
	meta,err:=verifyEnvelope(raw,publicKey,keyID,minimum,time.Now().UTC());if err!=nil{return err}
	tmp:=cachePath+".tmp"
	if err:=os.WriteFile(tmp,raw,0600);err!=nil{return err}
	if err:=os.Rename(tmp,cachePath);err!=nil{return err}
	state.mu.Lock();state.raw=append(state.raw[:0],raw...);state.version=meta.Version;state.expires=meta.ExpiresAt;state.mu.Unlock()
	return nil
}

func verifyEnvelope(raw []byte,publicKey ed25519.PublicKey,keyID string,minimum int64,now time.Time)(manifestMeta,error){
	var env envelope
	if err:=json.Unmarshal(raw,&env);err!=nil{return manifestMeta{},err}
	if env.KeyID!=keyID{return manifestMeta{},errors.New("unexpected signing key id")}
	payload,err:=base64.RawURLEncoding.DecodeString(env.Payload);if err!=nil{return manifestMeta{},err}
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature);if err!=nil{return manifestMeta{},err}
	if !ed25519.Verify(publicKey,payload,sig){return manifestMeta{},errors.New("invalid manifest signature")}
	var meta manifestMeta
	if err:=json.Unmarshal(payload,&meta);err!=nil{return manifestMeta{},err}
	if meta.SchemaVersion!=1||meta.Version<minimum{return manifestMeta{},errors.New("invalid or rolled back manifest")}
	if meta.CreatedAt.After(now.Add(5*time.Minute))||!meta.ExpiresAt.After(now){return manifestMeta{},errors.New("manifest outside validity window")}
	return meta,nil
}

func env(key,fallback string)string{
	if v:=strings.TrimSpace(os.Getenv(key));v!=""{return v}
	return fallback
}
