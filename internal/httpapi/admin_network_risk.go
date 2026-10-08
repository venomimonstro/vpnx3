package httpapi

import "net/http"

func (s *Server) handleNetworkRisk(w http.ResponseWriter,r *http.Request){
	data,err:=s.store.NetworkRisk(r.Context())
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,data)
}
