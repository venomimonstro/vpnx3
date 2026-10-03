package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

func main() {
	pub,_,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil { panic(err) }
	_,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil { panic(err) }
	seed:=priv.Seed()
	sum:=sha256.Sum256(priv.Public().(ed25519.PublicKey))
	fmt.Println("VPNX3_CONFIG_SIGNING_KEY="+base64.RawURLEncoding.EncodeToString(seed))
	fmt.Println("public_key="+base64.RawURLEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
	fmt.Println("key_id="+hex.EncodeToString(sum[:8]))
	_ = pub
}
