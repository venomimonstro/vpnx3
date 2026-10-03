package httpapi

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (s *Server) handlePublishConfig(w http.ResponseWriter,r *http.Request) {
	admin,_:=adminFromContext(r.Context())
	env,err:=s.configService.Publish(r.Context(),admin.ID)
	if err!=nil { s.internalError(w,r,err); return }
	ip:=clientIP(r)
	ipValue:=""
	if ip!=nil { ipValue=ip.String() }
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"config.publish","config_manifest","",requestIDFromContext(r.Context()),ipValue,"success")
	writeJSON(w,http.StatusCreated,env)
}

func (s *Server) handleLatestConfig(w http.ResponseWriter,r *http.Request) {
	env,err:=s.configService.Latest(r.Context())
	if errors.Is(err,pgx.ErrNoRows) {
		writeError(w,http.StatusNotFound,"config_not_published")
		return
	}
	if err!=nil { s.internalError(w,r,err); return }
	w.Header().Set("Cache-Control","public, max-age=30")
	writeJSON(w,http.StatusOK,env)
}

func (s *Server) handleConfigSigningKey(w http.ResponseWriter,r *http.Request) {
	writeJSON(w,http.StatusOK,map[string]string{
		"key_id":s.configSigner.KeyID(),
		"public_key":s.configSigner.PublicKeyBase64(),
		"algorithm":"ed25519",
	})
}
