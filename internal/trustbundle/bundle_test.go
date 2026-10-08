package trustbundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
)

func signerForTest(t *testing.T)(*signing.Signer,string){
	t.Helper()
	_,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
	s,err:=signing.FromSeedBase64(base64.RawURLEncoding.EncodeToString(priv.Seed()));if err!=nil{t.Fatal(err)}
	return s,s.PublicKeyBase64()
}

func bundleKey(t *testing.T,purpose,state string)Key{
	t.Helper()
	_,pub:=signerForTest(t)
	raw,_:=base64.RawURLEncoding.DecodeString(pub)
	sum:=sha256.Sum256(raw)
	return Key{Purpose:purpose,KeyID:hex.EncodeToString(sum[:8]),Algorithm:"ed25519",PublicKey:pub,State:state}
}

func TestBundleRoundTripAndRollback(t *testing.T){
	root,_:=signerForTest(t)
	now:=time.Now().UTC().Truncate(time.Second)
	env,err:=Issue(root,Payload{
		SchemaVersion:1,Version:7,IssuedAt:now,ExpiresAt:now.Add(24*time.Hour),
		Keys:[]Key{bundleKey(t,"config","active"),bundleKey(t,"access","active"),bundleKey(t,"release","active")},
	})
	if err!=nil{t.Fatal(err)}
	v,err:=NewVerifier(root.PublicKeyBase64());if err!=nil{t.Fatal(err)}
	p,err:=v.Verify(env,7,now);if err!=nil{t.Fatal(err)}
	if p.Version!=7{t.Fatalf("unexpected version %d",p.Version)}
	if _,err:=v.Verify(env,8,now);err==nil{t.Fatal("expected rollback rejection")}
}

func TestBundleRequiresSingleActiveCoreKeys(t *testing.T){
	root,_:=signerForTest(t)
	now:=time.Now().UTC().Truncate(time.Second)
	_,err:=Issue(root,Payload{
		SchemaVersion:1,Version:1,IssuedAt:now,ExpiresAt:now.Add(time.Hour),
		Keys:[]Key{bundleKey(t,"config","active"),bundleKey(t,"config","active"),bundleKey(t,"access","active")},
	})
	if err==nil{t.Fatal("expected duplicate active config rejection")}
}
