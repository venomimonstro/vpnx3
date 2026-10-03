package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/agent"
	"github.com/venomimonstro/vpnx3/internal/clientconfig"
	"github.com/venomimonstro/vpnx3/internal/nodeauth"
	"github.com/venomimonstro/vpnx3/internal/store"
)

type Client struct {
	controlURL string
	http *http.Client
	identityPath string
	identity agent.Identity
	sequence int64
}

func New(controlURL,identityPath string,id agent.Identity) *Client {
	return &Client{
		controlURL:strings.TrimRight(controlURL,"/"),
		http:&http.Client{Timeout:10*time.Second},
		identityPath:identityPath,
		identity:id,
	}
}

func (c *Client) Report(ctx context.Context,results []store.ProbeObservation) error {
	c.sequence++
	body,err:=json.Marshal(map[string]any{"sequence":c.sequence,"results":results})
	if err!=nil { return err }

	path:="/api/v1/node/probe-results"
	ts:=strconv.FormatInt(time.Now().UTC().Unix(),10)
	privateKey,err:=c.identity.Private()
	if err!=nil { return err }
	sig:=nodeauth.Sign(privateKey,http.MethodPost,path,ts,body)

	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,c.controlURL+path,bytes.NewReader(body))
	if err!=nil { return err }
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("X-VPNX3-Node-ID",c.identity.NodeID)
	req.Header.Set("X-VPNX3-Timestamp",ts)
	req.Header.Set("X-VPNX3-Signature",sig)
	resp,err:=c.http.Do(req)
	if err!=nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusNoContent {
		raw,_:=io.ReadAll(io.LimitReader(resp.Body,64<<10))
		return fmt.Errorf("probe report failed status=%d body=%s",resp.StatusCode,string(raw))
	}
	return nil
}

func Observe(manifest clientconfig.Envelope) ([]store.ProbeObservation,error) {
	var payload struct {
		Workers []struct {
			ID string `json:"id"`
			Endpoints []struct {
				Kind string `json:"kind"`
				Scheme string `json:"scheme"`
				Host string `json:"host"`
				Port int `json:"port"`
				Path string `json:"path"`
			} `json:"endpoints"`
		} `json:"workers"`
	}
	if err:=clientconfig.DecodePayload(manifest,&payload); err!=nil { return nil,err }

	results:=make([]store.ProbeObservation,0)
	httpClient:=&http.Client{Timeout:5*time.Second}
	for _,worker:=range payload.Workers {
		for _,ep:=range worker.Endpoints {
			if ep.Kind!="session_api" || ep.Scheme!="https" { continue }
			url:=fmt.Sprintf("https://%s:%d%s",ep.Host,ep.Port,ep.Path)
			start:=time.Now()
			req,err:=http.NewRequest(http.MethodGet,url,nil)
			success:=false
			if err==nil {
				resp,doErr:=httpClient.Do(req)
				if doErr==nil {
					success=resp.StatusCode<500
					_ = resp.Body.Close()
				}
			}
			latency:=int(time.Since(start).Milliseconds())
			results=append(results,store.ProbeObservation{
				TargetNodeID:worker.ID,
				EndpointKind:"session_api",
				Success:success,
				LatencyMS:latency,
			})
		}
	}
	return results,nil
}
