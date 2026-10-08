package main

import (
 "crypto/ed25519"
 "encoding/base64"
 "crypto/sha256"
 "fmt"
 "os"
)

func main(){
 for _,purpose:=range []string{"CONFIG","ACCESS","RELEASE"}{
  name:="VPNX3_"+purpose+"_SIGNING_KEY"
  encoded:=os.Getenv(name)
  seed,err:=base64.RawURLEncoding.DecodeString(encoded)
  if err!=nil||len(seed)!=ed25519.SeedSize{
   fmt.Fprintln(os.Stderr,"invalid or missing "+name)
   os.Exit(1)
  }
  pub:=ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
  fmt.Printf("VPNX3_%s_PUBLIC_KEY=%s\n",purpose,base64.RawURLEncoding.EncodeToString(pub))
  sum:=sha256.Sum256(pub)
  fmt.Printf("VPNX3_%s_KEY_ID=%x\n",purpose,sum[:8])
 }
}
