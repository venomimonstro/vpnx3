package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accesslease"
	"github.com/venomimonstro/vpnx3/internal/nodeauth"
)

func (c *Client) SyntheticLease(ctx context.Context,tunnelPublicKey string)(accesslease.Envelope,error){
	c.leaseSequence++
	if err:=saveSequence(c.leaseSequencePath,c.leaseSequence);err!=nil{
		return accesslease.Envelope{},fmt.Errorf("persist probe lease sequence: %w",err)
	}

	body,err:=json.Marshal(map[string]any{
		"sequence":c.leaseSequence,
		"tunnel_public_key":tunnelPublicKey,
	})
	if err!=nil{return accesslease.Envelope{},err}

	path:="/api/v1/node/probe-lease"
	ts:=strconv.FormatInt(time.Now().UTC().Unix(),10)
	privateKey,err:=c.identity.Private()
	if err!=nil{return accesslease.Envelope{},err}
	sig:=nodeauth.Sign(privateKey,http.MethodPost,path,ts,body)

	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,c.controlURL+path,bytes.NewReader(body))
	if err!=nil{return accesslease.Envelope{},err}
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("X-VPNX3-Node-ID",c.identity.NodeID)
	req.Header.Set("X-VPNX3-Timestamp",ts)
	req.Header.Set("X-VPNX3-Signature",sig)

	resp,err:=c.http.Do(req)
	if err!=nil{return accesslease.Envelope{},err}
	defer resp.Body.Close()
	raw,err:=io.ReadAll(io.LimitReader(resp.Body,64<<10))
	if err!=nil{return accesslease.Envelope{},err}
	if resp.StatusCode!=http.StatusOK{
		return accesslease.Envelope{},fmt.Errorf("probe lease failed status=%d body=%s",resp.StatusCode,string(raw))
	}
	var env accesslease.Envelope
	if err:=json.Unmarshal(raw,&env);err!=nil{return accesslease.Envelope{},err}
	return env,nil
}
