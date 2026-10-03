package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/venomimonstro/vpnx3/internal/store"
)

func (s *Server) handleListNodeEndpoints(w http.ResponseWriter,r *http.Request) {
	endpoints,err:=s.store.ListNodeEndpoints(r.Context(),r.PathValue("id"))
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"endpoints":endpoints})
}

func (s *Server) handleCreateNodeEndpoint(w http.ResponseWriter,r *http.Request) {
	var req struct {
		Kind string `json:"kind"`
		Transport string `json:"transport"`
		Scheme string `json:"scheme"`
		Host string `json:"host"`
		Port int `json:"port"`
		Path string `json:"path"`
		Priority int `json:"priority"`
	}
	if err:=decodeJSON(w,r,&req); err!=nil { return }
	ep,err:=s.store.CreateNodeEndpoint(r.Context(),store.CreateNodeEndpointInput{
		NodeID:r.PathValue("id"),Kind:req.Kind,Transport:req.Transport,Scheme:req.Scheme,
		Host:req.Host,Port:req.Port,Path:req.Path,Priority:req.Priority,
	})
	if errors.Is(err,store.ErrNotFound) { writeError(w,http.StatusNotFound,"node_not_found"); return }
	if err!=nil {
		if strings.Contains(err.Error(),"invalid") || strings.Contains(err.Error(),"requires") {
			writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_endpoint","detail":err.Error()}); return
		}
		s.internalError(w,r,err); return
	}
	admin,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"node.endpoint.create","node_endpoint",ep.ID,requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	writeJSON(w,http.StatusCreated,ep)
}

func (s *Server) handleDeleteNodeEndpoint(w http.ResponseWriter,r *http.Request) {
	err:=s.store.DeleteNodeEndpoint(r.Context(),r.PathValue("id"),r.PathValue("endpointId"))
	if errors.Is(err,store.ErrNotFound) { writeError(w,http.StatusNotFound,"endpoint_not_found"); return }
	if err!=nil { s.internalError(w,r,err); return }
	admin,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"node.endpoint.delete","node_endpoint",r.PathValue("endpointId"),requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	w.WriteHeader(http.StatusNoContent)
}
