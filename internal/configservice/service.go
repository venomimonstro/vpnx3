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
	GatewayIPv4         string   `json:"gateway_ipv4"`
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
	ConfigMirrors []store.ConfigNode    `json:"config_mirrors"`
	Network       NetworkPolicy         `json:"network"`
	Features      map[string]bool       `json:"features"`
}

type Envelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
	KeyID     string `json:"key_id"`
}

const configPublishAdvisoryLock int64 = 0x56504e5833434647

func (s *Service) Publish(ctx context.Context,adminID string)(Envelope,error){
	var out Envelope
	err:=s.store.WithAdvisoryLock(ctx,configPublishAdvisoryLock,func(lockCtx context.Context)error{
		env,err:=s.publishUnlocked(lockCtx,adminID)
		if err==nil{out=env}
		return err
	})
	return out,err
}

func (s *Service) publishUnlocked(ctx context.Context, adminID string) (Envelope,error) {
	version,err:=s.store.NextConfigVersion(ctx)
	if err!=nil { return Envelope{},err }
	ingresses,err:=s.store.ActiveConfigNodes(ctx,"ingress")
	if err!=nil { return Envelope{},err }
	workers,err:=s.store.ActiveConfigNodes(ctx,"worker")
	if err!=nil { return Envelope{},err }
	mirrors,err:=s.store.ActiveConfigNodes(ctx,"config_mirror")
	if err!=nil { return Envelope{},err }

	now:=time.Now().UTC().Truncate(time.Second)
	manifest:=Manifest{
		SchemaVersion:1,
		Version:version,
		CreatedAt:now,
		ExpiresAt:now.Add(24*time.Hour),
		Ingresses:ingresses,
		Workers:workers,
		ConfigMirrors:mirrors,
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


func (s *Service) CleanupHistory(ctx context.Context) (int64,error) {
	return s.store.CleanupConfigManifests(ctx)
}
