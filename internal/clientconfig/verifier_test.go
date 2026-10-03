package clientconfig

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func signedEnvelope(t *testing.T,version int64,now time.Time) (*Verifier,Envelope) {
	t.Helper()
	pub,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil { t.Fatal(err) }
	v,err:=NewVerifier(base64.RawURLEncoding.EncodeToString(pub))
	if err!=nil { t.Fatal(err) }
	payload,_:=json.Marshal(map[string]any{
		"schema_version":1,
		"version":version,
		"created_at":now,
		"expires_at":now.Add(time.Hour),
	})
	return v,Envelope{
		Payload:base64.RawURLEncoding.EncodeToString(payload),
		Signature:base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv,payload)),
		KeyID:v.keyID,
	}
}

func TestVerifierRejectsRollback(t *testing.T) {
	now:=time.Now().UTC().Truncate(time.Second)
	v,env:=signedEnvelope(t,4,now)
	if _,err:=v.Verify(env,5,now); err==nil { t.Fatal("expected rollback rejection") }
}

func TestVerifierAcceptsValidManifest(t *testing.T) {
	now:=time.Now().UTC().Truncate(time.Second)
	v,env:=signedEnvelope(t,5,now)
	meta,err:=v.Verify(env,5,now)
	if err!=nil { t.Fatal(err) }
	if meta.Version!=5 { t.Fatalf("got version %d",meta.Version) }
}
