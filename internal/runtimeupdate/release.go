package runtimeupdate

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"
)

type ReleaseEnvelope struct {
	Payload string `json:"payload"`
	Signature string `json:"signature"`
	KeyID string `json:"key_id"`
}

type ReleaseInfo struct {
	SchemaVersion int `json:"schema_version"`
	ReleaseID string `json:"release_id"`
	Version string `json:"version"`
	ArtifactID string `json:"artifact_id"`
	Target string `json:"target"`
	FileName string `json:"file_name"`
	SHA256 string `json:"sha256"`
	SizeBytes int64 `json:"size_bytes"`
	DownloadPath string `json:"download_path"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func VerifyRelease(env ReleaseEnvelope,authorized map[string]string,target string,now time.Time)(ReleaseInfo,error){
	encoded,ok:=authorized[env.KeyID]
	if !ok{return ReleaseInfo{},fmt.Errorf("release signing key is not authorized")}
	pub,err:=base64.RawURLEncoding.DecodeString(encoded)
	if err!=nil||len(pub)!=ed25519.PublicKeySize{return ReleaseInfo{},fmt.Errorf("invalid release public key")}
	sum:=sha256.Sum256(pub)
	if hex.EncodeToString(sum[:8])!=env.KeyID{return ReleaseInfo{},fmt.Errorf("release key id mismatch")}
	payload,err:=base64.RawURLEncoding.DecodeString(env.Payload)
	if err!=nil{return ReleaseInfo{},fmt.Errorf("invalid release payload")}
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature)
	if err!=nil{return ReleaseInfo{},fmt.Errorf("invalid release signature")}
	if !ed25519.Verify(ed25519.PublicKey(pub),payload,sig){return ReleaseInfo{},fmt.Errorf("release signature verification failed")}
	var info ReleaseInfo
	if err:=json.Unmarshal(payload,&info);err!=nil{return ReleaseInfo{},err}
	if info.SchemaVersion!=1||info.ReleaseID==""||info.ArtifactID==""||info.Version==""||info.Target!=target{
		return ReleaseInfo{},fmt.Errorf("invalid release metadata")
	}
	if len(info.SHA256)!=64||info.SizeBytes<=0||info.SizeBytes>512<<20{return ReleaseInfo{},fmt.Errorf("invalid release artifact metadata")}
	for _,r:=range info.SHA256{
		if !((r>='0'&&r<='9')||(r>='a'&&r<='f')){return ReleaseInfo{},fmt.Errorf("invalid release sha256")}
	}
	if !strings.HasPrefix(info.DownloadPath,"/api/v1/releases/")||path.Clean(info.DownloadPath)!=info.DownloadPath{
		return ReleaseInfo{},fmt.Errorf("unsafe release download path")
	}
	if info.IssuedAt.After(now.Add(2*time.Minute))||!info.ExpiresAt.After(now)||info.ExpiresAt.Sub(info.IssuedAt)>20*time.Minute{
		return ReleaseInfo{},fmt.Errorf("release metadata outside validity window")
	}
	return info,nil
}
