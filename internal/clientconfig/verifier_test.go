package clientconfig

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestVerifierRejectsRollback(t *testing.T) {
	pub,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil { t.Fatal(err) }
	v,err:=NewVerifier(base64.RawURLEncoding.EncodeToString(pub))
	if err!=nil { t.Fatal(err) }
	now:=time.Now().UTC().Truncate(time.Second)
	payload,_:=json.Marshal(map[string]any{
		"schema_version":1,
		"version":4,
		"created_at":now,
		"expires_at":now.Add(time.Hour),
	})
	env:=Envelope{
		Manifest:payload,
		Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv,payload)),
		KeyID:v.keyID,
	}
	if _,err:=v.Verify(env,5,now); err==nil {
		t.Fatal("expected rollback rejection")
	}
}

func TestVerifierAcceptsValidManifest(t *testing.T) {
	pub,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil { t.Fatal(err) }
	v,_:=NewVerifier(base64.RawURLEncoding.EncodeToString(pub))
	now:=time.Now().UTC().Truncate(time.Second)
	payload,_:=json.Marshal(map[string]any{
		"schema_version":1,
		"version":5,
		"created_at":now,
		"expires_at":now.Add(time.Hour),
	})
	env:=Envelope{
		Manifest:payload,
		Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv,payload)),
		KeyID:v.keyID,
	}
	meta,err:=v.Verify(env,5,now)
	if err!=nil { t.Fatal(err) }
	if meta.Version!=5 { t.Fatalf("got version %d",meta.Version) }
}
