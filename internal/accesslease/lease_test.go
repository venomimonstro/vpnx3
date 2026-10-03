package accesslease

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
)

func TestLeaseRoundTrip(t *testing.T) {
	_,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil { t.Fatal(err) }
	seed:=base64.RawURLEncoding.EncodeToString(priv.Seed())
	signer,err:=signing.FromSeedBase64(seed)
	if err!=nil { t.Fatal(err) }
	verifier,err:=NewVerifier(signer.PublicKeyBase64())
	if err!=nil { t.Fatal(err) }

	now:=time.Now().UTC().Truncate(time.Second)
	env,err:=Issue(signer,Claims{
		SchemaVersion:1,
		LeaseID:"lease-1",
		UserID:"user-1",
		DeviceID:"device-1",
		Entitlement:"trial",
		IssuedAt:now,
		ExpiresAt:now.Add(time.Hour),
	})
	if err!=nil { t.Fatal(err) }
	claims,err:=verifier.Verify(env,now)
	if err!=nil { t.Fatal(err) }
	if claims.DeviceID!="device-1" { t.Fatalf("unexpected device %s",claims.DeviceID) }
}

func TestExpiredLeaseRejected(t *testing.T) {
	_,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil { t.Fatal(err) }
	signer,_:=signing.FromSeedBase64(base64.RawURLEncoding.EncodeToString(priv.Seed()))
	verifier,_:=NewVerifier(signer.PublicKeyBase64())
	now:=time.Now().UTC().Truncate(time.Second)
	env,_:=Issue(signer,Claims{
		SchemaVersion:1,LeaseID:"x",UserID:"u",DeviceID:"d",Entitlement:"trial",
		IssuedAt:now.Add(-2*time.Hour),ExpiresAt:now.Add(-time.Hour),
	})
	if _,err:=verifier.Verify(env,now); err==nil { t.Fatal("expected expired lease rejection") }
}
