package clientconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Fetcher struct {
	Client *http.Client
	Verifier *Verifier
	Cache Cache
}

func NewFetcher(verifier *Verifier,cachePath string) *Fetcher {
	return &Fetcher{
		Client:&http.Client{Timeout:10*time.Second},
		Verifier:verifier,
		Cache:Cache{Path:cachePath,Verifier:verifier},
	}
}

func (f *Fetcher) Fetch(ctx context.Context,url string,now time.Time) (Envelope,ManifestMeta,error) {
	minVersion:=int64(0)
	if _,meta,err:=f.Cache.Load(now); err==nil {
		minVersion=meta.Version
	}

	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,url,nil)
	if err!=nil { return Envelope{},ManifestMeta{},err }
	resp,err:=f.Client.Do(req)
	if err!=nil {
		return f.loadFallback(now,fmt.Errorf("fetch config: %w",err))
	}
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusOK {
		return f.loadFallback(now,fmt.Errorf("config endpoint status %d",resp.StatusCode))
	}
	raw,err:=io.ReadAll(io.LimitReader(resp.Body,2<<20))
	if err!=nil { return f.loadFallback(now,err) }
	var env Envelope
	if err:=json.Unmarshal(raw,&env); err!=nil {
		return f.loadFallback(now,fmt.Errorf("decode config envelope: %w",err))
	}
	meta,err:=f.Cache.Save(env,minVersion,now)
	if err!=nil { return f.loadFallback(now,err) }
	return env,meta,nil
}

func (f *Fetcher) loadFallback(now time.Time,cause error) (Envelope,ManifestMeta,error) {
	env,meta,err:=f.Cache.Load(now)
	if err!=nil {
		return Envelope{},ManifestMeta{},fmt.Errorf("%v; fallback unavailable: %w",cause,err)
	}
	return env,meta,nil
}
