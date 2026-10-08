package accesslease

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Verifier struct {
	mu sync.RWMutex
	keys map[string]ed25519.PublicKey
	notAfter time.Time
}

func NewVerifier(publicKeyBase64 string) (*Verifier,error) {
	raw,err:=base64.RawURLEncoding.DecodeString(publicKeyBase64)
	if err!=nil || len(raw)!=ed25519.PublicKeySize { return nil,fmt.Errorf("invalid access public key") }
	sum:=sha256.Sum256(raw)
	return &Verifier{keys:map[string]ed25519.PublicKey{
		hex.EncodeToString(sum[:8]):ed25519.PublicKey(raw),
	}},nil
}

func NewVerifierSet(keys map[string]string)(*Verifier,error){
	v:=&Verifier{}
	if err:=v.ReplaceKeys(keys);err!=nil{return nil,err}
	return v,nil
}

func (v *Verifier) ReplaceKeys(keys map[string]string)error{
	return v.ReplaceKeysUntil(keys,time.Time{})
}

func (v *Verifier) ReplaceKeysUntil(keys map[string]string,notAfter time.Time)error{
	if len(keys)<1||len(keys)>8{return fmt.Errorf("access verifier key count outside safe bounds")}
	next:=make(map[string]ed25519.PublicKey,len(keys))
	for keyID,encoded:=range keys{
		raw,err:=base64.RawURLEncoding.DecodeString(encoded)
		if err!=nil||len(raw)!=ed25519.PublicKeySize{return fmt.Errorf("invalid access public key")}
		sum:=sha256.Sum256(raw)
		if keyID!=hex.EncodeToString(sum[:8]){return fmt.Errorf("access key id mismatch")}
		next[keyID]=ed25519.PublicKey(append([]byte(nil),raw...))
	}
	v.mu.Lock();v.keys=next;v.notAfter=notAfter.UTC();v.mu.Unlock()
	return nil
}

func (v *Verifier) Verify(env Envelope,now time.Time) (Claims,error) {
	v.mu.RLock()
	public,ok:=v.keys[env.KeyID]
	notAfter:=v.notAfter
	v.mu.RUnlock()
	if !notAfter.IsZero() && !notAfter.After(now){return Claims{},fmt.Errorf("access trust bundle expired")}
	if !ok { return Claims{},fmt.Errorf("unexpected access signing key") }

	payload,err:=base64.RawURLEncoding.DecodeString(env.Payload)
	if err!=nil { return Claims{},fmt.Errorf("invalid payload encoding") }
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature)
	if err!=nil { return Claims{},fmt.Errorf("invalid signature encoding") }
	if !ed25519.Verify(public,payload,sig) {
		return Claims{},fmt.Errorf("invalid access lease signature")
	}

	var claims Claims
	if err:=json.Unmarshal(payload,&claims); err!=nil { return Claims{},err }
	if claims.SchemaVersion!=1 || claims.LeaseID=="" || claims.DeviceID=="" || claims.UserID=="" || claims.TunnelPublicKey=="" {
		return Claims{},fmt.Errorf("invalid access lease claims")
	}
	if !claims.ExpiresAt.After(now) { return Claims{},fmt.Errorf("access lease expired") }
	if claims.IssuedAt.After(now.Add(2*time.Minute)) { return Claims{},fmt.Errorf("access lease issued in the future") }
	return claims,nil
}
