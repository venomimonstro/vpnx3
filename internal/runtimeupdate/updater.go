package runtimeupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/trustbundle"
)

type Target struct {
	Name string
	BinaryPath string
	Service string
}

var targets=map[string]Target{
	"node_agent_linux_amd64":{Name:"node_agent_linux_amd64",BinaryPath:"/usr/local/bin/vpnx3-node-agent",Service:"vpnx3-node-agent.service"},
	"vpn_worker_linux_amd64":{Name:"vpn_worker_linux_amd64",BinaryPath:"/usr/local/bin/vpnx3-worker",Service:"vpnx3-worker.service"},
	"probe_agent_linux_amd64":{Name:"probe_agent_linux_amd64",BinaryPath:"/usr/local/bin/vpnx3-probe-agent",Service:"vpnx3-probe-agent.service"},
	"ingress_proxy_linux_amd64":{Name:"ingress_proxy_linux_amd64",BinaryPath:"/usr/local/bin/vpnx3-ingress-proxy",Service:"vpnx3-ingress-proxy.service"},
	"config_mirror_linux_amd64":{Name:"config_mirror_linux_amd64",BinaryPath:"/usr/local/bin/vpnx3-config-mirror",Service:"vpnx3-config-mirror.service"},
}

type Config struct {
	ControlURL string
	TrustRootPublicKey string
	ReleaseSources []string
	TargetName string
	StatePath string
	HealthURL string
}

