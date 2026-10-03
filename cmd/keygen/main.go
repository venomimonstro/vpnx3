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
	printKey("VPNX3_CONFIG_SIGNING_KEY")
	printKey("VPNX3_ACCESS_SIGNING_KEY")
}

func printKey(name string) {
	_,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil { panic(err) }
	seed:=priv.Seed()
	pub:=priv.Public().(ed25519.PublicKey)
	sum:=sha256.Sum256(pub)
	fmt.Println(name+"="+base64.RawURLEncoding.EncodeToString(seed))
	fmt.Println(name+"_PUBLIC="+base64.RawURLEncoding.EncodeToString(pub))
	fmt.Println(name+"_KEY_ID="+hex.EncodeToString(sum[:8]))
}
