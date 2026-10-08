package httpapi

import (
	"net/http"
	"time"

	"github.com/venomimonstro/vpnx3/internal/revocation"
)

func (s *Server) handleRevocationSnapshot(w http.ResponseWriter,r *http.Request){
	data,err:=s.store.RevocationSnapshot(r.Context())
	if err!=nil{s.internalError(w,r,err);return}
	env,err:=revocation.Issue(
		s.accessSigner,data.Version,data.DeviceIDs,time.Now().UTC(),10*time.Minute,
	)
	if err!=nil{s.internalError(w,r,err);return}
	w.Header().Set("Cache-Control","no-store")
	writeJSON(w,http.StatusOK,env)
}