type state struct {
	Version string `json:"version"`
	SHA256 string `json:"sha256"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Updater struct {
	cfg Config
	target Target
	http *http.Client
}

func New(cfg Config)(*Updater,error){
	cfg.ControlURL=strings.TrimRight(strings.TrimSpace(cfg.ControlURL),"/")
	cfg.TrustRootPublicKey=strings.TrimSpace(cfg.TrustRootPublicKey)
	cfg.TargetName=strings.TrimSpace(cfg.TargetName)
	if cfg.ControlURL==""||cfg.TrustRootPublicKey==""||cfg.TargetName==""{
		return nil,fmt.Errorf("control url, trust root and target are required")
	}
	u,err:=url.Parse(cfg.ControlURL)
	if err!=nil||u.Scheme!="https"||u.Hostname()==""||u.User!=nil||u.Fragment!=""{
		return nil,fmt.Errorf("control URL must be credential-free https")
	}
	target,ok:=targets[cfg.TargetName]
	if !ok{return nil,fmt.Errorf("unsupported runtime update target")}
	if cfg.StatePath==""{cfg.StatePath="/var/lib/vpnx3-updater/"+cfg.TargetName+".json"}
	if cfg.HealthURL!=""{
		h,err:=url.Parse(cfg.HealthURL)
		if err!=nil||!(h.Scheme=="http"||h.Scheme=="https")||h.Hostname()==""||h.User!=nil{
			return nil,fmt.Errorf("invalid health URL")
		}
		host:=h.Hostname()
		if host!="127.0.0.1"&&host!="localhost"&&host!="::1"{
			return nil,fmt.Errorf("health URL must be loopback-only")
		}
	}
	return &Updater{
		cfg:cfg,target:target,
		http:&http.Client{
			Timeout:20*time.Second,
			CheckRedirect:func(_ *http.Request,_ []*http.Request)error{return http.ErrUseLastResponse},
		},
	},nil
}

func (u *Updater) Run(ctx context.Context)(bool,error){
	sources:=append([]string{u.cfg.ControlURL},u.cfg.ReleaseSources...)
	_,trustPayload,err:=trustbundle.Bootstrap(
		ctx,sources,u.cfg.TrustRootPublicKey,u.cfg.StatePath+".trust",time.Now().UTC(),
	)
	if err!=nil{return false,fmt.Errorf("trust bootstrap: %w",err)}
	releaseKeys:=trustbundle.VerificationKeys(trustPayload,"release")
	if len(releaseKeys)==0{return false,fmt.Errorf("no authorized release key")}

	env,base,err:=u.latestRelease(ctx)
	if err!=nil{return false,err}
	info,err:=VerifyRelease(env,releaseKeys,u.target.Name,time.Now().UTC())
	if err!=nil{return false,err}

	current,_:=u.loadState()
	if current.Version!=""{
		cmp,err:=CompareVersions(info.Version,current.Version)
		if err!=nil{return false,err}
		if cmp<0{return false,fmt.Errorf("release downgrade rejected: %s -> %s",current.Version,info.Version)}
		if cmp==0{
			if current.SHA256!=""&&current.SHA256!=info.SHA256{
				return false,fmt.Errorf("same version has different artifact hash")
			}
			return false,nil
		}
	}

	tmp,err:=u.download(ctx,base,info)
	if err!=nil{return false,err}
	defer os.Remove(tmp)
	if err:=u.installAndRestart(ctx,tmp);err!=nil{return false,err}
	if err:=u.saveState(state{Version:info.Version,SHA256:info.SHA256,UpdatedAt:time.Now().UTC()});err!=nil{
		return true,err
	}
	return true,nil
}

func (u *Updater) latestRelease(ctx context.Context)(ReleaseEnvelope,string,error){
	sources:=append([]string{u.cfg.ControlURL},u.cfg.ReleaseSources...)
	var last error
	seen:=map[string]struct{}{}
	for _,base:=range sources{
		base=strings.TrimRight(strings.TrimSpace(base),"/")
		if base==""{continue}
		if _,ok:=seen[base];ok{continue}
		seen[base]=struct{}{}
		parsed,err:=url.Parse(base)
		if err!=nil||parsed.Scheme!="https"||parsed.Hostname()==""||parsed.User!=nil||parsed.Fragment!=""{
			last=fmt.Errorf("unsafe release source")
			continue
		}
		endpoint:=base+"/api/v1/releases/latest?target="+url.QueryEscape(u.target.Name)
		var env ReleaseEnvelope
		if err:=u.getJSON(ctx,endpoint,&env);err!=nil{last=err;continue}
		return env,base,nil
	}
	if last==nil{last=fmt.Errorf("release sources unavailable")}
	return ReleaseEnvelope{},"",last
}

func (u *Updater) download(ctx context.Context,base string,info ReleaseInfo)(string,error){
	full:=strings.TrimRight(base,"/")+info.DownloadPath
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,full,nil)
	if err!=nil{return "",err}
	req.Header.Set("Accept","application/octet-stream")
	resp,err:=u.http.Do(req)
	if err!=nil{return "",err}
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusOK{return "",fmt.Errorf("artifact download HTTP %d",resp.StatusCode)}
	if resp.ContentLength>0&&resp.ContentLength!=info.SizeBytes{return "",fmt.Errorf("artifact size header mismatch")}
	if err:=os.MkdirAll(filepath.Dir(u.cfg.StatePath),0700);err!=nil{return "",err}
	f,err:=os.CreateTemp(filepath.Dir(u.cfg.StatePath),"artifact-*")
	if err!=nil{return "",err}
	name:=f.Name()
	h:=sha256.New()
	n,copyErr:=io.Copy(io.MultiWriter(f,h),io.LimitReader(resp.Body,info.SizeBytes+1))
	syncErr:=f.Sync()
	closeErr:=f.Close()
	if copyErr!=nil||syncErr!=nil||closeErr!=nil{
		os.Remove(name)
		return "",fmt.Errorf("artifact write failed")
	}
	if n!=info.SizeBytes{os.Remove(name);return "",fmt.Errorf("artifact size mismatch")}
	if hex.EncodeToString(h.Sum(nil))!=info.SHA256{
		os.Remove(name)
		return "",fmt.Errorf("artifact sha256 mismatch")
	}
	if err:=os.Chmod(name,0755);err!=nil{os.Remove(name);return "",err}
	return name,nil
}

func (u *Updater) installAndRestart(ctx context.Context,tmp string)error{
	dir:=filepath.Dir(u.target.BinaryPath)
	if err:=os.MkdirAll(dir,0755);err!=nil{return err}
	staged:=u.target.BinaryPath+".new"
	backup:=u.target.BinaryPath+".previous"
	if err:=copyFile(tmp,staged,0755);err!=nil{return err}

	hadOld:=false
	if _,err:=os.Stat(u.target.BinaryPath);err==nil{
		hadOld=true
		_ = os.Remove(backup)
		if err:=os.Rename(u.target.BinaryPath,backup);err!=nil{return err}
	}else if !errors.Is(err,os.ErrNotExist){
		return err
	}
	if err:=os.Rename(staged,u.target.BinaryPath);err!=nil{
		if hadOld{_ = os.Rename(backup,u.target.BinaryPath)}
		return err
	}
	if err:=syncDir(dir);err!=nil{return u.rollback(ctx,hadOld,backup,err)}
	if err:=systemctl(ctx,"restart",u.target.Service);err!=nil{return u.rollback(ctx,hadOld,backup,err)}
	if err:=u.waitHealthy(ctx);err!=nil{return u.rollback(ctx,hadOld,backup,err)}
	_ = os.Remove(backup)
	return nil
}

func (u *Updater) rollback(ctx context.Context,hadOld bool,backup string,cause error)error{
	if !hadOld{return fmt.Errorf("new service failed and no previous binary exists: %w",cause)}
	_ = systemctl(ctx,"stop",u.target.Service)
	_ = os.Remove(u.target.BinaryPath)
	if err:=os.Rename(backup,u.target.BinaryPath);err!=nil{
		return fmt.Errorf("rollback restore failed after %v: %w",cause,err)
	}
	_ = systemctl(ctx,"restart",u.target.Service)
	return fmt.Errorf("update rolled back: %w",cause)
}

func (u *Updater) waitHealthy(ctx context.Context)error{
	deadline:=time.Now().Add(45*time.Second)
	for time.Now().Before(deadline){
		checkCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
		activeErr:=systemctl(checkCtx,"is-active","--quiet",u.target.Service)
		cancel()
		if activeErr==nil{
			if u.cfg.HealthURL==""{return nil}
			req,_:=http.NewRequestWithContext(ctx,http.MethodGet,u.cfg.HealthURL,nil)
			resp,err:=u.http.Do(req)
			if err==nil{
				_,_ = io.Copy(io.Discard,io.LimitReader(resp.Body,4096))
				resp.Body.Close()
				if resp.StatusCode>=200&&resp.StatusCode<300{return nil}
			}
		}
		select{
		case <-ctx.Done():return ctx.Err()
		case <-time.After(2*time.Second):
		}
	}
	return fmt.Errorf("updated service failed health check")
}

func (u *Updater) getJSON(ctx context.Context,endpoint string,out any)error{
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,endpoint,nil)
	if err!=nil{return err}
	req.Header.Set("Accept","application/json")
	resp,err:=u.http.Do(req)
	if err!=nil{return err}
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusOK{return fmt.Errorf("HTTP %d",resp.StatusCode)}
	return json.NewDecoder(io.LimitReader(resp.Body,1<<20)).Decode(out)
}

func (u *Updater) loadState()(state,error){
	raw,err:=os.ReadFile(u.cfg.StatePath)
	if errors.Is(err,os.ErrNotExist){return state{},nil}
	if err!=nil{return state{},err}
	var s state
	if err:=json.Unmarshal(raw,&s);err!=nil{return state{},err}
	return s,nil
}

func (u *Updater) saveState(s state)error{
	raw,err:=json.Marshal(s)
	if err!=nil{return err}
	if err:=os.MkdirAll(filepath.Dir(u.cfg.StatePath),0700);err!=nil{return err}
	tmp:=u.cfg.StatePath+".tmp"
	if err:=os.WriteFile(tmp,raw,0600);err!=nil{return err}
	f,err:=os.Open(tmp)
	if err==nil{_ = f.Sync();_ = f.Close()}
	return os.Rename(tmp,u.cfg.StatePath)
}

func copyFile(src,dst string,mode os.FileMode)error{
	in,err:=os.Open(src)
	if err!=nil{return err}
	defer in.Close()
	out,err:=os.OpenFile(dst,os.O_CREATE|os.O_TRUNC|os.O_WRONLY,mode)
	if err!=nil{return err}
	_,copyErr:=io.Copy(out,in)
	syncErr:=out.Sync()
	closeErr:=out.Close()
	if copyErr!=nil{return copyErr}
	if syncErr!=nil{return syncErr}
	return closeErr
}

func syncDir(dir string)error{
	f,err:=os.Open(dir)
	if err!=nil{return err}
	defer f.Close()
	return f.Sync()
}

func systemctl(ctx context.Context,args ...string)error{
	out,err:=exec.CommandContext(ctx,"systemctl",args...).CombinedOutput()
	if err!=nil{
		return fmt.Errorf("systemctl %s: %w: %s",strings.Join(args," "),err,strings.TrimSpace(string(out)))
	}
	return nil
}

func CompareVersions(a,b string)(int,error){
	parse:=func(raw string)([]int,error){
		raw=strings.TrimSpace(strings.TrimPrefix(raw,"v"))
		if i:=strings.IndexByte(raw,'-');i>=0{raw=raw[:i]}
		parts:=strings.Split(raw,".")
		if len(parts)<1||len(parts)>4{return nil,fmt.Errorf("invalid version")}
		out:=make([]int,len(parts))
		for i,p:=range parts{
			n,err:=strconv.Atoi(p)
			if err!=nil||n<0||n>1_000_000{return nil,fmt.Errorf("invalid version")}
			out[i]=n
		}
		return out,nil
	}
	aa,err:=parse(a)
	if err!=nil{return 0,err}
	bb,err:=parse(b)
	if err!=nil{return 0,err}
	n:=len(aa)
	if len(bb)>n{n=len(bb)}
	for i:=0;i<n;i++{
		av,bv:=0,0
		if i<len(aa){av=aa[i]}
		if i<len(bb){bv=bb[i]}
		if av<bv{return -1,nil}
		if av>bv{return 1,nil}
	}
	return 0,nil
}
