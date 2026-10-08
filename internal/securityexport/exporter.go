package securityexport

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

type payload struct {
	SchemaVersion int `json:"schema_version"`
	AuditID int64 `json:"audit_id"`
	ActorType string `json:"actor_type"`
	ActorID *string `json:"actor_id,omitempty"`
	Action string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID *string `json:"resource_id,omitempty"`
	RequestID *string `json:"request_id,omitempty"`
	SourceIP *string `json:"source_ip,omitempty"`
	BeforeState json.RawMessage `json:"before_state,omitempty"`
	AfterState json.RawMessage `json:"after_state,omitempty"`
	Result string `json:"result"`
	CreatedAt time.Time `json:"created_at"`
	PrevHash string `json:"prev_hash"`
	EntryHash string `json:"entry_hash"`
}

type envelope struct {
	KeyID string `json:"key_id"`
	Payload string `json:"payload"`
	Signature string `json:"signature"`
}

func New(s *store.Store,signer *signing.Signer,endpoint string,logger *slog.Logger)*Exporter{
	return &Exporter{store:s,signer:signer,endpoint:endpoint,logger:logger,client:&http.Client{Timeout:10*time.Second}}
}

func (e *Exporter) Run(ctx context.Context){
	if e==nil||e.signer==nil||e.endpoint==""{return}
	t:=time.NewTicker(15*time.Second);defer t.Stop()
	e.flush(ctx)
	for{
		select{
		case <-ctx.Done():return
		case <-t.C:e.flush(ctx)
		}
	}
}

func (e *Exporter) flush(parent context.Context){
	ctx,cancel:=context.WithTimeout(parent,30*time.Second);defer cancel()
	events,err:=e.store.ClaimSecurityExportEvents(ctx,25,time.Now().UTC())
	if err!=nil{
		e.logger.Warn("security export claim failed","error",err)
		return
	}
	for _,event:=range events{
		if parent.Err()!=nil{return}
		if err:=e.deliver(parent,event);err!=nil{
			delay:=backoff(event.Attempts)
			_ = e.store.MarkSecurityExportFailed(parent,event.AuditID,time.Now().UTC().Add(delay),err.Error())
			e.logger.Warn("security event export failed","audit_id",event.AuditID,"attempt",event.Attempts,"retry_in",delay.String(),"error",err)
			continue
		}
		if err:=e.store.MarkSecurityExportDelivered(parent,event.AuditID,time.Now().UTC());err!=nil{
			e.logger.Warn("security event delivery acknowledgement failed","audit_id",event.AuditID,"error",err)
		}
	}
}

func (e *Exporter) deliver(ctx context.Context,event store.SecurityExportEvent)error{
	p:=payload{
		SchemaVersion:1,AuditID:event.AuditID,ActorType:event.ActorType,ActorID:event.ActorID,
		Action:event.Action,ResourceType:event.ResourceType,ResourceID:event.ResourceID,
		RequestID:event.RequestID,SourceIP:event.SourceIP,BeforeState:event.BeforeState,
		AfterState:event.AfterState,Result:event.Result,CreatedAt:event.CreatedAt.UTC(),
		PrevHash:base64.RawURLEncoding.EncodeToString(event.PrevHash),
		EntryHash:base64.RawURLEncoding.EncodeToString(event.EntryHash),
	}
	raw,err:=json.Marshal(p);if err!=nil{return err}
	env:=envelope{
		KeyID:e.signer.KeyID(),
		Payload:base64.RawURLEncoding.EncodeToString(raw),
		Signature:base64.RawURLEncoding.EncodeToString(e.signer.Sign(raw)),
	}
	body,err:=json.Marshal(env);if err!=nil{return err}
	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,e.endpoint,bytes.NewReader(body));if err!=nil{return err}
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("Accept","application/json")
	req.Header.Set("User-Agent","VPNX3-Security-Export/1")
	req.Header.Set("X-VPNX3-Audit-ID",fmt.Sprintf("%d",event.AuditID))
	resp,err:=e.client.Do(req);if err!=nil{return err}
	defer resp.Body.Close()
	if resp.StatusCode<200||resp.StatusCode>=300{return fmt.Errorf("security export HTTP %d",resp.StatusCode)}
	return nil
}

func backoff(attempt int)time.Duration{
	if attempt<1{attempt=1}
	shift:=attempt-1
	if shift>8{shift=8}
	d:=15*time.Second*time.Duration(1<<shift)
	if d>time.Hour{d=time.Hour}
	return d
}
