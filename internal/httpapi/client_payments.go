package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/deviceauth"
	"github.com/venomimonstro/vpnx3/internal/store"
	"github.com/venomimonstro/vpnx3/internal/resilience"
)

func (s *Server) handleClientCreatePayment(w http.ResponseWriter,r *http.Request) {
	if s.yooKassa==nil {
		writeError(w,http.StatusServiceUnavailable,"payments_unavailable")
		return
	}
	body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,64<<10))
	if err!=nil { writeError(w,http.StatusBadRequest,"invalid_body"); return }
	var req struct {
		Sequence int64 `json:"sequence"`
		PlanID string `json:"plan_id"`
		AutoRenew bool `json:"auto_renew"`
	}
	decoder:=json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&req); err!=nil || req.Sequence<=0 || strings.TrimSpace(req.PlanID)=="" {
		writeError(w,http.StatusBadRequest,"invalid_json"); return
	}

	deviceID:=strings.TrimSpace(r.Header.Get("X-VPNX3-Device-ID"))
	ts:=strings.TrimSpace(r.Header.Get("X-VPNX3-Timestamp"))
	sig:=strings.TrimSpace(r.Header.Get("X-VPNX3-Signature"))
	state,err:=s.store.DeviceAuthState(r.Context(),deviceID)
	if err!=nil || state.Status!="active" { writeError(w,http.StatusUnauthorized,"invalid_device"); return }
	if req.Sequence<=state.Sequence { writeError(w,http.StatusConflict,"stale_request"); return }
	if err:=deviceauth.Verify(state.IdentityAlgorithm,state.PublicKey,r.Method,r.URL.Path,ts,sig,body,time.Now().UTC()); err!=nil {
		writeError(w,http.StatusUnauthorized,"invalid_device_signature"); return
	}
	if err:=s.store.AdvanceDeviceSequence(r.Context(),deviceID,req.Sequence); err!=nil {
		writeError(w,http.StatusConflict,"stale_request"); return
	}

	plan,err:=s.store.PlanByID(r.Context(),strings.TrimSpace(req.PlanID))
	if err==store.ErrNotFound || (err==nil && !plan.SaleEnabled) {
		writeError(w,http.StatusNotFound,"plan_not_available"); return
	}
	if err!=nil { s.internalError(w,r,err); return }

	allowed,_,err:=s.store.AllowPaymentCreation(r.Context(),state.UserID,time.Now().UTC(),10)
	if err!=nil { s.internalError(w,r,err); return }
	if !allowed {
		_ = s.store.IncrementSecurityCounter(r.Context(),"payment_rate_limited")
		w.Header().Set("Retry-After","3600")
		writeError(w,http.StatusTooManyRequests,"payment_rate_limited")
		return
	}

	bucket:=time.Now().UTC().Truncate(15*time.Minute).Format(time.RFC3339)
	sum:=sha256.Sum256([]byte("vpnx3-payment-v2\x00"+deviceID+"\x00"+plan.ID+"\x00"+strconv.FormatBool(req.AutoRenew)+"\x00"+bucket))
	idempotenceKey:=hex.EncodeToString(sum[:])

	result,err:=s.yooKassa.CreatePayment(r.Context(),state.UserID,plan,idempotenceKey,req.AutoRenew)
	if err!=nil {
		if errors.Is(err,resilience.ErrDependencyUnavailable){
			w.Header().Set("Retry-After","30")
			writeError(w,http.StatusServiceUnavailable,"payments_temporarily_unavailable")
			return
		}
		s.internalError(w,r,err); return
	}
	if _,err:=s.billing.ApplyVerifiedEvent(r.Context(),result.Event); err!=nil {
		s.internalError(w,r,err); return
	}
	writeJSON(w,http.StatusCreated,map[string]any{
		"provider":"yookassa",
		"payment_id":result.Event.ProviderPaymentID,
		"confirmation_url":result.ConfirmationURL,
	})
}

func (s *Server) handleYooKassaWebhook(w http.ResponseWriter,r *http.Request) {
	if s.yooKassa==nil { writeError(w,http.StatusNotFound,"not_found"); return }
	raw,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,1<<20))
	if err!=nil { writeError(w,http.StatusBadRequest,"invalid_body"); return }

	var notice struct{ Event string `json:"event"` }
	if err:=json.Unmarshal(raw,&notice);err!=nil{writeError(w,http.StatusBadRequest,"invalid_webhook");return}

	if notice.Event=="refund.succeeded" {
		event,err:=s.yooKassa.VerifyAndNormalizeRefundWebhook(r.Context(),raw)
		if err!=nil{
			s.logger.Warn("YooKassa refund webhook verification failed","error",err)
			if errors.Is(err,resilience.ErrDependencyUnavailable){
				w.Header().Set("Retry-After","30")
				writeError(w,http.StatusServiceUnavailable,"provider_verification_unavailable");return
			}
			writeError(w,http.StatusBadRequest,"invalid_webhook");return
		}
		if _,err:=s.billing.ApplyVerifiedRefund(r.Context(),event);err!=nil{s.internalError(w,r,err);return}
		w.WriteHeader(http.StatusOK);return
	}

	event,err:=s.yooKassa.VerifyAndNormalizeWebhook(r.Context(),r.Header,raw)
	if err!=nil {
		s.logger.Warn("YooKassa webhook verification failed","error",err)
		if errors.Is(err,resilience.ErrDependencyUnavailable){
			w.Header().Set("Retry-After","30")
			writeError(w,http.StatusServiceUnavailable,"provider_verification_unavailable")
			return
		}
		writeError(w,http.StatusBadRequest,"invalid_webhook")
		return
	}
	if _,err:=s.billing.ApplyVerifiedEvent(r.Context(),event); err!=nil {
		s.internalError(w,r,err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
