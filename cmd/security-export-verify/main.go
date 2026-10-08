package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

type envelope struct {
	KeyID string `json:"key_id"`
	Payload string `json:"payload"`
	Signature string `json:"signature"`
}

type payload struct {
	AuditID int64 `json:"audit_id"`
	PrevHash string `json:"prev_hash"`
	EntryHash string `json:"entry_hash"`
}

func main(){
	publicRaw:=flag.String("public-key","","Ed25519 public key base64url without padding")
	flag.Parse()
	public,err:=base64.RawURLEncoding.DecodeString(*publicRaw)
	if err!=nil||len(public)!=ed25519.PublicKeySize{
		fmt.Fprintln(os.Stderr,"invalid Ed25519 public key")
		os.Exit(2)
	}
	body,err:=io.ReadAll(io.LimitReader(os.Stdin,1<<20))
	if err!=nil{panic(err)}
	var env envelope
	if err:=json.Unmarshal(body,&env);err!=nil{
		fmt.Fprintln(os.Stderr,"invalid envelope:",err);os.Exit(1)
	}
	raw,err:=base64.RawURLEncoding.DecodeString(env.Payload)
	if err!=nil{fmt.Fprintln(os.Stderr,"invalid payload encoding");os.Exit(1)}
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature)
	if err!=nil||len(sig)!=ed25519.SignatureSize{
		fmt.Fprintln(os.Stderr,"invalid signature encoding");os.Exit(1)
	}
	sum:=sha256.Sum256(public)
	expectedID:=hex.EncodeToString(sum[:8])
	if env.KeyID!=expectedID{
		fmt.Fprintln(os.Stderr,"key_id mismatch");os.Exit(1)
	}
	if !ed25519.Verify(ed25519.PublicKey(public),raw,sig){
		fmt.Fprintln(os.Stderr,"signature mismatch");os.Exit(1)
	}
	var p payload
	if err:=json.Unmarshal(raw,&p);err!=nil||p.AuditID<=0||len(p.EntryHash)!=64||(len(p.PrevHash)!=0&&len(p.PrevHash)!=64){
		fmt.Fprintln(os.Stderr,"invalid signed payload");os.Exit(1)
	}
	fmt.Printf("{\"status\":\"ok\",\"audit_id\":%d,\"prev_hash\":%q,\"entry_hash\":%q}\n",
		p.AuditID,p.PrevHash,p.EntryHash)
}
