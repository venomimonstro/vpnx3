package securityexport

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type Worker struct {
	store *store.Store
	url string
	secret []byte
	logger *slog.Logger
	client *http.Client
}

func New(s *store.Store,url,secret string,logger *slog.Logger)*Worker{
	client:=&http.Client{
		Timeout:10*time.Second,
		CheckRedirect:func(_ *http.Request,_ []*http.Request)error{
			return http.ErrUseLastResponse
		},
	}
	return &Worker{
		store:s,url:url,secret:[]byte(secret),logger:logger,client:client,
	}
}

func (w *Worker) Run(ctx context.Context){
	if w.url==""||len(w.secret)==0{return}
	ticker:=time.NewTicker(5*time.Second)
	defer ticker.Stop()
	w.runBatch(ctx)
	for{
		select{
		case <-ctx.Done():return
		case <-ticker.C:w.runBatch(ctx)
		}
	}
}

func (w *Worker) runBatch(parent context.Context){
	for i:=0;i<50;i++{
		if parent.Err()!=nil{return}
		now:=time.Now().UTC()
		ctx,cancel:=context.WithTimeout(parent,12*time.Second)
		event,err:=w.store.ClaimSecurityAuditExport(ctx,now)
		if err==store.ErrNotFound{
			cancel()
			return
		}
		if err!=nil{
			cancel()
			w.logger.Error("security audit export claim failed","error",err)
			return
		}

		body,err:=json.Marshal(event)
		if err==nil{
			err=w.deliver(ctx,event,body)
		}
		if err==nil{
			err=w.store.MarkSecurityAuditDelivered(ctx,event.AuditID,time.Now().UTC())
			if err==nil{
				w.logger.Info("security audit exported","audit_id",event.AuditID,"attempt",event.Attempt)
			}
		}else{
			dead:=event.Attempt>=10
			next:=time.Now().UTC().Add(retryDelay(event.Attempt))
			if markErr:=w.store.MarkSecurityAuditFailed(
				ctx,event.AuditID,err.Error(),next,dead,time.Now().UTC(),
			);markErr!=nil{
				w.logger.Error("security audit export failure persistence failed",
					"audit_id",event.AuditID,"error",markErr)
			}
			w.logger.Warn("security audit export failed",
				"audit_id",event.AuditID,"attempt",event.Attempt,"dead",dead,"error",err)
		}
		cancel()
	}
}

func (w *Worker) deliver(ctx context.Context,event store.SecurityAuditExport,body []byte)error{
	timestamp:=strconv.FormatInt(time.Now().UTC().Unix(),10)
	mac:=hmac.New(sha256.New,w.secret)
	_,_=mac.Write([]byte(timestamp))
	_,_=mac.Write([]byte("\n"))
	_,_=mac.Write(body)
	signature:=hex.EncodeToString(mac.Sum(nil))

	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,w.url,bytes.NewReader(body))
	if err!=nil{return err}
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("Accept","application/json")
	req.Header.Set("User-Agent","vpnx3-security-export/1")
	req.Header.Set("X-VPNX3-Audit-ID",strconv.FormatInt(event.AuditID,10))
	req.Header.Set("X-VPNX3-Timestamp",timestamp)
	req.Header.Set("X-VPNX3-Signature","sha256="+signature)

	resp,err:=w.client.Do(req)
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
