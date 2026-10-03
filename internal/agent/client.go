package agent

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

	"github.com/venomimonstro/vpnx3/internal/nodeauth"
)

type Config struct {
	ControlURL      string
	EnrollmentToken string
	NodeName        string
	Provider        string
	CountryCode     string
	PublicIP        string
	Capacity        int
	AgentVersion    string
	IdentityPath    string
}

type Client struct {
	cfg Config
	http *http.Client
	id Identity
}

func New(cfg Config,id Identity) *Client {
	return &Client{
		cfg:cfg,
		id:id,
		http:&http.Client{Timeout:15*time.Second},
	}
}

func (c *Client) Enroll(ctx context.Context) error {
	if c.id.NodeID != "" { return nil }
	payload := map[string]any{
		"token":c.cfg.EnrollmentToken,
		"name":c.cfg.NodeName,
		"public_key":c.id.PublicKey,
		"provider":c.cfg.Provider,
		"country_code":c.cfg.CountryCode,
		"public_ip":c.cfg.PublicIP,
		"capacity_sessions":c.cfg.Capacity,
		"agent_version":c.cfg.AgentVersion,
	}
	body,_ := json.Marshal(payload)
	req,err := http.NewRequestWithContext(ctx,http.MethodPost,strings.TrimRight(c.cfg.ControlURL,"/")+"/api/v1/node/enroll",bytes.NewReader(body))
	if err != nil { return err }
	req.Header.Set("Content-Type","application/json")
	resp,err := c.http.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	raw,_ := io.ReadAll(io.LimitReader(resp.Body,1<<20))
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("enrollment failed: status=%d body=%s",resp.StatusCode,string(raw))
	}
	var out struct{ NodeID string `json:"node_id"` }
	if err := json.Unmarshal(raw,&out); err != nil { return err }
	if out.NodeID == "" { return fmt.Errorf("empty node id") }
	c.id.NodeID = out.NodeID
	return Save(c.cfg.IdentityPath,c.id)
}

func (c *Client) Heartbeat(ctx context.Context,metadata map[string]any) error {
	c.id.Sequence++
	payload := map[string]any{
		"sequence":c.id.Sequence,
		"current_sessions":0,
		"capacity_sessions":c.cfg.Capacity,
		"agent_version":c.cfg.AgentVersion,
		"metadata":metadata,
	}
	body,_ := json.Marshal(payload)
	path := "/api/v1/node/heartbeat"
	ts := strconv.FormatInt(time.Now().UTC().Unix(),10)
	privateKey,err := c.id.Private()
	if err != nil { return err }
	sig := nodeauth.Sign(privateKey,http.MethodPost,path,ts,body)

	req,err := http.NewRequestWithContext(ctx,http.MethodPost,strings.TrimRight(c.cfg.ControlURL,"/")+path,bytes.NewReader(body))
	if err != nil { return err }
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("X-VPNX3-Node-ID",c.id.NodeID)
	req.Header.Set("X-VPNX3-Timestamp",ts)
	req.Header.Set("X-VPNX3-Signature",sig)

	resp,err := c.http.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw,_ := io.ReadAll(io.LimitReader(resp.Body,1<<20))
		return fmt.Errorf("heartbeat failed: status=%d body=%s",resp.StatusCode,string(raw))
	}
	return Save(c.cfg.IdentityPath,c.id)
}
