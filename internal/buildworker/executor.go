package buildworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Executor struct {
	SourceRepo string
	WorkRoot string
	Timeout time.Duration
}

type Result struct {
	Path string
	FileName string
	SHA256 string
	SizeBytes int64
}

func (e Executor) Run(ctx context.Context,job Job) (Result,error) {
	if !validCommit(job.SourceCommit) { return Result{},fmt.Errorf("invalid source commit") }
	if strings.TrimSpace(e.SourceRepo)=="" { return Result{},fmt.Errorf("source repository is not configured") }
	if e.Timeout<=0 { e.Timeout=30*time.Minute }

	work,err:=os.MkdirTemp(e.WorkRoot,"vpnx3-build-*")
	if err!=nil{return Result{},err}
	defer os.RemoveAll(work)

	buildCtx,cancel:=context.WithTimeout(ctx,e.Timeout);defer cancel()
	if err:=run(buildCtx,work,nil,"git","clone","--no-checkout","--filter=blob:none",e.SourceRepo,"src");err!=nil {
		return Result{},fmt.Errorf("clone source: %w",err)
	}
	src:=filepath.Join(work,"src")
	if err:=run(buildCtx,src,nil,"git","checkout","--detach",job.SourceCommit);err!=nil {
		return Result{},fmt.Errorf("checkout source commit: %w",err)
	}
	actual,err:=output(buildCtx,src,"git","rev-parse","HEAD")
	if err!=nil || strings.TrimSpace(actual)!=job.SourceCommit {
		return Result{},fmt.Errorf("source commit verification failed")
	}

	var artifact string
	switch job.Target {
	case "android_apk":
		if err:=run(buildCtx,filepath.Join(src,"apps","android"),buildEnv(),"gradle","--no-daemon",":app:assembleRelease");err!=nil {
			return Result{},fmt.Errorf("android apk build: %w",err)
		}
		artifact=filepath.Join(src,"apps","android","app","build","outputs","apk","release","app-release.apk")
	case "android_aab":
		if err:=run(buildCtx,filepath.Join(src,"apps","android"),buildEnv(),"gradle","--no-daemon",":app:bundleRelease");err!=nil {
			return Result{},fmt.Errorf("android aab build: %w",err)
		}
		artifact=filepath.Join(src,"apps","android","app","build","outputs","bundle","release","app-release.aab")
	case "chrome_zip":
		dir:=filepath.Join(src,"apps","browser-extension","chrome")
		if _,err:=os.Stat(dir);err!=nil{return Result{},fmt.Errorf("chrome extension source unavailable")}
		artifact=filepath.Join(work,"vpnx3-chrome.zip")
		if err:=run(buildCtx,dir,nil,"zip","-qr",artifact,".");err!=nil{return Result{},err}
	case "firefox_zip":
		dir:=filepath.Join(src,"apps","browser-extension","firefox")
		if _,err:=os.Stat(dir);err!=nil{return Result{},fmt.Errorf("firefox extension source unavailable")}
		artifact=filepath.Join(work,"vpnx3-firefox.zip")
		if err:=run(buildCtx,dir,nil,"zip","-qr",artifact,".");err!=nil{return Result{},err}
	case "ios_ipa":
		if runtime.GOOS!="darwin" { return Result{},fmt.Errorf("ios_ipa requires a macOS build worker") }
		return Result{},fmt.Errorf("ios recipe is not enabled until the Xcode project is present")
	default:
		return Result{},fmt.Errorf("unsupported build target")
	}

	return inspectArtifact(artifact)
}

func validCommit(v string) bool {
	if len(v)!=40{return false}
	for _,r:=range v{if !((r>='0'&&r<='9')||(r>='a'&&r<='f')){return false}}
	return true
}

func run(ctx context.Context,dir string,extraEnv []string,name string,args ...string) error {
	cmd:=exec.CommandContext(ctx,name,args...)
	cmd.Dir=dir
	cmd.Env=append(os.Environ(),extraEnv...)
	out,err:=cmd.CombinedOutput()
	if err!=nil {
		text:=strings.TrimSpace(string(out));if len(text)>4000{text=text[len(text)-4000:]}
		return fmt.Errorf("%s: %w: %s",name,err,text)
	}
	return nil
}
func output(ctx context.Context,dir,name string,args ...string)(string,error){
	cmd:=exec.CommandContext(ctx,name,args...);cmd.Dir=dir;b,err:=cmd.Output();return string(b),err
}
func buildEnv() []string {
	keys:=[]string{
		"VPNX3_ANDROID_KEYSTORE_PATH","VPNX3_ANDROID_KEYSTORE_PASSWORD",
		"VPNX3_ANDROID_KEY_ALIAS","VPNX3_ANDROID_KEY_PASSWORD",
		"VPNX3_CLIENT_CONTROL_URL","VPNX3_CONFIG_PUBLIC_KEY","VPNX3_RELEASE_PUBLIC_KEY",
	}
	out:=make([]string,0,len(keys))
	for _,k:=range keys{if v:=os.Getenv(k);v!=""{out=append(out,k+"="+v)}}
	return out
}
func inspectArtifact(path string)(Result,error){
	f,err:=os.Open(path);if err!=nil{return Result{},fmt.Errorf("open artifact: %w",err)};defer f.Close()
	h:=sha256.New();n,err:=io.Copy(h,f);if err!=nil{return Result{},err}
	return Result{Path:path,FileName:filepath.Base(path),SHA256:hex.EncodeToString(h.Sum(nil)),SizeBytes:n},nil
}
