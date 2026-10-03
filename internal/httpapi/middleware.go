package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"runtime/debug"
	"time"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	clientIPKey contextKey = "client_ip"
)

func requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)

		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func requestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)

		logger.Info("http request",
			"request_id", requestIDFromContext(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func recoverer(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered",
					"request_id", requestIDFromContext(r.Context()),
					"error", recovered,
					"stack", string(debug.Stack()),
				)
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": "internal_server_error",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func newRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(raw[:])
}


func trustedProxyContext(cidrs []string,next http.Handler) http.Handler {
	networks:=make([]*net.IPNet,0,len(cidrs))
	for _,raw:=range cidrs{
		_,network,err:=net.ParseCIDR(strings.TrimSpace(raw))
		if err==nil && network!=nil{networks=append(networks,network)}
	}
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		peer:=remoteIP(r.RemoteAddr)
		client:=peer
		if peer!=nil && ipInNetworks(peer,networks){
			if forwarded:=firstForwardedIP(r.Header.Get("X-Forwarded-For"));forwarded!=nil{
				client=forwarded
			}else if real:=net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP")));real!=nil{
				client=real
			}
		}
		ctx:=context.WithValue(r.Context(),clientIPKey,client)
		next.ServeHTTP(w,r.WithContext(ctx))
	})
}

func remoteIP(remoteAddr string) net.IP {
	host,_,err:=net.SplitHostPort(remoteAddr)
	if err==nil{
		if ip:=net.ParseIP(host);ip!=nil{return ip}
	}
	return net.ParseIP(remoteAddr)
}

func firstForwardedIP(raw string) net.IP {
	for _,part:=range strings.Split(raw,","){
		value:=strings.TrimSpace(part)
		if value==""{continue}
		if ip:=net.ParseIP(value);ip!=nil{return ip}
		// RFC 7239-like accidental host:port values are tolerated only when
		// SplitHostPort can unambiguously parse them.
		if host,_,err:=net.SplitHostPort(value);err==nil{
			if ip:=net.ParseIP(host);ip!=nil{return ip}
		}
	}
	return nil
}

func ipInNetworks(ip net.IP,networks []*net.IPNet) bool {
	for _,network:=range networks{
		if network.Contains(ip){return true}
	}
	return false
}
