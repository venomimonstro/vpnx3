package revocation

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
)

func testSigner(t *testing.T) *signing.Signer {
	t.Helper()
	_,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	s,err:=signing.FromSeedBase64(base64.RawURLEncoding.EncodeToString(priv.Seed()))
	if err!=nil{t.Fatal(err)}
	return s
}

func TestVersionedSnapshotRejectsRollback(t *testing.T){
	signer:=testSigner(t)
	verifier,err:=NewVerifier(signer.PublicKeyBase64())
	if err!=nil{t.Fatal(err)}
	now:=time.Now().UTC().Truncate(time.Second)

	env,err:=Issue(signer,4,[]string{"device-a"},now,10*time.Minute)
	if err!=nil{t.Fatal(err)}
	if _,err:=verifier.Verify(env,5,now);err==nil{
		t.Fatal("expected rollback rejection")
	}
}

func TestLKGUsableIsBounded(t *testing.T){
	now:=time.Now().UTC().Truncate(time.Second)
	p:=Payload{GeneratedAt:now.Add(-20*time.Minute),ExpiresAt:now.Add(-5*time.Minute)}
	if !LKGUsable(p,now,10*time.Minute){
		t.Fatal("expected snapshot within grace to be usable")
	}
	if LKGUsable(p,now,4*time.Minute){
		t.Fatal("expected snapshot outside grace to be rejected")
	}

	veryOld:=Payload{GeneratedAt:now.Add(-4*time.Hour),ExpiresAt:now.Add(-3*time.Hour)}
	if LKGUsable(veryOld,now,24*time.Hour){
		t.Fatal("grace must be capped at two hours")
	}
}

func TestDeviceHashStableAndDomainSeparated(t *testing.T){
	a:=DeviceHash("device-1")
	b:=DeviceHash("device-1")
	c:=DeviceHash("device-2")
	if a!=b{t.Fatal("device hash must be stable")}
	if a==c{t.Fatal("different devices must not share hash")}
	if len(a)!=64{t.Fatalf("unexpected hash length %d",len(a))}
}
