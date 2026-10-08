package httpapi

import (
	"net/http"
)

func (s *Server) handleTrustBundle(w http.ResponseWriter,r *http.Request){
	if s.trustBundle==nil{
		writeError(w,http.StatusServiceUnavailable,"trust_bundle_disabled")
		return
	}
	w.Header().Set("Cache-Control","public, max-age=60")
	writeJSON(w,http.StatusOK,*s.trustBundle)
}
