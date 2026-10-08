package httpapi

import "net/http"

func (s *Server) handleFleetHealth(w http.ResponseWriter,r *http.Request){
	summary,nodes,err:=s.store.FleetHealth(r.Context())
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,map[string]any{
		"summary":summary,
		"nodes":nodes,
	})
}
