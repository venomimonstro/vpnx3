package revocations

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
)

const deviceHashDomain="vpnx3-revoked-device-v1\x00"

type Payload struct {
	SchemaVersion int `json:"schema_version"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	WindowHours int `json:"window_hours"`
	RevokedDeviceHashes []string `json:"revoked_device_hashes"`
}

type Envelope struct {
	Payload string `json:"payload"`
	Signature string `json:"signature"`
	KeyID string `json:"key_id"`
}

func DeviceHash(deviceID string) string {
	sum:=sha256.Sum256([]byte(deviceHashDomain+deviceID))
	return hex.EncodeToString(sum[:])
}

func Issue(signer *signing.Signer,p Payload)(Envelope,error){
	raw,err:=json.Marshal(p);if err!=nil{return Envelope{},err}
	return Envelope{
		Payload:base64.RawURLEncoding.EncodeToString(raw),
		Signature:base64.RawURLEncoding.EncodeToString(signer.Sign(raw)),
		KeyID:signer.KeyID(),
	},nil
}

type Verifier struct{
	public ed25519.PublicKey
	keyID string
}

func NewVerifier(publicKeyBase64 string)(*Verifier,error){
	raw,err:=base64.RawURLEncoding.DecodeString(publicKeyBase64)
	if err!=nil||len(raw)!=ed25519.PublicKeySize{return nil,fmt.Errorf("invalid revocation public key")}
	sum:=sha256.Sum256(raw)
	return &Verifier{public:ed25519.PublicKey(raw),keyID:hex.EncodeToString(sum[:8])},nil
}

func (v *Verifier) Verify(env Envelope,now time.Time)(Payload,error){
	if env.KeyID!=v.keyID{return Payload{},fmt.Errorf("unexpected revocation signing key")}
	raw,err:=base64.RawURLEncoding.DecodeString(env.Payload);if err!=nil{return Payload{},fmt.Errorf("invalid revocation payload")}
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature);if err!=nil{return Payload{},fmt.Errorf("invalid revocation signature encoding")}
	if !ed25519.Verify(v.public,raw,sig){return Payload{},fmt.Errorf("invalid revocation signature")}
	var p Payload
	if err:=json.Unmarshal(raw,&p);err!=nil{return Payload{},err}
	if p.SchemaVersion!=1||p.WindowHours<1||p.WindowHours>25{return Payload{},fmt.Errorf("invalid revocation payload schema")}
	if p.IssuedAt.After(now.Add(2*time.Minute)){return Payload{},fmt.Errorf("revocation feed issued in future")}
	if !p.ExpiresAt.After(now){return Payload{},fmt.Errorf("revocation feed expired")}
	if p.ExpiresAt.Sub(p.IssuedAt)>5*time.Minute{return Payload{},fmt.Errorf("revocation feed validity too long")}
	if len(p.RevokedDeviceHashes)>10000{return Payload{},fmt.Errorf("revocation feed too large")}
	for _,h:=range p.RevokedDeviceHashes{
		if len(h)!=64{ return Payload{},fmt.Errorf("invalid revoked device hash") }
		if _,err:=hex.DecodeString(h);err!=nil{return Payload{},fmt.Errorf("invalid revoked device hash")}
	}
	return p,nil
}
