package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type releaseEnvelope struct {
	Payload string `json:"payload"`
	Signature string `json:"signature"`
	KeyID string `json:"key_id"`
}

func (s *Server) handleLatestRelease(w http.ResponseWriter,r *http.Request) {
	if s.releaseSigner==nil { writeError(w,http.StatusServiceUnavailable,"release_channel_disabled");return }
	target:=strings.TrimSpace(r.URL.Query().Get("target"))
	switch target {
	case "android_apk","android_aab","chrome_zip","firefox_zip","ios_ipa":
	default:
		writeError(w,http.StatusBadRequest,"invalid_release_target");return
	}
	a,err:=s.store.LatestPublishedArtifact(r.Context(),target)
	if err==store.ErrNotFound { writeError(w,http.StatusNotFound,"release_not_found");return }
	if err!=nil{s.internalError(w,r,err);return}

	payload:=map[string]any{
		"schema_version":1,
		"release_id":a.ReleaseID,
		"version":a.Version,
		"artifact_id":a.ArtifactID,
		"target":a.Target,
		"file_name":a.FileName,
		"sha256":a.SHA256,
		"size_bytes":a.SizeBytes,
		"download_path":fmt.Sprintf("/api/v1/releases/%s/artifacts/%s/download",a.ReleaseID,a.ArtifactID),
		"issued_at":time.Now().UTC().Truncate(time.Second),
		"expires_at":time.Now().UTC().Add(10*time.Minute).Truncate(time.Second),
	}
	raw,err:=json.Marshal(payload);if err!=nil{s.internalError(w,r,err);return}
	sig:=s.releaseSigner.Sign(raw)
	w.Header().Set("Cache-Control","public, max-age=60")
	writeJSON(w,http.StatusOK,releaseEnvelope{
		Payload:base64.RawURLEncoding.EncodeToString(raw),
		Signature:base64.RawURLEncoding.EncodeToString(sig),
		KeyID:s.releaseSigner.KeyID(),
	})
}

func (s *Server) handleReleaseSigningKey(w http.ResponseWriter,r *http.Request) {
	if s.releaseSigner==nil { writeError(w,http.StatusServiceUnavailable,"release_channel_disabled");return }
	writeJSON(w,http.StatusOK,map[string]string{
		"key_id":s.releaseSigner.KeyID(),
		"public_key":s.releaseSigner.PublicKeyBase64(),
		"algorithm":"ed25519",
	})
}

func (s *Server) handlePublicArtifactDownload(w http.ResponseWriter,r *http.Request) {
	a,err:=s.store.PublishedArtifactByID(r.Context(),r.PathValue("id"),r.PathValue("artifactId"))
	if err!=nil{writeError(w,http.StatusNotFound,"artifact_not_found");return}
	root,err:=filepath.Abs(s.cfg.ArtifactDir);if err!=nil{s.internalError(w,r,err);return}
	full,err:=filepath.Abs(filepath.Join(root,filepath.FromSlash(a.StorageKey)));if err!=nil{s.internalError(w,r,err);return}
	rel,err:=filepath.Rel(root,full)
	if err!=nil||rel==".."||strings.HasPrefix(rel,".."+string(os.PathSeparator)){
		s.internalError(w,r,fmt.Errorf("artifact path escaped storage root"));return
	}
	f,err:=os.Open(full);if err!=nil{writeError(w,http.StatusNotFound,"artifact_file_missing");return}
	defer f.Close()
	stat,err:=f.Stat();if err!=nil{s.internalError(w,r,err);return}
	w.Header().Set("Content-Disposition",fmt.Sprintf("attachment; filename=%q",a.FileName))
	w.Header().Set("X-VPNX3-SHA256",a.SHA256)
	w.Header().Set("Cache-Control","public, max-age=300, immutable")
	http.ServeContent(w,r,a.FileName,stat.ModTime(),f)
}
