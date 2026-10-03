package signing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

type Signer struct {
	private ed25519.PrivateKey
	public ed25519.PublicKey
	keyID string
}

func FromSeedBase64(raw string) (*Signer,error) {
	seed,err:=base64.RawURLEncoding.DecodeString(raw)
	if err!=nil { return nil,fmt.Errorf("decode signing seed: %w",err) }
	if len(seed)!=ed25519.SeedSize { return nil,fmt.Errorf("signing seed must be 32 bytes") }
	private:=ed25519.NewKeyFromSeed(seed)
	public:=private.Public().(ed25519.PublicKey)
	sum:=sha256.Sum256(public)
	return &Signer{private:private,public:public,keyID:hex.EncodeToString(sum[:8])},nil
}

func (s *Signer) Sign(payload []byte) []byte {
	return ed25519.Sign(s.private,payload)
}
func (s *Signer) KeyID() string { return s.keyID }
func (s *Signer) PublicKeyBase64() string {
	return base64.RawURLEncoding.EncodeToString(s.public)
}
