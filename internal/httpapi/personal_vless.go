package httpapi

import (
	"bufio"
	"encoding/base64"
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
	Mode   string `json:"mode,omitempty"`
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
	deadline := 35 * time.Second
	if req.Action == "openvpn_create" || req.Action == "openvpn_revoke" {
		deadline = 105 * time.Second
	}
	if err := conn.SetDeadline(time.Now().Add(deadline)); err != nil {
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
		if r.URL.Path == "/api/v1/admin/personal-vless/alternate-port" {
			req.Action = "enable_alternate_port"
			break
		}
		if r.URL.Path == "/api/v1/admin/personal-vless/check" {
			req.Action = "check"
			break
		}
		if id := r.PathValue("id"); id != "" && strings.HasSuffix(r.URL.Path, "/check") {
			if !personalVLESSUUID.MatchString(id) {
				writeError(w, http.StatusBadRequest, "invalid_profile_id")
				return
			}
			req.Action, req.ID = "check", id
			break
		}
		var body struct{
			Name string `json:"name"`
			Mode string `json:"mode"`
		}
		if err := decodeJSON(w, r, &body); err != nil { return }
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" || len([]rune(body.Name)) > 64 || strings.ContainsAny(body.Name, "\x00\r\n") {
			writeError(w, http.StatusBadRequest, "invalid_profile_name")
			return
		}
		if body.Mode == "" { body.Mode = "vision" }
		if body.Mode != "vision" && body.Mode != "ios" {
			writeError(w, http.StatusBadRequest, "invalid_profile_mode")
			return
		}
		req.Action, req.Name, req.Mode = "create", body.Name, body.Mode
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
	if req.Action == "create" || req.Action == "revoke" {
		admin, _ := adminFromContext(r.Context())
		_ = s.store.WriteAudit(r.Context(), "admin", admin.ID, "personal_vless."+req.Action, "personal_vless", req.ID,
			requestIDFromContext(r.Context()), ipString(clientIP(r)), "success")
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePersonalOpenVPN(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	req := personalVLESSRequest{}
	name := r.PathValue("name")
	if name != "" && !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,39}$`).MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid_openvpn_name")
		return
	}
	req.Name = name
	switch {
	case r.Method == http.MethodGet && name == "":
		req.Action = "openvpn_status"
	case r.Method == http.MethodPost && name == "":
		var body struct { Name string `json:"name"` }
		if err := decodeJSON(w, r, &body); err != nil { return }
		if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,39}$`).MatchString(body.Name) {
			writeError(w, http.StatusBadRequest, "invalid_openvpn_name")
			return
		}
		req.Action, req.Name = "openvpn_create", body.Name
	case r.Method == http.MethodGet && name != "":
		req.Action = "openvpn_download"
	case r.Method == http.MethodDelete && name != "":
		req.Action = "openvpn_revoke"
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	result, err := personalVLESSCall(req)
	if err != nil {
		s.logger.Warn("OpenVPN manager operation failed", "action", req.Action, "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error":"openvpn_operation_failed", "detail":err.Error()})
		return
	}
	if req.Action == "openvpn_download" {
		encoded, ok := result["content_base64"].(string)
		if !ok { writeError(w, http.StatusBadGateway, "openvpn_profile_missing"); return }
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(data)>96*1024 { writeError(w, http.StatusBadGateway, "invalid_openvpn_profile"); return }
		w.Header().Set("Content-Type", "application/x-openvpn-profile")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+name+".ovpn\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	} else {
		writeJSON(w, http.StatusOK, result)
	}
	if req.Action == "openvpn_create" || req.Action == "openvpn_revoke" || req.Action == "openvpn_download" {
		admin, _ := adminFromContext(r.Context())
		_ = s.store.WriteAudit(r.Context(), "admin", admin.ID, req.Action, "openvpn_profile", req.Name,
			requestIDFromContext(r.Context()), ipString(clientIP(r)), "success")
	}
}
