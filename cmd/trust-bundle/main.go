package main

import (
	"encoding/base64"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
	"github.com/venomimonstro/vpnx3/internal/trustbundle"
)

func main(){
	var rootSeedFile,configPub,accessPub,releasePub,nextConfig,nextAccess,nextRelease,out string
	var version int64
	var validDays int
	flag.StringVar(&rootSeedFile,"root-seed-file","","path to offline root seed file (base64url 32-byte Ed25519 seed)")
	flag.StringVar(&configPub,"config-public-key","","active config public key")
	flag.StringVar(&accessPub,"access-public-key","","active access public key")
	flag.StringVar(&releasePub,"release-public-key","","active release public key")
	flag.StringVar(&nextConfig,"next-config-public-key","","optional next config public key")
	flag.StringVar(&nextAccess,"next-access-public-key","","optional next access public key")
	flag.StringVar(&nextRelease,"next-release-public-key","","optional next release public key")
	flag.StringVar(&out,"out","trust-bundle.json","output file")
	flag.Int64Var(&version,"version",0,"monotonic bundle version")
	flag.IntVar(&validDays,"valid-days",90,"bundle validity in days (1..366)")
	flag.Parse()

	if rootSeedFile==""||configPub==""||accessPub==""||version<=0||validDays<1||validDays>366{
		flag.Usage();os.Exit(2)
	}
	seedRaw,err:=os.ReadFile(rootSeedFile);must(err)
	root,err:=signing.FromSeedBase64(strings.TrimSpace(string(seedRaw)));must(err)
	now:=time.Now().UTC().Truncate(time.Second)
	keys:=[]trustbundle.Key{
		key("config",configPub,"active"),
		key("access",accessPub,"active"),
	}
	if releasePub!=""{keys=append(keys,key("release",releasePub,"active"))}
	if nextConfig!=""{keys=append(keys,key("config",nextConfig,"next"))}
	if nextAccess!=""{keys=append(keys,key("access",nextAccess,"next"))}
	if nextRelease!=""{keys=append(keys,key("release",nextRelease,"next"))}
	env,err:=trustbundle.Issue(root,trustbundle.Payload{
		SchemaVersion:1,Version:version,IssuedAt:now,
		ExpiresAt:now.Add(time.Duration(validDays)*24*time.Hour),Keys:keys,
	});must(err)
	raw,err:=json.MarshalIndent(env,"","  ");must(err)
	raw=append(raw,'\n')
	must(os.WriteFile(out,raw,0600))
	fmt.Println(out)
}

func key(purpose,pub,state string)trustbundle.Key{
	raw,err:=base64.RawURLEncoding.DecodeString(strings.TrimSpace(pub));must(err)
	if len(raw)!=32{panic("public key must contain 32 bytes")}
	sum:=sha256Sum(raw)
	return trustbundle.Key{
		Purpose:purpose,KeyID:sum,Algorithm:"ed25519",
		PublicKey:strings.TrimSpace(pub),State:state,
	}
}

func sha256Sum(raw []byte)string{
	h:=sha256.New();_,_=h.Write(raw)
	return fmt.Sprintf("%x",h.Sum(nil)[:8])
}

func must(err error){if err!=nil{panic(err)}}
