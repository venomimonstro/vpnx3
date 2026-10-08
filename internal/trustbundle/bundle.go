package trustbundle

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
)

type Key struct {
	Purpose string `json:"purpose"`
	KeyID string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
	State string `json:"state"`
}

type Payload struct {
	SchemaVersion int `json:"schema_version"`
	Version int64 `json:"version"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Keys []Key `json:"keys"`
}

type Envelope struct {
	KeyID string `json:"key_id"`
	Payload string `json:"payload"`
	Signature string `json:"signature"`
}

type Verifier struct{
	public ed25519.PublicKey
	keyID string
}

func NewVerifier(publicBase64 string)(*Verifier,error){
	raw,err:=base64.RawURLEncoding.DecodeString(strings.TrimSpace(publicBase64))
	if err!=nil||len(raw)!=ed25519.PublicKeySize{return nil,fmt.Errorf("invalid trust root public key")}
	sum:=sha256.Sum256(raw)
	return &Verifier{public:ed25519.PublicKey(raw),keyID:hex.EncodeToString(sum[:8])},nil
}

func Issue(root *signing.Signer,p Payload)(Envelope,error){
	if root==nil{return Envelope{},fmt.Errorf("root signer is required")}
	if err:=validatePayload(p,time.Time{},false);err!=nil{return Envelope{},err}
	raw,err:=json.Marshal(p);if err!=nil{return Envelope{},err}
	return Envelope{
		KeyID:root.KeyID(),
		Payload:base64.RawURLEncoding.EncodeToString(raw),
		Signature:base64.RawURLEncoding.EncodeToString(root.Sign(raw)),
	},nil
}

func (v *Verifier) Verify(env Envelope,minimumVersion int64,now time.Time)(Payload,error){
	if env.KeyID!=v.keyID{return Payload{},fmt.Errorf("unexpected trust root key id")}
	raw,err:=base64.RawURLEncoding.DecodeString(env.Payload);if err!=nil{return Payload{},fmt.Errorf("invalid trust bundle payload encoding")}
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature);if err!=nil||len(sig)!=ed25519.SignatureSize{return Payload{},fmt.Errorf("invalid trust bundle signature encoding")}
	if !ed25519.Verify(v.public,raw,sig){return Payload{},fmt.Errorf("invalid trust bundle signature")}
	var p Payload
	if err:=json.Unmarshal(raw,&p);err!=nil{return Payload{},err}
	if p.Version<minimumVersion{return Payload{},fmt.Errorf("trust bundle rollback")}
	if err:=validatePayload(p,now,true);err!=nil{return Payload{},err}
	return p,nil
}

func LoadFile(path,rootPublic string,minimumVersion int64,now time.Time)(Envelope,Payload,error){
	raw,err:=os.ReadFile(path);if err!=nil{return Envelope{},Payload{},err}
	var env Envelope
	if err:=json.Unmarshal(raw,&env);err!=nil{return Envelope{},Payload{},fmt.Errorf("decode trust bundle: %w",err)}
	verifier,err:=NewVerifier(rootPublic);if err!=nil{return Envelope{},Payload{},err}
	p,err:=verifier.Verify(env,minimumVersion,now);if err!=nil{return Envelope{},Payload{},err}
	return env,p,nil
}

func ActiveKey(p Payload,purpose string)(Key,bool){
	for _,k:=range p.Keys{
		if k.Purpose==purpose&&k.State=="active"{return k,true}
	}
	return Key{},false
}

func MatchesSigner(p Payload,purpose string,signer *signing.Signer)error{
	if signer==nil{return fmt.Errorf("%s signer is required",purpose)}
	k,ok:=ActiveKey(p,purpose);if !ok{return fmt.Errorf("trust bundle has no active %s key",purpose)}
	if k.KeyID!=signer.KeyID()||k.PublicKey!=signer.PublicKeyBase64(){
		return fmt.Errorf("active %s signer does not match trust bundle",purpose)
	}
	return nil
}

func validatePayload(p Payload,now time.Time,enforceTime bool)error{
	if p.SchemaVersion!=1{return fmt.Errorf("unsupported trust bundle schema")}
	if p.Version<=0{return fmt.Errorf("trust bundle version must be positive")}
	if !p.ExpiresAt.After(p.IssuedAt){return fmt.Errorf("invalid trust bundle validity")}
	if p.ExpiresAt.Sub(p.IssuedAt)>366*24*time.Hour{return fmt.Errorf("trust bundle validity exceeds 366 days")}
	if enforceTime{
		if p.IssuedAt.After(now.Add(5*time.Minute)){return fmt.Errorf("trust bundle issued in future")}
		if !p.ExpiresAt.After(now){return fmt.Errorf("trust bundle expired")}
	}
	if len(p.Keys)<2||len(p.Keys)>12{return fmt.Errorf("trust bundle key count outside safe bounds")}
	active:=map[string]int{}
	seen:=map[string]struct{}{}
	for _,k:=range p.Keys{
		if k.Purpose!="config"&&k.Purpose!="access"&&k.Purpose!="release"{return fmt.Errorf("unsupported key purpose %q",k.Purpose)}
		if k.Algorithm!="ed25519"{return fmt.Errorf("unsupported key algorithm")}
		if k.State!="active"&&k.State!="next"&&k.State!="retired"{return fmt.Errorf("unsupported key state")}
		raw,err:=base64.RawURLEncoding.DecodeString(k.PublicKey);if err!=nil||len(raw)!=ed25519.PublicKeySize{return fmt.Errorf("invalid %s public key",k.Purpose)}
		sum:=sha256.Sum256(raw)
		expected:=hex.EncodeToString(sum[:8])
		if k.KeyID!=expected{return fmt.Errorf("key id mismatch for %s",k.Purpose)}
		id:=k.Purpose+"\x00"+k.KeyID
		if _,ok:=seen[id];ok{return fmt.Errorf("duplicate key %s",id)}
		seen[id]=struct{}{}
		if k.State=="active"{active[k.Purpose]++}
	}
	if active["config"]!=1||active["access"]!=1{return fmt.Errorf("exactly one active config and access key required")}
	if active["release"]>1{return fmt.Errorf("at most one active release key allowed")}
	sort.Slice(p.Keys,func(i,j int)bool{
		if p.Keys[i].Purpose!=p.Keys[j].Purpose{return p.Keys[i].Purpose<p.Keys[j].Purpose}
		return p.Keys[i].KeyID<p.Keys[j].KeyID
	})
	return nil
}
