package probe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accesslease"
	"github.com/venomimonstro/vpnx3/internal/clientconfig"
	"github.com/venomimonstro/vpnx3/internal/store"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type syntheticManifest struct {
	Workers []manifestNode `json:"workers"`
	Network struct {
		GatewayIPv4 string `json:"gateway_ipv4"`
		MTU int `json:"mtu"`
	} `json:"network"`
}

type syntheticWorkerSession struct {
	ID string `json:"id"`
	Config struct {
		AssignedIP string `json:"assigned_ip"`
		ServerPublicKey string `json:"server_public_key"`
		Endpoint string `json:"endpoint"`
		ExpiresAt time.Time `json:"expires_at"`
	} `json:"config"`
}

func ObserveWireGuard(ctx context.Context,manifest clientconfig.Envelope,c *Client,interfaceName string,maxWorkers int)([]store.ProbeObservation,error){
	if c==nil{return nil,fmt.Errorf("probe client is required")}
	if strings.TrimSpace(interfaceName)==""{interfaceName="wgprobe0"}
	if len(interfaceName)>15{return nil,fmt.Errorf("probe interface name is too long")}
	if maxWorkers<=0{maxWorkers=5}

	var payload syntheticManifest
	if err:=clientconfig.DecodePayload(manifest,&payload);err!=nil{return nil,err}
	gateway:=net.ParseIP(strings.TrimSpace(payload.Network.GatewayIPv4))
	if gateway==nil||gateway.To4()==nil{return nil,fmt.Errorf("signed manifest has invalid gateway_ipv4")}
	if len(payload.Workers)==0{return []store.ProbeObservation{},nil}

	selected:=rotatingWorkers(payload.Workers,c.identity.NodeID,maxWorkers,time.Now().UTC())
	out:=make([]store.ProbeObservation,0,len(selected))
	for _,worker:=range selected{
		obs:=observeWireGuardWorker(ctx,c,worker,interfaceName,gateway.String(),payload.Network.MTU)
		out=append(out,obs)
	}
	return out,nil
}

func rotatingWorkers(workers []manifestNode,probeNodeID string,max int,now time.Time)[]manifestNode{
	if len(workers)<=max{return workers}
	sum:=sha256.Sum256([]byte(probeNodeID+"|"+strconv.FormatInt(now.Unix()/60,10)))
	start:=int(binary.BigEndian.Uint32(sum[:4])%uint32(len(workers)))
	out:=make([]manifestNode,0,max)
	for i:=0;i<max;i++{out=append(out,workers[(start+i)%len(workers)])}
	return out
}

func observeWireGuardWorker(parent context.Context,c *Client,worker manifestNode,iface,gateway string,mtu int) store.ProbeObservation {
	start:=time.Now()
	obs:=store.ProbeObservation{TargetNodeID:worker.ID,EndpointKind:"wireguard_data_plane"}
	ctx,cancel:=context.WithTimeout(parent,15*time.Second)
	defer cancel()

	sessionURL:=""
	signedWG:=""
	for _,ep:=range worker.Endpoints{
		switch {
		case ep.Kind=="session_api"&&ep.Scheme=="https":
			if sessionURL==""{sessionURL=fmt.Sprintf("https://%s:%d%s",ep.Host,ep.Port,ep.Path)}
		case ep.Kind=="wireguard"&&ep.Scheme=="udp":
			if signedWG==""{signedWG=joinHostPort(ep.Host,ep.Port)}
		}
	}
	if sessionURL==""||signedWG==""{
		obs.LatencyMS=int(time.Since(start).Milliseconds())
		return obs
	}

	privateKey,err:=wgtypes.GeneratePrivateKey()
	if err!=nil{return finishSynthetic(obs,start,false)}
	publicKey:=privateKey.PublicKey().String()
	lease,err:=c.SyntheticLease(ctx,publicKey)
	if err!=nil{return finishSynthetic(obs,start,false)}

	session,err:=createSyntheticSession(ctx,sessionURL,lease,publicKey)
	if err!=nil{return finishSynthetic(obs,start,false)}
	defer closeSyntheticSession(context.Background(),sessionURL,session.ID)

	if session.Config.Endpoint!=signedWG{
		return finishSynthetic(obs,start,false)
	}

	_ = exec.Command("ip","link","del",iface).Run()
	defer exec.Command("ip","link","del",iface).Run()

	if err:=runCommand(ctx,"ip","link","add","dev",iface,"type","wireguard");err!=nil{
		return finishSynthetic(obs,start,false)
	}

	serverKey,err:=wgtypes.ParseKey(session.Config.ServerPublicKey)
	if err!=nil{return finishSynthetic(obs,start,false)}
	endpoint,err:=net.ResolveUDPAddr("udp",session.Config.Endpoint)
	if err!=nil{return finishSynthetic(obs,start,false)}
	_,allowed,err:=net.ParseCIDR(gateway+"/32")
	if err!=nil{return finishSynthetic(obs,start,false)}
	keepalive:=5*time.Second

	wg,err:=wgctrl.New()
	if err!=nil{return finishSynthetic(obs,start,false)}
	defer wg.Close()
	if err:=wg.ConfigureDevice(iface,wgtypes.Config{
		PrivateKey:&privateKey,
		Peers:[]wgtypes.PeerConfig{{
			PublicKey:serverKey,
			Endpoint:endpoint,
			ReplaceAllowedIPs:true,
			AllowedIPs:[]net.IPNet{*allowed},
			PersistentKeepaliveInterval:&keepalive,
		}},
	});err!=nil{return finishSynthetic(obs,start,false)}

	assigned:=strings.TrimSpace(session.Config.AssignedIP)
	if net.ParseIP(assigned)==nil{return finishSynthetic(obs,start,false)}
	if mtu<576||mtu>1500{mtu=1280}
	if err:=runCommand(ctx,"ip","address","add",assigned+"/32","dev",iface);err!=nil{
		return finishSynthetic(obs,start,false)
	}
	if err:=runCommand(ctx,"ip","link","set","dev",iface,"mtu",strconv.Itoa(mtu),"up");err!=nil{
		return finishSynthetic(obs,start,false)
	}
	if err:=runCommand(ctx,"ip","route","replace",gateway+"/32","dev",iface);err!=nil{
		return finishSynthetic(obs,start,false)
	}

	pingStart:=time.Now()
	if err:=runCommand(ctx,"ping","-n","-c","1","-W","3","-I",iface,gateway);err!=nil{
		return finishSynthetic(obs,start,false)
	}
	latency:=int(time.Since(pingStart).Milliseconds())

	device,err:=wg.Device(iface)
	if err!=nil{return finishSynthetic(obs,start,false)}
	handshakeOK:=false
	for _,peer:=range device.Peers{
		if peer.PublicKey==serverKey && !peer.LastHandshakeTime.IsZero() && time.Since(peer.LastHandshakeTime)<30*time.Second{
			handshakeOK=true
			break
		}
	}
	if !handshakeOK{return finishSynthetic(obs,start,false)}
	obs.Success=true
	obs.LatencyMS=latency
	return obs
}

