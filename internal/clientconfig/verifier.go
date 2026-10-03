package clientconfig

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Envelope struct {
	Manifest  json.RawMessage `json:"manifest"`
	Signature string          `json:"signature"`
	KeyID     string          `json:"key_id"`
}

type ManifestMeta struct {
	SchemaVersion int       `json:"schema_version"`
	Version       int64     `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type Verifier struct {
	public ed25519.PublicKey
	keyID string
}

func NewVerifier(publicKeyBase64 string) (*Verifier,error) {
	raw,err:=base64.RawURLEncoding.DecodeString(publicKeyBase64)
	if err!=nil { return nil,fmt.Errorf("decode public key: %w",err) }
	if len(raw)!=ed25519.PublicKeySize { return nil,fmt.Errorf("invalid public key length") }
	sum:=sha256.Sum256(raw)
	return &Verifier{public:ed25519.PublicKey(raw),keyID:hex.EncodeToString(sum[:8])},nil
}

func (v *Verifier) Verify(env Envelope,minimumVersion int64,now time.Time) (ManifestMeta,error) {
	if env.KeyID!=v.keyID {
		return ManifestMeta{},fmt.Errorf("unexpected signing key")
	}
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature)
	if err!=nil { return ManifestMeta{},fmt.Errorf("decode signature: %w",err) }
	if !ed25519.Verify(v.public,env.Manifest,sig) {
		return ManifestMeta{},fmt.Errorf("invalid manifest signature")
	}
	var meta ManifestMeta
	if err:=json.Unmarshal(env.Manifest,&meta); err!=nil {
		return ManifestMeta{},fmt.Errorf("decode manifest metadata: %w",err)
	}
	if meta.SchemaVersion!=1 { return ManifestMeta{},fmt.Errorf("unsupported schema version %d",meta.SchemaVersion) }
	if meta.Version<=0 { return ManifestMeta{},fmt.Errorf("invalid manifest version") }
	if minimumVersion>0 && meta.Version<minimumVersion {
		return ManifestMeta{},fmt.Errorf("manifest rollback detected: got %d, minimum %d",meta.Version,minimumVersion)
	}
	if !meta.ExpiresAt.After(now) {
		return ManifestMeta{},fmt.Errorf("manifest expired")
	}
	if meta.CreatedAt.After(now.Add(5*time.Minute)) {
		return ManifestMeta{},fmt.Errorf("manifest created in the future")
	}
	return meta,nil
}
