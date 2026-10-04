package artifactstorage

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCleanObjectKey(t *testing.T){
	valid:=[]string{"releases/1/app.apk","a/b/c","file.bin"}
	for _,v:=range valid{
		if _,err:=cleanObjectKey(v);err!=nil{t.Fatalf("%q unexpected error: %v",v,err)}
	}
	for _,v:=range []string{"","/","../x","a/../b","a//b","./x"}{
		if _,err:=cleanObjectKey(v);err==nil{t.Fatalf("%q expected error",v)}
	}
}

func TestS3ObjectURL(t *testing.T){
	s,err:=NewS3(S3Config{
		Endpoint:"https://storage.example.test/base",
		Region:"ru-1",
		Bucket:"vpnx3",
		AccessKey:"access",
		SecretKey:"secret",
		TempDir:t.TempDir(),
	})
	if err!=nil{t.Fatal(err)}
	u,err:=s.objectURL("releases/1/file name.apk")
	if err!=nil{t.Fatal(err)}
	if got,want:=u.String(),"https://storage.example.test/base/vpnx3/releases/1/file%20name.apk";got!=want{
		t.Fatalf("url=%q want %q",got,want)
	}
}

func TestS3SignDeterministic(t *testing.T){
	s,err:=NewS3(S3Config{
		Endpoint:"https://storage.example.test",
		Region:"test-1",
		Bucket:"vpnx3",
		AccessKey:"AKIDEXAMPLE",
		SecretKey:"secret-example",
		TempDir:t.TempDir(),
	})
	if err!=nil{t.Fatal(err)}
	u,_:=url.Parse("https://storage.example.test/vpnx3/releases/file.bin")
	req,_:=http.NewRequest(http.MethodGet,u.String(),nil)
	empty:=sha256.Sum256(nil)
	now:=time.Date(2026,10,4,7,0,0,0,time.UTC)
	if err:=s.sign(req,hex.EncodeToString(empty[:]),now);err!=nil{t.Fatal(err)}
	auth:=req.Header.Get("Authorization")
	if !strings.Contains(auth,"Credential=AKIDEXAMPLE/20261004/test-1/s3/aws4_request"){
		t.Fatalf("unexpected authorization scope: %s",auth)
	}
	if !strings.Contains(auth,"SignedHeaders=host;x-amz-content-sha256;x-amz-date"){
		t.Fatalf("unexpected signed headers: %s",auth)
	}
	first:=auth
	req2,_:=http.NewRequest(http.MethodGet,u.String(),nil)
	if err:=s.sign(req2,hex.EncodeToString(empty[:]),now);err!=nil{t.Fatal(err)}
	if req2.Header.Get("Authorization")!=first{
		t.Fatal("signature must be deterministic for identical request/time")
	}
}