func finishSynthetic(obs store.ProbeObservation,start time.Time,success bool)store.ProbeObservation{
	obs.Success=success
	obs.LatencyMS=int(time.Since(start).Milliseconds())
	if obs.LatencyMS>120000{obs.LatencyMS=120000}
	return obs
}

func createSyntheticSession(ctx context.Context,sessionURL string,lease accesslease.Envelope,publicKey string)(syntheticWorkerSession,error){
	body,err:=json.Marshal(map[string]any{"lease":lease,"client_public_key":publicKey})
	if err!=nil{return syntheticWorkerSession{},err}
	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,sessionURL,bytes.NewReader(body))
	if err!=nil{return syntheticWorkerSession{},err}
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("Accept","application/json")
	client:=&http.Client{Timeout:10*time.Second}
	resp,err:=client.Do(req)
	if err!=nil{return syntheticWorkerSession{},err}
	defer resp.Body.Close()
	raw,err:=io.ReadAll(io.LimitReader(resp.Body,128<<10))
	if err!=nil{return syntheticWorkerSession{},err}
	if resp.StatusCode<200||resp.StatusCode>=300{return syntheticWorkerSession{},fmt.Errorf("worker session status %d",resp.StatusCode)}
	var session syntheticWorkerSession
	if err:=json.Unmarshal(raw,&session);err!=nil{return syntheticWorkerSession{},err}
	if session.ID==""||session.Config.AssignedIP==""||session.Config.ServerPublicKey==""||session.Config.Endpoint==""{
		return syntheticWorkerSession{},fmt.Errorf("worker returned incomplete synthetic session")
	}
	return session,nil
}

func closeSyntheticSession(ctx context.Context,sessionURL,sessionID string){
	if sessionID==""{return}
	closeCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
	defer cancel()
	base:=strings.TrimRight(sessionURL,"/")
	req,err:=http.NewRequestWithContext(closeCtx,http.MethodDelete,base+"/"+sessionID,nil)
	if err!=nil{return}
	resp,err:=(&http.Client{Timeout:5*time.Second}).Do(req)
	if err==nil&&resp!=nil{_ = resp.Body.Close()}
}

func runCommand(ctx context.Context,name string,args ...string)error{
	cmd:=exec.CommandContext(ctx,name,args...)
	if out,err:=cmd.CombinedOutput();err!=nil{
		text:=strings.TrimSpace(string(out))
		if len(text)>1000{text=text[len(text)-1000:]}
		return fmt.Errorf("%s failed: %w: %s",name,err,text)
	}
	return nil
}

func joinHostPort(host string,port int)string{
	return net.JoinHostPort(strings.Trim(host,"[]"),strconv.Itoa(port))
}
