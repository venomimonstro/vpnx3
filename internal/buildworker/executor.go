package buildworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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
		env,err:=androidBuildEnv(job.Version)
		if err!=nil{return Result{},err}
		if err:=run(buildCtx,filepath.Join(src,"apps","android"),env,"gradle","--no-daemon",":app:assembleRelease");err!=nil {
			return Result{},fmt.Errorf("android apk build: %w",err)
		}
		artifact=filepath.Join(src,"apps","android","app","build","outputs","apk","release","app-release.apk")
		if err:=run(buildCtx,src,nil,"bash","scripts/validate-android-release.sh","--apk",artifact);err!=nil{return Result{},fmt.Errorf("android apk validation: %w",err)}
	case "android_aab":
		env,err:=androidBuildEnv(job.Version)
		if err!=nil{return Result{},err}
		if err:=run(buildCtx,filepath.Join(src,"apps","android"),env,"gradle","--no-daemon",":app:bundleRelease");err!=nil {
			return Result{},fmt.Errorf("android aab build: %w",err)
		}
		artifact=filepath.Join(src,"apps","android","app","build","outputs","bundle","release","app-release.aab")
		if err:=run(buildCtx,src,nil,"bash","scripts/validate-android-release.sh","--aab",artifact);err!=nil{return Result{},fmt.Errorf("android aab validation: %w",err)}
	case "chrome_zip":
		dir:=filepath.Join(src,"apps","browser-extension","chrome")
		if _,err:=os.Stat(dir);err!=nil{return Result{},fmt.Errorf("chrome extension source unavailable")}
		if err:=writeBrowserRuntimeConfig(dir);err!=nil{return Result{},err}
		if err:=writeExtensionVersion(dir,job.Version);err!=nil{return Result{},err}
		if err:=run(buildCtx,src,nil,"python3","scripts/validate-browser-extension.py","--browser","chrome","--dir",dir);err!=nil{return Result{},fmt.Errorf("chrome extension validation: %w",err)}
		artifact=filepath.Join(work,"vpnx3-chrome.zip")
		if err:=run(buildCtx,dir,nil,"zip","-qr",artifact,".");err!=nil{return Result{},err}
		if err:=run(buildCtx,src,nil,"python3","scripts/validate-browser-package.py","--browser","chrome","--zip",artifact);err!=nil{return Result{},fmt.Errorf("chrome package validation: %w",err)}
	case "firefox_zip":
		dir:=filepath.Join(src,"apps","browser-extension","firefox")
		if _,err:=os.Stat(dir);err!=nil{return Result{},fmt.Errorf("firefox extension source unavailable")}
		if err:=writeBrowserRuntimeConfig(dir);err!=nil{return Result{},err}
		if err:=writeExtensionVersion(dir,job.Version);err!=nil{return Result{},err}
		if err:=run(buildCtx,src,nil,"python3","scripts/validate-browser-extension.py","--browser","firefox","--dir",dir);err!=nil{return Result{},fmt.Errorf("firefox extension validation: %w",err)}
		artifact=filepath.Join(work,"vpnx3-firefox.zip")
		if err:=run(buildCtx,dir,nil,"zip","-qr",artifact,".");err!=nil{return Result{},err}
		if err:=run(buildCtx,src,nil,"python3","scripts/validate-browser-package.py","--browser","firefox","--zip",artifact);err!=nil{return Result{},fmt.Errorf("firefox package validation: %w",err)}
	case "ios_ipa":
		if runtime.GOOS!="darwin" { return Result{},fmt.Errorf("ios_ipa requires a macOS build worker") }
		env,err:=iosBuildEnv(job.Version)
		if err!=nil{return Result{},err}
		if err:=run(buildCtx,src,env,"bash","scripts/build-ios.sh");err!=nil{
			return Result{},fmt.Errorf("ios ipa build: %w",err)
		}
		matches,err:=filepath.Glob(filepath.Join(src,"apps","ios",".export","*.ipa"))
		if err!=nil||len(matches)!=1{return Result{},fmt.Errorf("ios build did not produce exactly one IPA")}
		artifact=matches[0]
	case "controlplane_linux_amd64":
		artifact=filepath.Join(work,"vpnx3-controlplane")
		if err:=run(buildCtx,src,[]string{"CGO_ENABLED=0","GOOS=linux","GOARCH=amd64"},"go","build","-trimpath","-ldflags=-s -w","-o",artifact,"./cmd/controlplane");err!=nil{return Result{},err}
	case "node_agent_linux_amd64":
		artifact=filepath.Join(work,"vpnx3-node-agent")
		if err:=run(buildCtx,src,[]string{"CGO_ENABLED=0","GOOS=linux","GOARCH=amd64"},"go","build","-trimpath","-ldflags=-s -w","-o",artifact,"./cmd/node-agent");err!=nil{return Result{},err}
	case "vpn_worker_linux_amd64":
		artifact=filepath.Join(work,"vpnx3-vpn-worker")
		if err:=run(buildCtx,src,[]string{"CGO_ENABLED=0","GOOS=linux","GOARCH=amd64"},"go","build","-trimpath","-ldflags=-s -w","-o",artifact,"./cmd/vpn-worker");err!=nil{return Result{},err}
	case "probe_agent_linux_amd64":
		artifact=filepath.Join(work,"vpnx3-probe-agent")
		if err:=run(buildCtx,src,[]string{"CGO_ENABLED=0","GOOS=linux","GOARCH=amd64"},"go","build","-trimpath","-ldflags=-s -w","-o",artifact,"./cmd/probe-agent");err!=nil{return Result{},err}
	case "ingress_proxy_linux_amd64":
		artifact=filepath.Join(work,"vpnx3-ingress-proxy")
		if err:=run(buildCtx,src,[]string{"CGO_ENABLED=0","GOOS=linux","GOARCH=amd64"},"go","build","-trimpath","-ldflags=-s -w","-o",artifact,"./cmd/ingress-proxy");err!=nil{return Result{},err}
	case "build_worker_darwin_arm64":
		artifact=filepath.Join(work,"vpnx3-build-worker-darwin-arm64")
		if err:=run(buildCtx,src,[]string{"CGO_ENABLED=0","GOOS=darwin","GOARCH=arm64"},"go","build","-trimpath","-ldflags=-s -w","-o",artifact,"./cmd/build-worker");err!=nil{return Result{},err}
	case "config_mirror_linux_amd64":
		artifact=filepath.Join(work,"vpnx3-config-mirror")
		if err:=run(buildCtx,src,[]string{"CGO_ENABLED=0","GOOS=linux","GOARCH=amd64"},"go","build","-trimpath","-ldflags=-s -w","-o",artifact,"./cmd/config-mirror");err!=nil{return Result{},err}
	case "runtime_updater_linux_amd64":
		artifact=filepath.Join(work,"vpnx3-runtime-updater")
		if err:=run(buildCtx,src,[]string{"CGO_ENABLED=0","GOOS=linux","GOARCH=amd64"},"go","build","-trimpath","-ldflags=-s -w","-o",artifact,"./cmd/runtime-updater");err!=nil{return Result{},err}
	case "backup_replicator_linux_amd64":
		artifact=filepath.Join(work,"vpnx3-backup-replicator")
		if err:=run(buildCtx,src,[]string{"CGO_ENABLED=0","GOOS=linux","GOARCH=amd64"},"go","build","-trimpath","-ldflags=-s -w","-o",artifact,"./cmd/backup-replicator");err!=nil{return Result{},err}
	case "wal_replicator_linux_amd64":
		artifact=filepath.Join(work,"vpnx3-wal-replicator")
		if err:=run(buildCtx,src,[]string{"CGO_ENABLED=0","GOOS=linux","GOARCH=amd64"},"go","build","-trimpath","-ldflags=-s -w","-o",artifact,"./cmd/wal-replicator");err!=nil{return Result{},err}
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
func androidBuildEnv(releaseVersion string) ([]string,error) {
	versionName,versionCode,err:=androidVersion(releaseVersion)
	if err!=nil{return nil,err}
	keys:=[]string{
		"VPNX3_ANDROID_KEYSTORE_PATH","VPNX3_ANDROID_KEYSTORE_PASSWORD",
		"VPNX3_ANDROID_KEY_ALIAS","VPNX3_ANDROID_KEY_PASSWORD",
		"VPNX3_CLIENT_CONTROL_URL","VPNX3_CONFIG_PUBLIC_KEY","VPNX3_TRUST_ROOT_PUBLIC_KEY","VPNX3_RELEASE_PUBLIC_KEY",
		"VPNX3_CONFIG_BOOTSTRAP_URLS",
		"ANDROID_SDK_ROOT","ANDROID_HOME","GRADLE_USER_HOME","HOME",
	}
	out:=make([]string,0,len(keys)+2)
	for _,k:=range keys{if v:=os.Getenv(k);v!=""{out=append(out,k+"="+v)}}
	out=append(out,"VPNX3_RELEASE_VERSION="+versionName)
	out=append(out,"VPNX3_ANDROID_VERSION_CODE="+strconv.Itoa(versionCode))
	return out,nil
}

