package alertnotifier

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
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type Notifier struct {
	store *store.Store
	endpoint string
	secret []byte
	logger *slog.Logger
	client *http.Client
}

type envelope struct {
	SchemaVersion int `json:"schema_version"`
	NotificationID int64 `json:"notification_id"`
	IncidentID string `json:"incident_id"`
	EventType string `json:"event_type"`
	Incident json.RawMessage `json:"incident"`
	SentAt time.Time `json:"sent_at"`
}

func New(s *store.Store,endpoint,secret string,logger *slog.Logger)*Notifier{
	transport:=http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns=10
	transport.MaxIdleConnsPerHost=2
	transport.IdleConnTimeout=60*time.Second
	transport.TLSHandshakeTimeout=5*time.Second
	transport.ResponseHeaderTimeout=5*time.Second
	return &Notifier{
		store:s,endpoint:endpoint,secret:[]byte(secret),logger:logger,
		client:&http.Client{
			Transport:transport,
			Timeout:10*time.Second,
			CheckRedirect:func(_ *http.Request,_ []*http.Request)error{return http.ErrUseLastResponse},
		},
	}
}

func (n *Notifier) Run(ctx context.Context){
	if n==nil||n.endpoint==""||len(n.secret)==0{return}
	t:=time.NewTicker(15*time.Second);defer t.Stop()
	n.flush(ctx)
	for{
		select{
		case <-ctx.Done():return
		case <-t.C:n.flush(ctx)
		}
	}
}

func (n *Notifier) flush(parent context.Context){
	ctx,cancel:=context.WithTimeout(parent,30*time.Second);defer cancel()
	items,err:=n.store.ClaimIncidentNotifications(ctx,25,time.Now().UTC())
	if err!=nil{n.logger.Warn("incident notification claim failed","error",err);return}
	for _,item:=range items{
		if parent.Err()!=nil{return}
		if err:=n.deliver(parent,item);err!=nil{
			delay:=backoff(item.Attempts)
			dead:=item.Attempts>=10
			_ = n.store.MarkIncidentNotificationFailed(parent,item.ID,time.Now().UTC().Add(delay),err.Error(),dead)
			n.logger.Warn("incident notification delivery failed","notification_id",item.ID,"retry_in",delay.String(),"dead",dead,"error",err)
			// Stop this batch after the first external delivery failure. During a
			// provider outage this prevents burning one retry attempt on every
			// queued notification in the same cycle.
			return
		}
		_ = n.store.MarkIncidentNotificationDelivered(parent,item.ID,time.Now().UTC())
	}
}

func (n *Notifier) deliver(ctx context.Context,item store.IncidentNotification)error{
	body,err:=json.Marshal(envelope{
		SchemaVersion:1,NotificationID:item.ID,IncidentID:item.IncidentID,
		EventType:item.EventType,Incident:item.Payload,SentAt:time.Now().UTC(),
	})
	if err!=nil{return err}
	mac:=hmac.New(sha256.New,n.secret);_,_=mac.Write(body)
	signature:=hex.EncodeToString(mac.Sum(nil))
	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,n.endpoint,bytes.NewReader(body));if err!=nil{return err}
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("Accept","application/json")
	req.Header.Set("User-Agent","VPNX3-Incident-Notifier/1")
	req.Header.Set("X-VPNX3-Signature","sha256="+signature)
	req.Header.Set("X-VPNX3-Notification-ID",fmt.Sprintf("%d",item.ID))
	resp,err:=n.client.Do(req);if err!=nil{return err}
	defer resp.Body.Close()
	_,_=io.Copy(io.Discard,io.LimitReader(resp.Body,64<<10))
	if resp.StatusCode<200||resp.StatusCode>=300{return fmt.Errorf("notification HTTP %d",resp.StatusCode)}
	return nil
}

func backoff(attempt int)time.Duration{
	if attempt<1{attempt=1}
	shift:=attempt-1;if shift>8{shift=8}
	d:=15*time.Second*time.Duration(1<<shift)
	if d>time.Hour{d=time.Hour}
	return d
}
