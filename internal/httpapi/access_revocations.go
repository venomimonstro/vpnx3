package httpapi

import (
	"net/http"
	"time"

	"github.com/venomimonstro/vpnx3/internal/revocations"
)

func (s *Server) handleAccessRevocations(w http.ResponseWriter,r *http.Request){
	now:=time.Now().UTC().Truncate(time.Second)
	const window=25*time.Hour
	hashes,err:=s.store.RevokedDeviceHashes(r.Context(),now.Add(-window),10000)
	if err!=nil{s.internalError(w,r,err);return}
	env,err:=revocations.Issue(s.accessSigner,revocations.Payload{
		SchemaVersion:1,
		IssuedAt:now,
		ExpiresAt:now.Add(2*time.Minute),
		WindowHours:25,
		RevokedDeviceHashes:hashes,
	})
	if err!=nil{s.internalError(w,r,err);return}
	w.Header().Set("Cache-Control","public, max-age=30")
	writeJSON(w,http.StatusOK,env)
}
