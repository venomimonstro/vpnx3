package httpapi

import (
	"net/http"
	"strconv"
)

func (s *Server) handleRecentProbeResults(w http.ResponseWriter,r *http.Request) {
	limit:=200
	if raw:=r.URL.Query().Get("limit"); raw!="" {
		if parsed,err:=strconv.Atoi(raw); err==nil { limit=parsed }
	}
	results,err:=s.store.RecentProbeResults(r.Context(),limit)
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"results":results})
}
