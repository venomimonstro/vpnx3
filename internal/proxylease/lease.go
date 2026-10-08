package proxylease

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
)

const Scope="browser_proxy"

type Claims struct {
	SchemaVersion int `json:"schema_version"`
	Scope string `json:"scope"`
	LeaseID string `json:"lease_id"`
	UserID string `json:"user_id"`
	DeviceID string `json:"device_id"`
	Entitlement string `json:"entitlement"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Envelope struct {
	Payload string `json:"payload"`
	Signature string `json:"signature"`
	KeyID string `json:"key_id"`
}

func Issue(signer *signing.Signer,claims Claims)(Envelope,error){
	claims.Scope=Scope
	raw,err:=json.Marshal(claims);if err!=nil{return Envelope{},err}
	sig:=signer.Sign(raw)
	return Envelope{
		Payload:base64.RawURLEncoding.EncodeToString(raw),
		Signature:base64.RawURLEncoding.EncodeToString(sig),
		KeyID:signer.KeyID(),
	},nil
}

type Verifier struct{
	mu sync.RWMutex
	keys map[string]ed25519.PublicKey
}

func NewVerifier(publicKeyBase64 string)(*Verifier,error){
	raw,err:=base64.RawURLEncoding.DecodeString(publicKeyBase64)
	if err!=nil||len(raw)!=ed25519.PublicKeySize{return nil,fmt.Errorf("invalid proxy lease public key")}
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
	if len(keys)<1||len(keys)>8{return fmt.Errorf("proxy verifier key count outside safe bounds")}
	next:=make(map[string]ed25519.PublicKey,len(keys))
	for keyID,encoded:=range keys{
		raw,err:=base64.RawURLEncoding.DecodeString(encoded)
		if err!=nil||len(raw)!=ed25519.PublicKeySize{return fmt.Errorf("invalid proxy lease public key")}
		sum:=sha256.Sum256(raw)
		if keyID!=hex.EncodeToString(sum[:8]){return fmt.Errorf("proxy key id mismatch")}
		next[keyID]=ed25519.PublicKey(append([]byte(nil),raw...))
	}
	v.mu.Lock();v.keys=next;v.mu.Unlock()
	return nil
}

func (v *Verifier) Verify(env Envelope,now time.Time)(Claims,error){
	v.mu.RLock();public,ok:=v.keys[env.KeyID];v.mu.RUnlock()
	if !ok{return Claims{},fmt.Errorf("unexpected proxy lease signing key")}
	payload,err:=base64.RawURLEncoding.DecodeString(env.Payload);if err!=nil{return Claims{},fmt.Errorf("invalid payload")}
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature);if err!=nil{return Claims{},fmt.Errorf("invalid signature")}
	if !ed25519.Verify(public,payload,sig){return Claims{},fmt.Errorf("invalid proxy lease signature")}
	var claims Claims
	if err:=json.Unmarshal(payload,&claims);err!=nil{return Claims{},err}
	if claims.SchemaVersion!=1||claims.Scope!=Scope||claims.LeaseID==""||claims.UserID==""||claims.DeviceID==""{
		return Claims{},fmt.Errorf("invalid proxy lease claims")
	}
	if !claims.ExpiresAt.After(now){return Claims{},fmt.Errorf("proxy lease expired")}
	if claims.IssuedAt.After(now.Add(2*time.Minute)){return Claims{},fmt.Errorf("proxy lease issued in future")}
	return claims,nil
}

func EncodeCredential(env Envelope)(string,error){
	raw,err:=json.Marshal(env);if err!=nil{return "",err}
	return base64.RawURLEncoding.EncodeToString(raw),nil
}

func DecodeCredential(raw string)(Envelope,error){
	data,err:=base64.RawURLEncoding.DecodeString(raw);if err!=nil{return Envelope{},fmt.Errorf("invalid credential encoding")}
	var env Envelope
	if err:=json.Unmarshal(data,&env);err!=nil{return Envelope{},fmt.Errorf("invalid credential")}
	return env,nil
}
