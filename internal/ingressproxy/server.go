package ingressproxy

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/venomimonstro/vpnx3/internal/proxylease"
)

type Server struct {
	http *http.Server
	logger *slog.Logger
	verifier *proxylease.Verifier
	mu sync.Mutex
	active map[string]int
	maxPerLease int
}

func New(addr string,logger *slog.Logger,verifier *proxylease.Verifier)*Server{
	s:=&Server{logger:logger,verifier:verifier,active:map[string]int{},maxPerLease:32}
	s.http=&http.Server{
		Addr:addr,Handler:s,
		ReadHeaderTimeout:10*time.Second,
		IdleTimeout:90*time.Second,
		MaxHeaderBytes:32<<10,
		TLSNextProto:map[string]func(*http.Server,*tls.Conn,http.Handler){},
	}
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter,r *http.Request){
	claims,ok:=s.authorize(w,r)
	if !ok{return}
	if !s.acquire(claims.LeaseID){
		http.Error(w,"too many proxy connections",http.StatusTooManyRequests);return
	}
	defer s.release(claims.LeaseID)

	if r.Method==http.MethodConnect{s.handleConnect(w,r);return}
	s.handleHTTP(w,r)
}

func (s *Server) authorize(w http.ResponseWriter,r *http.Request)(proxylease.Claims,bool){
	raw:=strings.TrimSpace(r.Header.Get("Proxy-Authorization"))
	if !strings.HasPrefix(raw,"Basic "){
		w.Header().Set("Proxy-Authenticate",`Basic realm="VPNX3"`)
		http.Error(w,"proxy authentication required",http.StatusProxyAuthRequired)
		return proxylease.Claims{},false
	}
	decoded,err:=base64.StdEncoding.DecodeString(strings.TrimPrefix(raw,"Basic "))
	if err!=nil{http.Error(w,"invalid proxy credentials",http.StatusProxyAuthRequired);return proxylease.Claims{},false}
	user,pass,ok:=strings.Cut(string(decoded),":")
	if !ok||user!="vpnx3"{http.Error(w,"invalid proxy credentials",http.StatusProxyAuthRequired);return proxylease.Claims{},false}
	env,err:=proxylease.DecodeCredential(pass)
	if err!=nil{http.Error(w,"invalid proxy credentials",http.StatusProxyAuthRequired);return proxylease.Claims{},false}
	claims,err:=s.verifier.Verify(env,time.Now().UTC())
	if err!=nil{http.Error(w,"expired or invalid proxy lease",http.StatusProxyAuthRequired);return proxylease.Claims{},false}
	return claims,true
}

func (s *Server) acquire(id string)bool{
	s.mu.Lock();defer s.mu.Unlock()
	if s.active[id]>=s.maxPerLease{return false}
	s.active[id]++
	return true
}
func (s *Server) release(id string){
	s.mu.Lock();defer s.mu.Unlock()
	if s.active[id]<=1{delete(s.active,id)}else{s.active[id]--}
}

func (s *Server) handleConnect(w http.ResponseWriter,r *http.Request){
	host,port,err:=splitWebTarget(r.Host)
	if err!=nil{http.Error(w,"invalid target",http.StatusBadRequest);return}
	conn,err:=dialPublic(r.Context(),host,port)
	if err!=nil{http.Error(w,"target unavailable",http.StatusBadGateway);return}
	defer conn.Close()
	hj,ok:=w.(http.Hijacker)
	if !ok{http.Error(w,"proxy transport unavailable",http.StatusInternalServerError);return}
	client,buf,err:=hj.Hijack()
	if err!=nil{return}
	defer client.Close()
	_,_ = buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	_ = buf.Flush()

	done:=make(chan struct{},2)
	go func(){_,_=io.Copy(conn,client);done<-struct{}{}}()
	go func(){_,_=io.Copy(client,conn);done<-struct{}{}}()
	<-done
}

func (s *Server) handleHTTP(w http.ResponseWriter,r *http.Request){
	if r.URL==nil||r.URL.Scheme!="http"{
		http.Error(w,"only HTTP or CONNECT is supported",http.StatusBadRequest);return
	}
	host:=r.URL.Hostname()
	port:=r.URL.Port();if port==""{port="80"}
	target,err:=resolvePublic(r.Context(),host,port)
	if err!=nil{http.Error(w,"target unavailable",http.StatusBadGateway);return}

	req:=r.Clone(r.Context())
	req.RequestURI=""
	req.Header=req.Header.Clone()
	req.Header.Del("Proxy-Authorization")
	removeHopHeaders(req.Header)

	transport:=&http.Transport{
		Proxy:nil,
		DialContext:func(ctx context.Context,network,address string)(net.Conn,error){
			d:=net.Dialer{Timeout:10*time.Second,KeepAlive:30*time.Second}
			return d.DialContext(ctx,"tcp",target)
		},
		DisableCompression:false,
		ForceAttemptHTTP2:false,
	}
	defer transport.CloseIdleConnections()
	resp,err:=transport.RoundTrip(req)
	if err!=nil{http.Error(w,"target unavailable",http.StatusBadGateway);return}
	defer resp.Body.Close()
	removeHopHeaders(resp.Header)
	for k,values:=range resp.Header{for _,v:=range values{w.Header().Add(k,v)}}
	w.WriteHeader(resp.StatusCode)
	_,_=io.Copy(w,resp.Body)
}

func splitWebTarget(hostport string)(string,string,error){
	host,port,err:=net.SplitHostPort(hostport)
	if err!=nil{return "","",err}
	if port!="443"&&port!="80"{return "","",fmt.Errorf("port not allowed")}
	return host,port,nil
}

func dialPublic(ctx context.Context,host,port string)(net.Conn,error){
	target,err:=resolvePublic(ctx,host,port);if err!=nil{return nil,err}
	d:=net.Dialer{Timeout:10*time.Second,KeepAlive:30*time.Second}
	return d.DialContext(ctx,"tcp",target)
}

func resolvePublic(ctx context.Context,host,port string)(string,error){
	if port!="80"&&port!="443"{return "",fmt.Errorf("port not allowed")}
	if strings.EqualFold(host,"localhost"){return "",fmt.Errorf("local target blocked")}
	ips,err:=net.DefaultResolver.LookupIPAddr(ctx,host);if err!=nil{return "",err}
	for _,entry:=range ips{
		ip:=entry.IP
		if isPublicIP(ip){return net.JoinHostPort(ip.String(),port),nil}
	}
	return "",fmt.Errorf("no public address")
}

func isPublicIP(ip net.IP)bool{
	if ip==nil||ip.IsLoopback()||ip.IsPrivate()||ip.IsUnspecified()||ip.IsMulticast(){return false}
	if ip.IsLinkLocalUnicast()||ip.IsLinkLocalMulticast(){return false}
	if v4:=ip.To4();v4!=nil{
		if v4[0]==169&&v4[1]==254{return false}
		if v4[0]==100&&v4[1]>=64&&v4[1]<=127{return false}
		if v4[0]==192&&v4[1]==0&&v4[2]==0{return false}
	}
	return true
}

func removeHopHeaders(h http.Header){
	for _,k:=range []string{"Connection","Proxy-Connection","Keep-Alive","Proxy-Authenticate","Proxy-Authorization","TE","Trailer","Transfer-Encoding","Upgrade"}{h.Del(k)}
}

func (s *Server) ListenAndServeTLS(cert,key string)error{return s.http.ListenAndServeTLS(cert,key)}
func (s *Server) Shutdown(ctx context.Context)error{return s.http.Shutdown(ctx)}
