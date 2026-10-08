package trustbundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Client struct {
	mu sync.Mutex
	sources []string
	http *http.Client
	verifier *Verifier
	statePath string
	minimumVersion int64
}

func NewClient(sources []string,rootPublicKey,statePath string)(*Client,error){
	verifier,err:=NewVerifier(rootPublicKey)
	if err!=nil{return nil,err}
	normalized:=make([]string,0,len(sources))
	seen:=map[string]struct{}{}
	for _,raw:=range sources{
		raw=strings.TrimSpace(raw)
		if raw==""{continue}
		u,err:=url.Parse(raw)
		if err!=nil||u.Scheme!="https"||u.Hostname()==""||u.User!=nil||u.Fragment!=""{
			return nil,fmt.Errorf("trust source must be credential-free https")
		}
		if u.Path==""||u.Path=="/"{u.Path="/api/v1/trust/bundle"}
		u.RawQuery=""
		value:=u.String()
		if _,ok:=seen[value];ok{continue}
		seen[value]=struct{}{}
		normalized=append(normalized,value)
	}
	if len(normalized)==0{return nil,fmt.Errorf("at least one trust source is required")}
	return &Client{
		sources:normalized,
		http:&http.Client{
			Timeout:8*time.Second,
			CheckRedirect:func(_ *http.Request,_ []*http.Request)error{return http.ErrUseLastResponse},
		},
		verifier:verifier,statePath:strings.TrimSpace(statePath),
	},nil
}

func (c *Client) LoadStored(now time.Time)(Payload,bool,error){
	c.mu.Lock();defer c.mu.Unlock()
	if c.statePath==""{return Payload{},false,nil}
	raw,err:=os.ReadFile(c.statePath)
	if os.IsNotExist(err){return Payload{},false,nil}
	if err!=nil{return Payload{},false,err}
	var env Envelope
	if err:=json.Unmarshal(raw,&env);err!=nil{return Payload{},false,fmt.Errorf("decode stored trust bundle: %w",err)}
	p,err:=c.verifier.Verify(env,0,now)
	if err!=nil{return Payload{},false,fmt.Errorf("verify stored trust bundle: %w",err)}
	c.minimumVersion=p.Version
	return p,true,nil
}

func (c *Client) Fetch(ctx context.Context,now time.Time)(Payload,error){
	c.mu.Lock();defer c.mu.Unlock()
	var last error
	for _,source:=range c.sources{
		p,env,err:=c.fetchOne(ctx,source,now)
		if err!=nil{last=err;continue}
		if err:=c.persist(env);err!=nil{return Payload{},err}
		if p.Version>c.minimumVersion{c.minimumVersion=p.Version}
		return p,nil
	}
	if last==nil{last=fmt.Errorf("trust sources unavailable")}
	return Payload{},last
}

func (c *Client) fetchOne(ctx context.Context,source string,now time.Time)(Payload,Envelope,error){
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,source,nil)
	if err!=nil{return Payload{},Envelope{},err}
	req.Header.Set("Accept","application/json")
	req.Header.Set("Cache-Control","no-cache")
	resp,err:=c.http.Do(req)
	if err!=nil{return Payload{},Envelope{},err}
	defer resp.Body.Close()
	raw,err:=io.ReadAll(io.LimitReader(resp.Body,1<<20))
	if err!=nil{return Payload{},Envelope{},err}
	if resp.StatusCode!=http.StatusOK{return Payload{},Envelope{},fmt.Errorf("trust source HTTP %d",resp.StatusCode)}
	var env Envelope
	if err:=json.Unmarshal(raw,&env);err!=nil{return Payload{},Envelope{},err}
	p,err:=c.verifier.Verify(env,c.minimumVersion,now)
	if err!=nil{return Payload{},Envelope{},err}
	return p,env,nil
}

func (c *Client) persist(env Envelope)error{
	if c.statePath==""{return nil}
	raw,err:=json.Marshal(env);if err!=nil{return err}
	if err:=os.MkdirAll(filepath.Dir(c.statePath),0700);err!=nil{return err}
	tmp:=c.statePath+".tmp"
	if err:=os.WriteFile(tmp,raw,0600);err!=nil{return err}
	if err:=os.Rename(tmp,c.statePath);err!=nil{return err}
	return nil
}

func VerificationKeys(p Payload,purpose string)map[string]string{
	out:=map[string]string{}
	for _,key:=range p.Keys{
		if key.Purpose!=purpose{continue}
		if key.State!="active"&&key.State!="retired"{continue}
		out[key.KeyID]=key.PublicKey
	}
	return out
}
