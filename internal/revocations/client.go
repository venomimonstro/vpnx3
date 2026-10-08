package revocations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct{
	url string
	http *http.Client
	verifier *Verifier
}

func NewClient(controlURL string,verifier *Verifier)(*Client,error){
	u,err:=url.Parse(strings.TrimSpace(controlURL))
	if err!=nil||u.Scheme!="https"||u.Hostname()==""||u.User!=nil{
		return nil,fmt.Errorf("revocation control URL must be credential-free https")
	}
	u.Path="/api/v1/access/revocations"
	u.RawQuery=""
	u.Fragment=""
	return &Client{
		url:u.String(),
		http:&http.Client{Timeout:8*time.Second},
		verifier:verifier,
	},nil
}

func (c *Client) Fetch(ctx context.Context,now time.Time)(Payload,error){
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,c.url,nil);if err!=nil{return Payload{},err}
	req.Header.Set("Accept","application/json")
	resp,err:=c.http.Do(req);if err!=nil{return Payload{},err}
	defer resp.Body.Close()
	raw,err:=io.ReadAll(io.LimitReader(resp.Body,2<<20));if err!=nil{return Payload{},err}
	if resp.StatusCode!=http.StatusOK{return Payload{},fmt.Errorf("revocation feed HTTP %d",resp.StatusCode)}
	var env Envelope
	if err:=json.Unmarshal(raw,&env);err!=nil{return Payload{},err}
	return c.verifier.Verify(env,now)
}
