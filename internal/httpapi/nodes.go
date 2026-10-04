package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.store.ListNodes(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
}

func (s *Server) handleGetNode(w http.ResponseWriter,r *http.Request) {
	node,err:=s.store.NodeByID(r.Context(),r.PathValue("id"))
	if errors.Is(err,store.ErrNotFound) {
		writeError(w,http.StatusNotFound,"node_not_found")
		return
	}
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,node)
}

func (s *Server) handleNodeEvents(w http.ResponseWriter,r *http.Request) {
	limit:=100
	if raw:=r.URL.Query().Get("limit"); raw!="" {
		if parsed,err:=strconv.Atoi(raw); err==nil { limit=parsed }
	}
	events,err:=s.store.NodeStateEvents(r.Context(),r.PathValue("id"),limit)
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"events":events})
}

func (s *Server) handleCreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role       string `json:"role"`
		TTLMinutes int    `json:"ttl_minutes"`
	}
	if err := decodeJSON(w, r, &req); err != nil { return }
	if req.TTLMinutes == 0 { req.TTLMinutes = 10 }
	if req.TTLMinutes < 1 || req.TTLMinutes > 60 {
		writeError(w, http.StatusBadRequest, "invalid_ttl")
		return
	}

	admin, _ := adminFromContext(r.Context())
	token, expires, err := s.store.CreateEnrollmentToken(r.Context(), strings.TrimSpace(req.Role), admin.ID, time.Duration(req.TTLMinutes)*time.Minute)
	if err != nil {
		if strings.Contains(err.Error(), "invalid node role") {
			writeError(w, http.StatusBadRequest, "invalid_role")
			return
		}
		s.internalError(w, r, err)
		return
	}
	ip := clientIP(r)
	ipValue:=""
	if ip!=nil { ipValue=ip.String() }
	_ = s.store.WriteAudit(r.Context(), "admin", admin.ID, "node.enrollment_token.create", "enrollment_token", "", requestIDFromContext(r.Context()), ipValue, "success")
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"expires_at": expires,
		"role": req.Role,
	})
}

func (s *Server) handleNodeTransition(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		var req struct { Reason string `json:"reason"` }
		if r.Body != nil && r.ContentLength != 0 {
			if err := decodeJSON(w, r, &req); err != nil { return }
		}
		admin, _ := adminFromContext(r.Context())
		node, err := s.store.TransitionNode(r.Context(), nodeID, target, strings.TrimSpace(req.Reason), "admin", admin.ID)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "node_not_found")
			return
		}
		if err != nil {
			if strings.Contains(err.Error(), "invalid node transition") || strings.Contains(err.Error(), "cannot retire") {
				writeJSON(w, http.StatusConflict, map[string]string{"error":"invalid_transition","detail":err.Error()})
				return
			}
			s.internalError(w, r, err)
			return
		}
		ip := clientIP(r)
		ipValue:=""
		if ip!=nil { ipValue=ip.String() }
		action := fmt.Sprintf("node.%s", target)
		_ = s.store.WriteAudit(r.Context(), "admin", admin.ID, action, "node", node.ID, requestIDFromContext(r.Context()), ipValue, "success")
		if s.onConfigChange!=nil { s.onConfigChange() }
		writeJSON(w, http.StatusOK, node)
	}
}
