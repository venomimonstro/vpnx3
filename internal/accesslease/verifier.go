package accesslease

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Verifier struct {
	public ed25519.PublicKey
	keyID string
}

func NewVerifier(publicKeyBase64 string) (*Verifier,error) {
	raw,err:=base64.RawURLEncoding.DecodeString(publicKeyBase64)
	if err!=nil || len(raw)!=ed25519.PublicKeySize { return nil,fmt.Errorf("invalid access public key") }
	sum:=sha256.Sum256(raw)
	return &Verifier{public:ed25519.PublicKey(raw),keyID:hex.EncodeToString(sum[:8])},nil
}

func (v *Verifier) Verify(env Envelope,now time.Time) (Claims,error) {
	if env.KeyID!=v.keyID { return Claims{},fmt.Errorf("unexpected access signing key") }
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature)
	if err!=nil { return Claims{},fmt.Errorf("invalid signature encoding") }
	if !ed25519.Verify(v.public,env.Claims,sig) { return Claims{},fmt.Errorf("invalid access lease signature") }
	var claims Claims
	if err:=json.Unmarshal(env.Claims,&claims); err!=nil { return Claims{},err }
	if claims.SchemaVersion!=1 || claims.LeaseID=="" || claims.DeviceID=="" || claims.UserID=="" {
		return Claims{},fmt.Errorf("invalid access lease claims")
	}
	if !claims.ExpiresAt.After(now) { return Claims{},fmt.Errorf("access lease expired") }
	if claims.IssuedAt.After(now.Add(2*time.Minute)) { return Claims{},fmt.Errorf("access lease issued in the future") }
	return claims,nil
}
