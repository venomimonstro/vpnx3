package securityexport

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
	"github.com/venomimonstro/vpnx3/internal/store"
)

type Exporter struct {
	store *store.Store
	signer *signing.Signer
	endpoint string
	logger *slog.Logger
	client *http.Client
}

type signedEnvelope struct {
	KeyID string `json:"key_id"`
	Payload string `json:"payload"`
	Signature string `json:"signature"`
}

func New(s *store.Store,signer *signing.Signer,endpoint string,logger *slog.Logger)*Exporter{
	client:=&http.Client{
		Timeout:10*time.Second,
		CheckRedirect:func(_ *http.Request,_ []*http.Request)error{
			return http.ErrUseLastResponse
		},
	}
	return &Exporter{store:s,signer:signer,endpoint:endpoint,logger:logger,client:client}
}

func (e *Exporter) Run(ctx context.Context){
	if e==nil||e.signer==nil||e.endpoint==""{return}
	t:=time.NewTicker(5*time.Second)
	defer t.Stop()
	e.flush(ctx)
	for{
		select{
		case <-ctx.Done():return
		case <-t.C:e.flush(ctx)
		}
	}
}

func (e *Exporter) flush(parent context.Context){
	for i:=0;i<50;i++{
		if parent.Err()!=nil{return}
		ctx,cancel:=context.WithTimeout(parent,12*time.Second)
		event,err:=e.store.ClaimSecurityAuditExport(ctx,time.Now().UTC())
		if err==store.ErrNotFound{
			cancel()
			return
		}
		if err!=nil{
			cancel()
			e.logger.Error("security audit export claim failed","error",err)
			return
		}

		err=e.deliver(ctx,event)
		if err==nil{
			err=e.store.MarkSecurityAuditDelivered(ctx,event.AuditID,time.Now().UTC())
			if err==nil{
				e.logger.Info("security audit exported","audit_id",event.AuditID,"attempt",event.Attempt)
			}
		}else{
			dead:=event.Attempt>=10
			next:=time.Now().UTC().Add(retryDelay(event.Attempt))
			if markErr:=e.store.MarkSecurityAuditFailed(
				ctx,event.AuditID,err.Error(),next,dead,time.Now().UTC(),
			);markErr!=nil{
				e.logger.Error("security audit export failure persistence failed",
					"audit_id",event.AuditID,"error",markErr)
			}
			e.logger.Warn("security audit export failed",
				"audit_id",event.AuditID,"attempt",event.Attempt,"dead",dead,"error",err)
		}
		cancel()
	}
}

func (e *Exporter) deliver(ctx context.Context,event store.SecurityAuditExport)error{
	raw,err:=json.Marshal(event)
	if err!=nil{return err}
	env:=signedEnvelope{
		KeyID:e.signer.KeyID(),
		Payload:base64.RawURLEncoding.EncodeToString(raw),
		Signature:base64.RawURLEncoding.EncodeToString(e.signer.Sign(raw)),
	}
	body,err:=json.Marshal(env)
	if err!=nil{return err}

	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,e.endpoint,bytes.NewReader(body))
	if err!=nil{return err}
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("Accept","application/json")
	req.Header.Set("User-Agent","VPNX3-Security-Export/2")
	req.Header.Set("X-VPNX3-Audit-ID",fmt.Sprintf("%d",event.AuditID))

	resp,err:=e.client.Do(req)
	if err!=nil{return fmt.Errorf("security export request: %w",err)}
	defer resp.Body.Close()
	_,_=io.Copy(io.Discard,io.LimitReader(resp.Body,64<<10))
	if resp.StatusCode<200||resp.StatusCode>=300{
		return fmt.Errorf("security export status %d",resp.StatusCode)
	}
	return nil
}

func retryDelay(attempt int)time.Duration{
	switch{
	case attempt<=1:return time.Minute
	case attempt==2:return 5*time.Minute
	case attempt==3:return 30*time.Minute
	case attempt==4:return 2*time.Hour
	default:return 6*time.Hour
	}
}
