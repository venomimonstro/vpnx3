package httpapi

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"path"
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
	storageKey:=path.Join(job.ReleaseID,job.JobID+path.Ext(fileName))

	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(s.cfg.ArtifactTransferTimeout))
	info,err:=s.artifacts.PutVerified(r.Context(),storageKey,r.Body,s.cfg.ArtifactMaxBytes,contentHash)
	if err!=nil{
		if strings.Contains(err.Error(),"size limit") || strings.Contains(err.Error(),"hash mismatch"){
			writeJSON(w,http.StatusBadRequest,map[string]string{"error":"artifact_upload_failed","detail":err.Error()})
			return
		}
		s.internalError(w,r,fmt.Errorf("store artifact: %w",err));return
	}

	if err:=s.store.RegisterArtifactAndSucceed(
		r.Context(),job.JobID,nodeID,fileName,storageKey,contentHash,info.Size,
	);err!=nil{
		_ = s.artifacts.Delete(r.Context(),storageKey)
		s.internalError(w,r,fmt.Errorf("finalize artifact: %w",err));return
	}
	writeJSON(w,http.StatusCreated,map[string]any{
		"file_name":fileName,"sha256":contentHash,"size_bytes":info.Size,
	})
}
