package accesslease

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
)

func rotationSigner(t *testing.T)*signing.Signer{
	t.Helper()
	_,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
	s,err:=signing.FromSeedBase64(base64.RawURLEncoding.EncodeToString(priv.Seed()));if err!=nil{t.Fatal(err)}
	return s
}

func testLease(t *testing.T,s *signing.Signer,now time.Time)Envelope{
	t.Helper()
	env,err:=Issue(s,Claims{
		SchemaVersion:1,LeaseID:"lease",UserID:"user",DeviceID:"device",
		Entitlement:"paid",TunnelPublicKey:"client-key",IssuedAt:now,ExpiresAt:now.Add(time.Hour),
	})
	if err!=nil{t.Fatal(err)}
	return env
}

func TestVerifierKeyRotationOverlapAndExpiry(t *testing.T){
	oldKey:=rotationSigner(t)
	newKey:=rotationSigner(t)
	now:=time.Now().UTC().Truncate(time.Second)

	v,err:=NewVerifierSet(map[string]string{
		oldKey.KeyID():oldKey.PublicKeyBase64(),
		newKey.KeyID():newKey.PublicKeyBase64(),
	})
	if err!=nil{t.Fatal(err)}
	if err:=v.ReplaceKeysUntil(map[string]string{
		oldKey.KeyID():oldKey.PublicKeyBase64(),
		newKey.KeyID():newKey.PublicKeyBase64(),
	},now.Add(2*time.Hour));err!=nil{t.Fatal(err)}

	if _,err:=v.Verify(testLease(t,oldKey,now),now);err!=nil{t.Fatalf("old overlap key rejected: %v",err)}
	if _,err:=v.Verify(testLease(t,newKey,now),now);err!=nil{t.Fatalf("new active key rejected: %v",err)}

	if err:=v.ReplaceKeysUntil(map[string]string{
		newKey.KeyID():newKey.PublicKeyBase64(),
	},now.Add(2*time.Hour));err!=nil{t.Fatal(err)}
	if _,err:=v.Verify(testLease(t,oldKey,now),now);err==nil{t.Fatal("retired key must be rejected after removal")}
	if _,err:=v.Verify(testLease(t,newKey,now),now);err!=nil{t.Fatal(err)}
	if _,err:=v.Verify(testLease(t,newKey,now),now.Add(3*time.Hour));err==nil{
		t.Fatal("expired trust bundle must fail closed")
	}
}
