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

type manifestNode struct {
	ID string `json:"id"`
	Endpoints []struct {
		Kind string `json:"kind"`
		Scheme string `json:"scheme"`
		Host string `json:"host"`
		Port int `json:"port"`
		Path string `json:"path"`
	} `json:"endpoints"`
}

func Observe(manifest clientconfig.Envelope) ([]store.ProbeObservation,error) {
	var payload struct {
		Workers []manifestNode `json:"workers"`
		Ingresses []manifestNode `json:"ingresses"`
	}
	if err:=clientconfig.DecodePayload(manifest,&payload); err!=nil { return nil,err }

	results:=make([]store.ProbeObservation,0)
	httpClient:=&http.Client{Timeout:5*time.Second}

	for _,worker:=range payload.Workers {
		for _,ep:=range worker.Endpoints {
			if ep.Kind!="session_api" || ep.Scheme!="https" { continue }
			results=append(results,observeHTTPS(httpClient,worker.ID,"session_api",
				fmt.Sprintf("https://%s:%d%s",ep.Host,ep.Port,ep.Path)))
		}
	}
	for _,ingress:=range payload.Ingresses {
		for _,ep:=range ingress.Endpoints {
			if ep.Kind!="ingress" || ep.Scheme!="https" { continue }
			results=append(results,observeHTTPS(httpClient,ingress.ID,"ingress_https",
				fmt.Sprintf("https://%s:%d/__vpnx3/health",ep.Host,ep.Port)))
		}
	}
	return results,nil
}

func observeHTTPS(client *http.Client,nodeID,kind,url string) store.ProbeObservation {
	start:=time.Now()
	req,err:=http.NewRequest(http.MethodGet,url,nil)
	success:=false
	if err==nil {
		resp,doErr:=client.Do(req)
		if doErr==nil {
			success=resp.StatusCode==http.StatusOK
			_ = resp.Body.Close()
		}
	}
	return store.ProbeObservation{
		TargetNodeID:nodeID,
		EndpointKind:kind,
		Success:success,
		LatencyMS:int(time.Since(start).Milliseconds()),
	}
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