func androidVersion(raw string)(string,int,error){
	v:=strings.TrimSpace(strings.TrimPrefix(raw,"v"))
	if i:=strings.IndexByte(v,'-');i>=0{v=v[:i]}
	parts:=strings.Split(v,".")
	if len(parts)<1||len(parts)>3{return "",0,fmt.Errorf("Android release version must contain 1-3 numeric components")}
	nums:=[]int{0,0,0}
	for i,part:=range parts{
		if part==""{return "",0,fmt.Errorf("invalid Android release version")}
		for _,r:=range part{if r<'0'||r>'9'{return "",0,fmt.Errorf("Android release version must be numeric")}}
		n,err:=strconv.Atoi(part);if err!=nil{return "",0,fmt.Errorf("invalid Android release version")}
		nums[i]=n
	}
	if nums[0]<0||nums[0]>2100||nums[1]<0||nums[1]>999||nums[2]<0||nums[2]>999{
		return "",0,fmt.Errorf("Android release version is outside supported versionCode range")
	}
	code:=nums[0]*1_000_000+nums[1]*1_000+nums[2]
	if code<=0||code>2_100_000_000{return "",0,fmt.Errorf("Android versionCode is outside Play range")}
	return strings.Join(parts,"."),code,nil
}
func inspectArtifact(path string)(Result,error){
	f,err:=os.Open(path);if err!=nil{return Result{},fmt.Errorf("open artifact: %w",err)};defer f.Close()
	h:=sha256.New();n,err:=io.Copy(h,f);if err!=nil{return Result{},err}
	return Result{Path:path,FileName:filepath.Base(path),SHA256:hex.EncodeToString(h.Sum(nil)),SizeBytes:n},nil
}

