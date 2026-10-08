package revocation

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
)

const hashDomain="vpnx3-revoked-device-v1\x00"

type Payload struct {
	SchemaVersion int `json:"schema_version"`
	Version int64 `json:"version"`
	GeneratedAt time.Time `json:"generated_at"`
	ExpiresAt time.Time `json:"expires_at"`
	RevokedDeviceHashes []string `json:"revoked_device_hashes"`
}

type Envelope struct {
	KeyID string `json:"key_id"`
	Payload string `json:"payload"`
	Signature string `json:"signature"`
}

func DeviceHash(deviceID string) string {
	sum:=sha256.Sum256([]byte(hashDomain+deviceID))
	return hex.EncodeToString(sum[:])
}

func Issue(signer *signing.Signer,version int64,deviceIDs []string,now time.Time,ttl time.Duration)(Envelope,error){
	if signer==nil{return Envelope{},fmt.Errorf("revocation signer is required")}
	if ttl<time.Minute||ttl>time.Hour{return Envelope{},fmt.Errorf("invalid revocation ttl")}
	hashes:=make([]string,0,len(deviceIDs))
	seen:=map[string]struct{}{}
	for _,id:=range deviceIDs{
		if id==""{continue}
		h:=DeviceHash(id)
		if _,ok:=seen[h];ok{continue}
		seen[h]=struct{}{}
		hashes=append(hashes,h)
	}
	sort.Strings(hashes)
	payload:=Payload{
		SchemaVersion:1,Version:version,GeneratedAt:now.UTC(),
		ExpiresAt:now.UTC().Add(ttl),RevokedDeviceHashes:hashes,
	}
	raw,err:=json.Marshal(payload)
	if err!=nil{return Envelope{},err}
	return Envelope{
		KeyID:signer.KeyID(),
		Payload:base64.RawURLEncoding.EncodeToString(raw),
		Signature:base64.RawURLEncoding.EncodeToString(signer.Sign(raw)),
	},nil
}

type Verifier struct {
	public ed25519.PublicKey
	keyID string
}

func NewVerifier(publicBase64 string)(*Verifier,error){
	raw,err:=base64.RawURLEncoding.DecodeString(publicBase64)
	if err!=nil||len(raw)!=ed25519.PublicKeySize{return nil,fmt.Errorf("invalid access public key")}
	sum:=sha256.Sum256(raw)
	return &Verifier{public:ed25519.PublicKey(raw),keyID:hex.EncodeToString(sum[:8])},nil
}

func (v *Verifier) Verify(env Envelope,minimumVersion int64,now time.Time)(Payload,error){
	return v.verify(env,minimumVersion,now,true)
}

func (v *Verifier) VerifyStored(env Envelope,minimumVersion int64)(Payload,error){
	return v.verify(env,minimumVersion,time.Time{},false)
}

func (v *Verifier) verify(env Envelope,minimumVersion int64,now time.Time,enforceTime bool)(Payload,error){
	if env.KeyID!=v.keyID{return Payload{},fmt.Errorf("unexpected revocation signing key")}
	raw,err:=base64.RawURLEncoding.DecodeString(env.Payload)
	if err!=nil{return Payload{},fmt.Errorf("invalid revocation payload encoding")}
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature)
	if err!=nil||len(sig)!=ed25519.SignatureSize{return Payload{},fmt.Errorf("invalid revocation signature encoding")}
	if !ed25519.Verify(v.public,raw,sig){return Payload{},fmt.Errorf("invalid revocation signature")}
	var p Payload
	if err:=json.Unmarshal(raw,&p);err!=nil{return Payload{},err}
	if p.SchemaVersion!=1||p.Version<0||p.Version<minimumVersion{
		return Payload{},fmt.Errorf("revocation snapshot rollback")
	}
	if p.ExpiresAt.Sub(p.GeneratedAt)<=0||p.ExpiresAt.Sub(p.GeneratedAt)>time.Hour{
		return Payload{},fmt.Errorf("invalid revocation snapshot validity")
	}
	if enforceTime{
		if p.GeneratedAt.After(now.Add(2*time.Minute)){return Payload{},fmt.Errorf("revocation snapshot issued in future")}
		if !p.ExpiresAt.After(now){return Payload{},fmt.Errorf("revocation snapshot expired")}
	}
	if len(p.RevokedDeviceHashes)>100000{return Payload{},fmt.Errorf("revocation snapshot too large")}
	for _,h:=range p.RevokedDeviceHashes{
		if len(h)!=64{return Payload{},fmt.Errorf("invalid revoked device hash")}
		if _,err:=hex.DecodeString(h);err!=nil{return Payload{},fmt.Errorf("invalid revoked device hash")}
	}
	return p,nil
}
