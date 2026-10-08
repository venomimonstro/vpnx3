package sessions

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"sync"
	"testing"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accesslease"
	"github.com/venomimonstro/vpnx3/internal/ipam"
	"github.com/venomimonstro/vpnx3/internal/revocation"
	"github.com/venomimonstro/vpnx3/internal/signing"
	"github.com/venomimonstro/vpnx3/network/transport"
)

type fakeAdapter struct{
	mu sync.Mutex
	closed []string
}

func (f *fakeAdapter) Name() string{return "fake"}
func (f *fakeAdapter) Healthy(context.Context) error{return nil}
func (f *fakeAdapter) CreateSession(_ context.Context,r transport.SessionRequest)(transport.SessionConfig,error){
	return transport.SessionConfig{
		Transport:"fake",AssignedIP:r.AssignedIP,ServerPublicKey:"server",
		Endpoint:"127.0.0.1:1",ExpiresAt:r.ExpiresAt,
	},nil
}
func (f *fakeAdapter) CloseSession(_ context.Context,publicKey string)error{
	f.mu.Lock();defer f.mu.Unlock()
	f.closed=append(f.closed,publicKey)
	return nil
}

func leaseFixture(t *testing.T,deviceID,publicKey string,now time.Time)(*accesslease.Verifier,accesslease.Envelope){
	t.Helper()
	_,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	signer,err:=signing.FromSeedBase64(base64.RawURLEncoding.EncodeToString(priv.Seed()))
	if err!=nil{t.Fatal(err)}
	verifier,err:=accesslease.NewVerifier(signer.PublicKeyBase64())
	if err!=nil{t.Fatal(err)}
	env,err:=accesslease.Issue(signer,accesslease.Claims{
		SchemaVersion:1,LeaseID:"lease-1",UserID:"user-1",DeviceID:deviceID,
		Entitlement:"paid",TunnelPublicKey:publicKey,
		IssuedAt:now,ExpiresAt:now.Add(time.Hour),
	})
	if err!=nil{t.Fatal(err)}
	return verifier,env
}

func TestRevocationClosesActiveSessionAndBlocksNew(t *testing.T){
	now:=time.Now().UTC().Truncate(time.Second)
	verifier,env:=leaseFixture(t,"device-1","client-key",now)
	pool,err:=ipam.New("10.66.0.0/29")
	if err!=nil{t.Fatal(err)}
	adapter:=&fakeAdapter{}
	m:=New(verifier,pool,adapter,"")

	ctx:=context.Background()
	if _,err:=m.Start(ctx,env,"client-key",now);err!=nil{t.Fatal(err)}
	if m.Count()!=1{t.Fatalf("expected one session, got %d",m.Count())}

	closed,err:=m.ApplyRevokedDeviceHashes(ctx,[]string{revocation.DeviceHash("device-1")})
	if err!=nil{t.Fatal(err)}
	if closed!=1{t.Fatalf("expected one closed session, got %d",closed)}
	if m.Count()!=0{t.Fatalf("expected zero sessions, got %d",m.Count())}

	if _,err:=m.Start(ctx,env,"client-key",now.Add(time.Minute));err==nil{
		t.Fatal("expected revoked device to be blocked")
	}
}

func TestCloseAllRemovesEverySession(t *testing.T){
	now:=time.Now().UTC().Truncate(time.Second)
	_,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	signer,err:=signing.FromSeedBase64(base64.RawURLEncoding.EncodeToString(priv.Seed()))
	if err!=nil{t.Fatal(err)}
	verifier,err:=accesslease.NewVerifier(signer.PublicKeyBase64())
	if err!=nil{t.Fatal(err)}
	pool,err:=ipam.New("10.66.0.0/29")
	if err!=nil{t.Fatal(err)}
	adapter:=&fakeAdapter{}
	m:=New(verifier,pool,adapter,"")

	for i,device:=range []string{"device-1","device-2"}{
		pub:="client-"+device
		env,err:=accesslease.Issue(signer,accesslease.Claims{
			SchemaVersion:1,LeaseID:"lease-"+string(rune('a'+i)),UserID:"user-1",
			DeviceID:device,Entitlement:"paid",TunnelPublicKey:pub,
			IssuedAt:now,ExpiresAt:now.Add(time.Hour),
		})
		if err!=nil{t.Fatal(err)}
		if _,err:=m.Start(context.Background(),env,pub,now);err!=nil{t.Fatal(err)}
	}
	if m.Count()!=2{t.Fatalf("expected two sessions, got %d",m.Count())}
	closed,err:=m.CloseAll(context.Background())
	if err!=nil{t.Fatal(err)}
	if closed!=2||m.Count()!=0{t.Fatalf("close all mismatch closed=%d count=%d",closed,m.Count())}
}
