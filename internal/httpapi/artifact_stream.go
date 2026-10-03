package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/venomimonstro/vpnx3/internal/artifactstorage"
)

var errArtifactStreamStarted=errors.New("artifact response stream started")

func (s *Server) serveArtifact(
	w http.ResponseWriter,
	r *http.Request,
	storageKey string,
	fileName string,
	expectedSHA256 string,
	public bool,
) error {
	first,_,err:=s.artifacts.Open(r.Context(),storageKey)
	if errors.Is(err,artifactstorage.ErrNotFound){return artifactstorage.ErrNotFound}
	if err!=nil{return err}

	h:=sha256.New()
	_,copyErr:=io.Copy(h,first)
	closeErr:=first.Close()
	if copyErr!=nil{return copyErr}
	if closeErr!=nil{return closeErr}
	actual:=hex.EncodeToString(h.Sum(nil))
	if actual!=expectedSHA256{
		return fmt.Errorf("artifact integrity mismatch: got %s want %s",actual,expectedSHA256)
	}

	body,info,err:=s.artifacts.Open(r.Context(),storageKey)
	if errors.Is(err,artifactstorage.ErrNotFound){return artifactstorage.ErrNotFound}
	if err!=nil{return err}
	defer body.Close()

	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(s.cfg.ArtifactTransferTimeout))
	w.Header().Set("Content-Type","application/octet-stream")
	w.Header().Set("Content-Disposition",fmt.Sprintf("attachment; filename=%q",fileName))
	w.Header().Set("Content-Length",strconv.FormatInt(info.Size,10))
	w.Header().Set("Last-Modified",info.ModTime.UTC().Format(http.TimeFormat))
	w.Header().Set("X-VPNX3-SHA256",expectedSHA256)
	if public{
		w.Header().Set("ETag",fmt.Sprintf("%q",expectedSHA256))
		w.Header().Set("Cache-Control","public, max-age=300, immutable")
	}else{
		w.Header().Set("Cache-Control","no-store")
	}
	w.WriteHeader(http.StatusOK)
	if _,err=io.Copy(w,body);err!=nil{
		s.logger.Warn("artifact response stream interrupted","storage_key",storageKey,"error",err)
		return fmt.Errorf("%w: %v",errArtifactStreamStarted,err)
	}
	return nil
}
