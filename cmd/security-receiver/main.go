package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type envelope struct {
	KeyID string `json:"key_id"`
	Payload string `json:"payload"`
	Signature string `json:"signature"`
}

type auditPayload struct {
	SchemaVersion int `json:"schema_version"`
	AuditID int64 `json:"audit_id"`
	EntryHash string `json:"entry_hash"`
	PrevHash string `json:"prev_hash"`
	Action string `json:"action"`
	ResourceType string `json:"resource_type"`
	CreatedAt time.Time `json:"created_at"`
}

type receiver struct {
	publicKey ed25519.PublicKey
	keyID string
	file *os.File
	mu sync.Mutex
}

func main(){
	addr:=env("VPNX3_RECEIVER_ADDR","127.0.0.1:9099")
	host,_,err:=net.SplitHostPort(addr)
	if err!=nil{log.Fatal(err)}
	ip:=net.ParseIP(host)
	if ip==nil||!ip.IsLoopback(){
		log.Fatal("test security receiver must listen on loopback only")
	}
	rawKey:=strings.TrimSpace(os.Getenv("VPNX3_SECURITY_EXPORT_PUBLIC_KEY"))
	key,err:=base64.RawURLEncoding.DecodeString(rawKey)
	if err!=nil||len(key)!=ed25519.PublicKeySize{log.Fatal("VPNX3_SECURITY_EXPORT_PUBLIC_KEY must be 32-byte base64url public key")}
	output:=env("VPNX3_RECEIVER_OUTPUT","./security-events.ndjson")
	if err:=os.MkdirAll(filepath.Dir(output),0700);err!=nil{log.Fatal(err)}
	f,err:=os.OpenFile(output,os.O_CREATE|os.O_WRONLY|os.O_APPEND,0600)
	if err!=nil{log.Fatal(err)}
	defer f.Close()

	r:=&receiver{publicKey:ed25519.PublicKey(key),file:f}
	mux:=http.NewServeMux()
	mux.HandleFunc("POST /events",r.handle)
	mux.HandleFunc("GET /health",func(w http.ResponseWriter,_ *http.Request){w.WriteHeader(http.StatusNoContent)})
	s:=&http.Server{Addr:addr,Handler:mux,ReadHeaderTimeout:5*time.Second,ReadTimeout:10*time.Second,WriteTimeout:10*time.Second,IdleTimeout:30*time.Second}
	log.Printf("VPNX3 security test receiver listening on %s; output=%s",addr,output)
	log.Fatal(s.ListenAndServe())
}

func (r *receiver) handle(w http.ResponseWriter,req *http.Request){
	body,err:=io.ReadAll(http.MaxBytesReader(w,req.Body,1<<20))
	if err!=nil{http.Error(w,"invalid body",http.StatusBadRequest);return}
	var env envelope
	if err:=json.Unmarshal(body,&env);err!=nil{http.Error(w,"invalid envelope",http.StatusBadRequest);return}
	payload,err:=base64.RawURLEncoding.DecodeString(env.Payload)
	if err!=nil{http.Error(w,"invalid payload",http.StatusBadRequest);return}
	sig,err:=base64.RawURLEncoding.DecodeString(env.Signature)
	if err!=nil||!ed25519.Verify(r.publicKey,payload,sig){http.Error(w,"invalid signature",http.StatusUnauthorized);return}
	var event auditPayload
	if err:=json.Unmarshal(payload,&event);err!=nil||event.SchemaVersion!=1||event.AuditID<=0||event.EntryHash==""{
		http.Error(w,"invalid event",http.StatusBadRequest);return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _,err:=r.file.Write(append(payload,'\n'));err!=nil{http.Error(w,"write failed",http.StatusInternalServerError);return}
	if err:=r.file.Sync();err!=nil{http.Error(w,"sync failed",http.StatusInternalServerError);return}
	w.Header().Set("Content-Type","application/json")
	w.WriteHeader(http.StatusAccepted)
	_,_=fmt.Fprintf(w,"{\"accepted\":true,\"audit_id\":%d}\n",event.AuditID)
}

func env(key,fallback string)string{
	if value:=strings.TrimSpace(os.Getenv(key));value!=""{return value}
	return fallback
}