func writeBrowserRuntimeConfig(dir string) error {
	control:=strings.TrimSpace(os.Getenv("VPNX3_CLIENT_CONTROL_URL"))
	key:=strings.TrimSpace(os.Getenv("VPNX3_CONFIG_PUBLIC_KEY"))
	releaseKey:=strings.TrimSpace(os.Getenv("VPNX3_RELEASE_PUBLIC_KEY"))
	rawMirrors:=strings.TrimSpace(os.Getenv("VPNX3_CONFIG_BOOTSTRAP_URLS"))
	trustRoot:=strings.TrimSpace(os.Getenv("VPNX3_TRUST_ROOT_PUBLIC_KEY"))
	if !strings.HasPrefix(control,"https://") || (trustRoot==""&&key=="") || (trustRoot==""&&releaseKey=="") || rawMirrors=="" {
		return fmt.Errorf("browser build requires control URL, trust root or legacy config key, release key and VPNX3_CONFIG_BOOTSTRAP_URLS")
	}
	mirrors:=make([]string,0)
	for _,part:=range strings.FieldsFunc(rawMirrors,func(r rune)bool{return r==','||r==';'}){
		v:=strings.TrimSpace(part)
		if v==""{continue}
		u,err:=url.Parse(v)
		if err!=nil||u.Scheme!="https"||u.Hostname()==""||u.User!=nil||u.Fragment!="" {
			return fmt.Errorf("invalid config bootstrap URL %q",v)
		}
		mirrors=append(mirrors,v)
	}
	if len(mirrors)==0{return fmt.Errorf("at least one config bootstrap URL is required")}
	mirrorsJSON,err:=json.Marshal(mirrors);if err!=nil{return err}
	content:=fmt.Sprintf(
		"const VPNX3_CONTROL_URL=%q;\nconst VPNX3_CONFIG_PUBLIC_KEY=%q;\nconst VPNX3_TRUST_ROOT_PUBLIC_KEY=%q;\nconst VPNX3_RELEASE_PUBLIC_KEY=%q;\nconst VPNX3_CONFIG_BOOTSTRAP_URLS=%s;\n",
		control,key,strings.TrimSpace(os.Getenv("VPNX3_TRUST_ROOT_PUBLIC_KEY")),releaseKey,string(mirrorsJSON),
	)
	return os.WriteFile(filepath.Join(dir,"runtime-config.js"),[]byte(content),0644)
}

