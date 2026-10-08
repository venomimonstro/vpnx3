package runtimeupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
)

func updateSigner(t *testing.T)*signing.Signer{
	t.Helper()
	_,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
	s,err:=signing.FromSeedBase64(base64.RawURLEncoding.EncodeToString(priv.Seed()))
	if err!=nil{t.Fatal(err)}
	return s
}

func signedRelease(t *testing.T,s *signing.Signer,target,version string,now time.Time)ReleaseEnvelope{
	t.Helper()
	info:=ReleaseInfo{
		SchemaVersion:1,ReleaseID:"release",Version:version,ArtifactID:"artifact",
		Target:target,FileName:"vpnx3-worker",SHA256:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SizeBytes:123,DownloadPath:"/api/v1/releases/release/artifacts/artifact/download",
		IssuedAt:now,ExpiresAt:now.Add(10*time.Minute),
	}
	raw,err:=json.Marshal(info);if err!=nil{t.Fatal(err)}
	return ReleaseEnvelope{
		Payload:base64.RawURLEncoding.EncodeToString(raw),
		Signature:base64.RawURLEncoding.EncodeToString(s.Sign(raw)),
		KeyID:s.KeyID(),
	}
}

func TestVerifyReleaseUsesAuthorizedKeyAndTarget(t *testing.T){
	s:=updateSigner(t);now:=time.Now().UTC().Truncate(time.Second)
	env:=signedRelease(t,s,"vpn_worker_linux_amd64","1.2.3",now)
	info,err:=VerifyRelease(env,map[string]string{s.KeyID():s.PublicKeyBase64()},"vpn_worker_linux_amd64",now)
	if err!=nil{t.Fatal(err)}
	if info.Version!="1.2.3"{t.Fatalf("unexpected version %s",info.Version)}

	other:=updateSigner(t)
	if _,err:=VerifyRelease(env,map[string]string{other.KeyID():other.PublicKeyBase64()},"vpn_worker_linux_amd64",now);err==nil{
		t.Fatal("unknown release key must be rejected")
	}
	if _,err:=VerifyRelease(env,map[string]string{s.KeyID():s.PublicKeyBase64()},"node_agent_linux_amd64",now);err==nil{
		t.Fatal("wrong target must be rejected")
	}
}

func TestCompareVersionsPreventsDowngrade(t *testing.T){
	cases:=[]struct{a,b string;want int}{
		{"1.2.3","1.2.2",1},
		{"1.2.3","1.2.3",0},
		{"1.2.2","1.2.3",-1},
		{"2.0","1.99.99",1},
		{"v1.2.3-rc1","1.2.3",0},
	}
	for _,tc:=range cases{
		got,err:=CompareVersions(tc.a,tc.b)
		if err!=nil{t.Fatalf("%s/%s: %v",tc.a,tc.b,err)}
		if got!=tc.want{t.Fatalf("%s/%s got %d want %d",tc.a,tc.b,got,tc.want)}
	}
}
