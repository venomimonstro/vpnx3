package httpapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const personalVLESSSocket = "/run/vpnx3-personal-vless/manager.sock"

var personalVLESSUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type personalVLESSRequest struct {
	Action string `json:"action"`
	Name   string `json:"name,omitempty"`
	ID     string `json:"id,omitempty"`
}

func personalVLESSCall(req personalVLESSRequest) (map[string]any, error) {
	conn, err := net.DialTimeout("unix", personalVLESSSocket, 2*time.Second)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("vless_manager_not_installed")
		}
		return nil, fmt.Errorf("vless_manager_unavailable: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(35 * time.Second)); err != nil {
		return nil, err
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}
	r := bufio.NewReaderSize(conn, 4096)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("vless_manager_response_failed: %w", err)
	}
	if len(line) > 256*1024 {
		return nil, fmt.Errorf("vless_manager_response_too_large")
	}
	var response map[string]any
	if err := json.Unmarshal(line, &response); err != nil {
		return nil, fmt.Errorf("invalid_vless_manager_response: %w", err)
	}
	if response["ok"] != true {
		message, _ := response["error"].(string)
		if message == "" {
			message = "vless_manager_operation_failed"
		}
		return nil, fmt.Errorf("%s", message)
	}
	return response, nil
}

func (s *Server) handlePersonalVLESS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	var req personalVLESSRequest
	switch r.Method {
	case http.MethodGet:
		req.Action = "status"
	case http.MethodPost:
		if r.URL.Path == "/api/v1/admin/personal-vless/check" {
			req.Action = "check"
			break
		}
		var body struct{ Name string `json:"name"` }
		if err := decodeJSON(w, r, &body); err != nil { return }
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" || len([]rune(body.Name)) > 64 || strings.ContainsAny(body.Name, "\x00\r\n") {
			writeError(w, http.StatusBadRequest, "invalid_profile_name")
			return
		}
		req.Action, req.Name = "create", body.Name
	case http.MethodDelete:
		id := r.PathValue("id")
		if !personalVLESSUUID.MatchString(id) {
			writeError(w, http.StatusBadRequest, "invalid_profile_id")
			return
		}
		req.Action, req.ID = "revoke", id
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	result, err := personalVLESSCall(req)
	if err != nil {
		s.logger.Warn("personal VLESS manager failed", "action", req.Action, "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "personal_vless_unavailable", "detail": err.Error()})
		return
	}
	if req.Action != "status" {
		admin, _ := adminFromContext(r.Context())
		_ = s.store.WriteAudit(r.Context(), "admin", admin.ID, "personal_vless."+req.Action, "personal_vless", req.ID,
			requestIDFromContext(r.Context()), ipString(clientIP(r)), "success")
	}
	writeJSON(w, http.StatusOK, result)
}