func writeExtensionVersion(dir,releaseVersion string) error {
	version,err:=normalizeExtensionVersion(releaseVersion)
	if err!=nil{return err}
	path:=filepath.Join(dir,"manifest.json")
	raw,err:=os.ReadFile(path);if err!=nil{return err}
	var manifest map[string]any
	if err:=json.Unmarshal(raw,&manifest);err!=nil{return fmt.Errorf("parse extension manifest: %w",err)}
	manifest["version"]=version
	out,err:=json.MarshalIndent(manifest,"","  ");if err!=nil{return err}
	out=append(out,'\n')
	return os.WriteFile(path,out,0644)
}

func normalizeExtensionVersion(raw string)(string,error){
	v:=strings.TrimSpace(strings.TrimPrefix(raw,"v"))
	if i:=strings.IndexByte(v,'-');i>=0{v=v[:i]}
	parts:=strings.Split(v,".")
	if len(parts)<1||len(parts)>4{return "",fmt.Errorf("extension release version must contain 1-4 numeric components")}
	for _,part:=range parts{
		if part==""{return "",fmt.Errorf("invalid extension release version")}
		for _,r:=range part{if r<'0'||r>'9'{return "",fmt.Errorf("extension release version must be numeric")}}
		n,err:=strconv.Atoi(part);if err!=nil||n<0||n>65535{return "",fmt.Errorf("extension version component outside 0..65535")}
	}
	return strings.Join(parts,"."),nil
}


func iosBuildEnv(releaseVersion string)([]string,error){
	versionName,_,err:=androidVersion(releaseVersion)
	if err!=nil{return nil,fmt.Errorf("invalid iOS release version: %w",err)}
	keys:=[]string{
		"VPNX3_IOS_TEAM_ID","VPNX3_IOS_EXPORT_OPTIONS_PLIST",
		"VPNX3_CLIENT_CONTROL_URL","VPNX3_CONFIG_PUBLIC_KEY","VPNX3_TRUST_ROOT_PUBLIC_KEY","VPNX3_RELEASE_PUBLIC_KEY",
		"VPNX3_CONFIG_BOOTSTRAP_URLS","HOME",
	}
	out:=make([]string,0,len(keys)+1)
	for _,k:=range keys{
		v:=strings.TrimSpace(os.Getenv(k))
		required:=k=="VPNX3_IOS_TEAM_ID"||k=="VPNX3_IOS_EXPORT_OPTIONS_PLIST"||
			k=="VPNX3_CLIENT_CONTROL_URL"||k=="VPNX3_TRUST_ROOT_PUBLIC_KEY"||
			k=="VPNX3_CONFIG_BOOTSTRAP_URLS"||k=="HOME"
		if required&&v==""{return nil,fmt.Errorf("%s is required for iOS release builds",k)}
		if v!=""{out=append(out,k+"="+v)}
	}
	out=append(out,"VPNX3_RELEASE_VERSION="+versionName)
	return out,nil
}
