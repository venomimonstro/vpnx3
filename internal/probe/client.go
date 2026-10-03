package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/agent"
	"github.com/venomimonstro/vpnx3/internal/clientconfig"
	"github.com/venomimonstro/vpnx3/internal/nodeauth"
	"github.com/venomimonstro/vpnx3/internal/store"
)

type Client struct {
	controlURL   string
	http         *http.Client
	identity     agent.Identity
	sequencePath string
	sequence     int64
}

func New(controlURL,identityPath string,id agent.Identity) *Client {
	sequencePath:=filepath.Join(filepath.Dir(identityPath),"probe-sequence")
	return &Client{
		controlURL:strings.TrimRight(controlURL,"/"),
		http:&http.Client{Timeout:10*time.Second},
		identity:id,
		sequencePath:sequencePath,
		sequence:loadSequence(sequencePath),
	}
}

func (c *Client) Report(ctx context.Context,results []store.ProbeObservation) error {
	c.sequence++
	if err:=saveSequence(c.sequencePath,c.sequence); err!=nil {
		return fmt.Errorf("persist probe sequence: %w",err)
	}

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

func loadSequence(path string) int64 {
	raw,err:=os.ReadFile(path)
	if err!=nil { return 0 }
	v,err:=strconv.ParseInt(strings.TrimSpace(string(raw)),10,64)
	if err!=nil || v<0 { return 0 }
	return v
}

func saveSequence(path string,value int64) error {
	if err:=os.MkdirAll(filepath.Dir(path),0700); err!=nil { return err }
	tmp:=path+".tmp"
	if err:=os.WriteFile(tmp,[]byte(strconv.FormatInt(value,10)),0600); err!=nil { return err }
	return os.Rename(tmp,path)
}
