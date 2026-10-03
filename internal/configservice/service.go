package configservice

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
	"github.com/venomimonstro/vpnx3/internal/store"
)

type NetworkPolicy struct {
	DNSServers          []string `json:"dns_servers"`
	MTU                 int      `json:"mtu"`
	PersistentKeepalive int      `json:"persistent_keepalive_seconds"`
}

type Service struct {
	store  *store.Store
	signer *signing.Signer
	policy NetworkPolicy
}

func New(s *store.Store, signer *signing.Signer, policy NetworkPolicy) *Service {
	return &Service{store:s, signer:signer, policy:policy}
}

type Manifest struct {
	SchemaVersion int                   `json:"schema_version"`
	Version       int64                 `json:"version"`
	CreatedAt     time.Time             `json:"created_at"`
	ExpiresAt     time.Time             `json:"expires_at"`
	Ingresses     []store.ConfigNode    `json:"ingresses"`
	Workers       []store.ConfigNode    `json:"workers"`
	Network       NetworkPolicy         `json:"network"`
	Features      map[string]bool       `json:"features"`
}

type Envelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
	KeyID     string `json:"key_id"`
}

func (s *Service) Publish(ctx context.Context, adminID string) (Envelope,error) {
	version,err:=s.store.NextConfigVersion(ctx)
	if err!=nil { return Envelope{},err }
	ingresses,err:=s.store.ActiveConfigNodes(ctx,"ingress")
	if err!=nil { return Envelope{},err }
	workers,err:=s.store.ActiveConfigNodes(ctx,"worker")
	if err!=nil { return Envelope{},err }

	now:=time.Now().UTC().Truncate(time.Second)
	manifest:=Manifest{
		SchemaVersion:1,
		Version:version,
		CreatedAt:now,
		ExpiresAt:now.Add(24*time.Hour),
		Ingresses:ingresses,
		Workers:workers,
		Network:s.policy,
		Features:map[string]bool{"automatic_routing":true},
	}
	payload,err:=json.Marshal(manifest)
	if err!=nil { return Envelope{},fmt.Errorf("marshal manifest: %w",err) }
	sig:=s.signer.Sign(payload)
	if err:=s.store.SaveConfigManifest(ctx,version,payload,sig,s.signer.KeyID(),adminID); err!=nil {
		return Envelope{},err
	}
	return encodeEnvelope(payload,sig,s.signer.KeyID()),nil
}

func (s *Service) Latest(ctx context.Context) (Envelope,error) {
	m,err:=s.store.LatestConfigManifest(ctx)
	if err!=nil { return Envelope{},err }
	return encodeEnvelope(m.Payload,m.Signature,m.KeyID),nil
}
