package buildworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/venomimonstro/vpnx3/internal/nodeauth"
)

type Client struct {
	controlURL string
	http *http.Client
	id agent.Identity
	identityPath string
	sequencePath string
	sequence int64
}

type Job struct {
	ID string `json:"id"`
	ReleaseID string `json:"release_id"`
	Target string `json:"target"`
	Version string `json:"version"`
	SourceCommit string `json:"source_commit"`
	Attempt int `json:"attempt"`
}

func New(controlURL,identityPath string,id agent.Identity) *Client {
	sequencePath:=filepath.Join(filepath.Dir(identityPath),"build-sequence")
	return &Client{
		controlURL:strings.TrimRight(controlURL,"/"),
		http:&http.Client{Timeout:30*time.Second},
		id:id,
		identityPath:identityPath,
		sequencePath:sequencePath,
		sequence:loadSequence(sequencePath),
	}
}

func (c *Client) Claim(ctx context.Context,targets []string) (*Job,error) {
	body,err:=c.payload(map[string]any{"targets":targets})
	if err!=nil { return nil,err }
	path:="/api/v1/build/claim"
	resp,raw,err:=c.request(ctx,http.MethodPost,path,body)
	if err!=nil { return nil,err }
	if resp==http.StatusNoContent { return nil,nil }
	if resp!=http.StatusOK { return nil,fmt.Errorf("claim status=%d body=%s",resp,string(raw)) }
	var job Job
	if err:=json.Unmarshal(raw,&job); err!=nil { return nil,err }
	return &job,nil
}

func (c *Client) UploadArtifact(ctx context.Context,jobID,path string) error {
	f,err:=os.Open(path);if err!=nil{return err}
	h:=sha256.New()
	if _,err:=io.Copy(h,f);err!=nil{f.Close();return err}
	hash:=hex.EncodeToString(h.Sum(nil))
	if _,err:=f.Seek(0,io.SeekStart);err!=nil{f.Close();return err}

	c.sequence++
	if err:=saveSequence(c.sequencePath,c.sequence);err!=nil{f.Close();return err}
	endpoint:="/api/v1/build/jobs/"+jobID+"/artifact"
	ts:=strconv.FormatInt(time.Now().UTC().Unix(),10)
	key,err:=c.id.Private();if err!=nil{f.Close();return err}
	sig:=nodeauth.Sign(key,http.MethodPut,endpoint,ts,[]byte(hash))
	req,err:=http.NewRequestWithContext(ctx,http.MethodPut,c.controlURL+endpoint,f)
	if err!=nil{f.Close();return err}
	req.Header.Set("Content-Type","application/octet-stream")
	req.Header.Set("X-VPNX3-Node-ID",c.id.NodeID)
	req.Header.Set("X-VPNX3-Timestamp",ts)
	req.Header.Set("X-VPNX3-Signature",sig)
	req.Header.Set("X-VPNX3-Sequence",strconv.FormatInt(c.sequence,10))
	req.Header.Set("X-VPNX3-Content-SHA256",hash)
	resp,err:=c.http.Do(req);f.Close()
	if err!=nil{return err}
	defer resp.Body.Close()
	raw,_:=io.ReadAll(io.LimitReader(resp.Body,1<<20))
	if resp.StatusCode!=http.StatusCreated{return fmt.Errorf("artifact upload status=%d body=%s",resp.StatusCode,string(raw))}
	return nil
}

func (c *Client) Complete(ctx context.Context,jobID,status,errorSummary string) error {
	body,err:=c.payload(map[string]any{"status":status,"error_summary":errorSummary})
	if err!=nil { return err }
	path:="/api/v1/build/jobs/"+jobID+"/complete"
	resp,raw,err:=c.request(ctx,http.MethodPost,path,body)
	if err!=nil { return err }
	if resp!=http.StatusNoContent { return fmt.Errorf("complete status=%d body=%s",resp,string(raw)) }
	return nil
}

func (c *Client) payload(fields map[string]any) ([]byte,error) {
	c.sequence++
	if err:=saveSequence(c.sequencePath,c.sequence); err!=nil { return nil,err }
	fields["sequence"]=c.sequence
	return json.Marshal(fields)
}

func (c *Client) request(ctx context.Context,method,path string,body []byte) (int,[]byte,error) {
	ts:=strconv.FormatInt(time.Now().UTC().Unix(),10)
	key,err:=c.id.Private();if err!=nil{return 0,nil,err}
	sig:=nodeauth.Sign(key,method,path,ts,body)
	req,err:=http.NewRequestWithContext(ctx,method,c.controlURL+path,bytes.NewReader(body))
	if err!=nil{return 0,nil,err}
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("X-VPNX3-Node-ID",c.id.NodeID)
	req.Header.Set("X-VPNX3-Timestamp",ts)
	req.Header.Set("X-VPNX3-Signature",sig)
	resp,err:=c.http.Do(req);if err!=nil{return 0,nil,err}
	defer resp.Body.Close()
	raw,_:=io.ReadAll(io.LimitReader(resp.Body,1<<20))
	return resp.StatusCode,raw,nil
}

func loadSequence(path string) int64 {
	raw,err:=os.ReadFile(path);if err!=nil{return 0}
	v,err:=strconv.ParseInt(strings.TrimSpace(string(raw)),10,64)
	if err!=nil || v<0{return 0};return v
}
func saveSequence(path string,value int64) error {
	if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return err}
	tmp:=path+".tmp"
	if err:=os.WriteFile(tmp,[]byte(strconv.FormatInt(value,10)),0600);err!=nil{return err}
	return os.Rename(tmp,path)
}
