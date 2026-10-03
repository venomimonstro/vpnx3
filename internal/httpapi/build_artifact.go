package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

func (s *Server) handleBuildArtifact(w http.ResponseWriter,r *http.Request) {
	nodeID:=strings.TrimSpace(r.Header.Get("X-VPNX3-Node-ID"))
	contentHash:=strings.ToLower(strings.TrimSpace(r.Header.Get("X-VPNX3-Content-SHA256")))
	rawSeq:=strings.TrimSpace(r.Header.Get("X-VPNX3-Sequence"))
	sequence,err:=strconv.ParseInt(rawSeq,10,64)
	if err!=nil || len(contentHash)!=64 {
		writeError(w,http.StatusBadRequest,"invalid_artifact_headers");return
	}
	if _,err:=hex.DecodeString(contentHash);err!=nil{
		writeError(w,http.StatusBadRequest,"invalid_artifact_hash");return
	}
	verifiedNode,ok:=s.verifyBuildWorkerRequest(w,r,[]byte(contentHash),sequence)
	if !ok{return}
	if nodeID!=verifiedNode{writeError(w,http.StatusUnauthorized,"invalid_build_worker");return}

	job,err:=s.store.ArtifactJobForWorker(r.Context(),r.PathValue("id"),nodeID)
	if err!=nil || job.Status!="running" {
		writeError(w,http.StatusNotFound,"build_job_not_found");return
	}
	fileName,err:=store.SafeArtifactFileName(job.Version,job.Target)
	if err!=nil{s.internalError(w,r,err);return}

	dir:=filepath.Join(s.cfg.ArtifactDir,job.ReleaseID)
	if err:=os.MkdirAll(dir,0750);err!=nil{s.internalError(w,r,err);return}
	finalPath:=filepath.Join(dir,job.JobID+filepath.Ext(fileName))
	tmp,err:=os.CreateTemp(dir,"upload-*")
	if err!=nil{s.internalError(w,r,err);return}
	tmpPath:=tmp.Name()
	defer func(){tmp.Close();os.Remove(tmpPath)}()

	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(s.cfg.ArtifactTransferTimeout))
	hasher:=sha256.New()
	limited:=http.MaxBytesReader(w,r.Body,s.cfg.ArtifactMaxBytes)
	written,copyErr:=io.Copy(io.MultiWriter(tmp,hasher),limited)
	closeErr:=tmp.Close()
	if copyErr!=nil || closeErr!=nil {
		writeError(w,http.StatusBadRequest,"artifact_upload_failed");return
	}
	actual:=hex.EncodeToString(hasher.Sum(nil))
	if actual!=contentHash {
		writeError(w,http.StatusBadRequest,"artifact_hash_mismatch");return
	}
	if err:=os.Chmod(tmpPath,0640);err!=nil{s.internalError(w,r,err);return}
	if err:=os.Rename(tmpPath,finalPath);err!=nil{s.internalError(w,r,err);return}

	storageKey:=filepath.ToSlash(filepath.Join(job.ReleaseID,filepath.Base(finalPath)))
	if err:=s.store.RegisterArtifactAndSucceed(r.Context(),job.JobID,nodeID,fileName,storageKey,actual,written);err!=nil{
		_ = os.Remove(finalPath)
		s.internalError(w,r,fmt.Errorf("finalize artifact: %w",err));return
	}
	writeJSON(w,http.StatusCreated,map[string]any{
		"file_name":fileName,"sha256":actual,"size_bytes":written,
	})
}
