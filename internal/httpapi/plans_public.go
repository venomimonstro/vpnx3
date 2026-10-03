package httpapi

import "net/http"

func (s *Server) handlePublicPlans(w http.ResponseWriter,r *http.Request) {
	plans,err:=s.store.ListPlans(r.Context(),true)
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"plans":plans})
}
