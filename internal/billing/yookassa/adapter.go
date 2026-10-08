package yookassa

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/billing"
	"github.com/venomimonstro/vpnx3/internal/store"
	"github.com/venomimonstro/vpnx3/internal/resilience"
)

const apiBase="https://api.yookassa.ru/v3"

type Adapter struct {
	shopID string
	secretKey string
	returnURL string
	client *http.Client
	breaker *resilience.CircuitBreaker
}

func New(shopID,secretKey,returnURL string) (*Adapter,error) {
	shopID=strings.TrimSpace(shopID)
	secretKey=strings.TrimSpace(secretKey)
	returnURL=strings.TrimSpace(returnURL)
	if shopID=="" || secretKey=="" || returnURL=="" {
		return nil,fmt.Errorf("YooKassa shop id, secret key and return URL are required")
	}
	if !strings.HasPrefix(returnURL,"https://") {
		return nil,fmt.Errorf("YooKassa return URL must use https")
	}
	return &Adapter{
		shopID:shopID,
		secretKey:secretKey,
		returnURL:returnURL,
		client:&http.Client{Timeout:12*time.Second},
		breaker:resilience.NewCircuitBreaker(5,30*time.Second),
	},nil
}

func (a *Adapter) Name() string { return "yookassa" }
func (a *Adapter) CircuitSnapshot() resilience.Snapshot { return a.breaker.Snapshot() }

func (a *Adapter) do(req *http.Request)(*http.Response,error){
	now:=time.Now().UTC()
	if !a.breaker.Allow(now){
		return nil,resilience.ErrDependencyUnavailable
	}
	resp,err:=a.client.Do(req)
	if err!=nil{
		a.breaker.Failure(now)
		return nil,fmt.Errorf("%w: YooKassa transport: %v",resilience.ErrDependencyUnavailable,err)
	}
	if resp.StatusCode==http.StatusTooManyRequests||resp.StatusCode>=500{
		a.breaker.Failure(now)
	}else{
		a.breaker.Success()
	}
	return resp,nil
}

type CreateResult struct {
	Event billing.NormalizedEvent
	ConfirmationURL string
}

func (a *Adapter) CreatePayment(ctx context.Context,userID string,plan store.Plan,idempotence string,autoRenew bool) (CreateResult,error) {
	idempotence=strings.TrimSpace(idempotence)
	if idempotence=="" {
		var err error
		idempotence,err=randomID()
		if err!=nil { return CreateResult{},err }
	}

	payload:=map[string]any{
		"amount":map[string]string{
			"value":minorToDecimal(plan.PriceMinor),
			"currency":plan.Currency,
		},
		"capture":true,
		"confirmation":map[string]string{
			"type":"redirect",
			"return_url":a.returnURL,
		},
		"description":fmt.Sprintf("VPNX3 %s",plan.Name),
		"metadata":map[string]string{
			"user_id":userID,
			"plan_id":plan.ID,
			"auto_renew":strconv.FormatBool(autoRenew),
		},
	}
	if autoRenew { payload["save_payment_method"]=true }
	raw,err:=json.Marshal(payload)
	if err!=nil { return CreateResult{},err }

	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,apiBase+"/payments",bytes.NewReader(raw))
	if err!=nil { return CreateResult{},err }
	req.SetBasicAuth(a.shopID,a.secretKey)
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("Accept","application/json")
	req.Header.Set("Idempotence-Key",idempotence)

	resp,err:=a.do(req)
	if err!=nil { return CreateResult{},fmt.Errorf("create YooKassa payment: %w",err) }
	defer resp.Body.Close()
	responseRaw,err:=io.ReadAll(io.LimitReader(resp.Body,1<<20))
	if err!=nil { return CreateResult{},err }
	if resp.StatusCode<200 || resp.StatusCode>=300 {
		if resp.StatusCode==http.StatusTooManyRequests||resp.StatusCode>=500{
			return CreateResult{},fmt.Errorf("%w: YooKassa create payment status %d",resilience.ErrDependencyUnavailable,resp.StatusCode)
		}
		return CreateResult{},fmt.Errorf("YooKassa create payment status %d",resp.StatusCode)
	}

	payment,err:=parsePayment(responseRaw)
	if err!=nil { return CreateResult{},err }
	if payment.Metadata.UserID!=userID || payment.Metadata.PlanID!=plan.ID {
		return CreateResult{},fmt.Errorf("YooKassa returned unexpected payment metadata")
	}
	amountMinor,err:=decimalToMinor(payment.Amount.Value)
	if err!=nil { return CreateResult{},err }
	if amountMinor!=plan.PriceMinor || payment.Amount.Currency!=plan.Currency {
		return CreateResult{},fmt.Errorf("YooKassa returned unexpected payment amount")
	}
	if payment.Confirmation.ConfirmationURL=="" {
		return CreateResult{},fmt.Errorf("YooKassa did not return confirmation URL")
	}

	status:=normalizePaymentStatus(payment.Status)
	event:=billing.NormalizedEvent{
		Provider:a.Name(),
		ProviderEventID:"create:"+payment.ID,
		EventType:"payment.created",
		ProviderPaymentID:payment.ID,
		UserID:userID,
		PlanID:plan.ID,
		Status:status,
		AmountMinor:amountMinor,
		Currency:payment.Amount.Currency,
		OccurredAt:payment.CreatedAt,
		RawPayload:responseRaw,
		PaymentMethodID:payment.PaymentMethod.ID,
		PaymentMethodSaved:payment.PaymentMethod.Saved,
		AutoRenewRequested:strings.EqualFold(payment.Metadata.AutoRenew,"true"),
	}
	return CreateResult{Event:event,ConfirmationURL:payment.Confirmation.ConfirmationURL},nil
}


func (a *Adapter) CreateRecurringPayment(ctx context.Context,userID string,plan store.Plan,paymentMethodID,attemptID,idempotence string) (CreateResult,error) {
	paymentMethodID=strings.TrimSpace(paymentMethodID)
	attemptID=strings.TrimSpace(attemptID)
	idempotence=strings.TrimSpace(idempotence)
	if paymentMethodID==""||attemptID==""||idempotence==""{return CreateResult{},fmt.Errorf("invalid recurring payment request")}
	payload:=map[string]any{
		"amount":map[string]string{"value":minorToDecimal(plan.PriceMinor),"currency":plan.Currency},
		"capture":true,
		"payment_method_id":paymentMethodID,
		"description":fmt.Sprintf("VPNX3 автопродление %s",plan.Name),
		"metadata":map[string]string{
			"user_id":userID,"plan_id":plan.ID,"auto_renew":"true","renewal_attempt_id":attemptID,
		},
	}
	raw,err:=json.Marshal(payload);if err!=nil{return CreateResult{},err}
	req,err:=http.NewRequestWithContext(ctx,http.MethodPost,apiBase+"/payments",bytes.NewReader(raw));if err!=nil{return CreateResult{},err}
	req.SetBasicAuth(a.shopID,a.secretKey);req.Header.Set("Content-Type","application/json");req.Header.Set("Accept","application/json");req.Header.Set("Idempotence-Key",idempotence)
	resp,err:=a.do(req);if err!=nil{return CreateResult{},fmt.Errorf("create YooKassa recurring payment: %w",err)}
	defer resp.Body.Close()
	responseRaw,err:=io.ReadAll(io.LimitReader(resp.Body,1<<20));if err!=nil{return CreateResult{},err}
	if resp.StatusCode<200||resp.StatusCode>=300{
		if resp.StatusCode==http.StatusTooManyRequests||resp.StatusCode>=500{
			return CreateResult{},fmt.Errorf("%w: YooKassa recurring payment status %d",resilience.ErrDependencyUnavailable,resp.StatusCode)
		}
		return CreateResult{},fmt.Errorf("YooKassa recurring payment status %d",resp.StatusCode)
	}
	payment,err:=parsePayment(responseRaw);if err!=nil{return CreateResult{},err}
	if payment.Metadata.UserID!=userID||payment.Metadata.PlanID!=plan.ID||payment.Metadata.RenewalAttemptID!=attemptID{return CreateResult{},fmt.Errorf("unexpected recurring payment metadata")}
	amountMinor,err:=decimalToMinor(payment.Amount.Value);if err!=nil{return CreateResult{},err}
	if amountMinor!=plan.PriceMinor||payment.Amount.Currency!=plan.Currency{return CreateResult{},fmt.Errorf("unexpected recurring payment amount")}
	event:=billing.NormalizedEvent{
		Provider:a.Name(),ProviderEventID:"create:"+payment.ID,EventType:"payment.created",
		ProviderPaymentID:payment.ID,UserID:userID,PlanID:plan.ID,Status:normalizePaymentStatus(payment.Status),
		AmountMinor:amountMinor,Currency:payment.Amount.Currency,OccurredAt:payment.CreatedAt,RawPayload:responseRaw,
		PaymentMethodID:payment.PaymentMethod.ID,PaymentMethodSaved:payment.PaymentMethod.Saved,
		AutoRenewRequested:true,RenewalAttemptID:attemptID,
	}
	return CreateResult{Event:event},nil
}

func (a *Adapter) FetchPaymentEvent(ctx context.Context,paymentID string)(billing.NormalizedEvent,error){
	payment,raw,err:=a.fetchPayment(ctx,strings.TrimSpace(paymentID))
	if err!=nil{return billing.NormalizedEvent{},err}
	if payment.Metadata.UserID==""||payment.Metadata.PlanID==""{
		return billing.NormalizedEvent{},fmt.Errorf("verified payment metadata is incomplete")
	}
	amountMinor,err:=decimalToMinor(payment.Amount.Value);if err!=nil{return billing.NormalizedEvent{},err}
	occurred:=payment.CreatedAt
	if payment.CapturedAt!=nil{occurred=*payment.CapturedAt}
	return billing.NormalizedEvent{
		Provider:a.Name(),ProviderEventID:"reconcile:"+payment.ID+":"+payment.Status,
		EventType:"payment.reconcile",ProviderPaymentID:payment.ID,
		UserID:payment.Metadata.UserID,PlanID:payment.Metadata.PlanID,
		Status:normalizePaymentStatus(payment.Status),AmountMinor:amountMinor,
		Currency:payment.Amount.Currency,OccurredAt:occurred,RawPayload:raw,
		PaymentMethodID:payment.PaymentMethod.ID,PaymentMethodSaved:payment.PaymentMethod.Saved,
		AutoRenewRequested:strings.EqualFold(payment.Metadata.AutoRenew,"true"),
		RenewalAttemptID:payment.Metadata.RenewalAttemptID,
	},nil
}

func (a *Adapter) VerifyAndNormalizeWebhook(ctx context.Context,_ http.Header,raw []byte) (billing.NormalizedEvent,error) {
	var notice struct {
		Type string `json:"type"`
		Event string `json:"event"`
		Object struct {
			ID string `json:"id"`
		} `json:"object"`
	}
	if err:=json.Unmarshal(raw,&notice); err!=nil {
		return billing.NormalizedEvent{},fmt.Errorf("decode YooKassa webhook: %w",err)
	}
	if notice.Type!="notification" || notice.Object.ID=="" {
		return billing.NormalizedEvent{},fmt.Errorf("invalid YooKassa notification")
	}
	switch notice.Event {
	case "payment.succeeded","payment.canceled","payment.waiting_for_capture":
	default:
		return billing.NormalizedEvent{},fmt.Errorf("unsupported YooKassa event %q",notice.Event)
	}

	payment,verifiedRaw,err:=a.fetchPayment(ctx,notice.Object.ID)
	if err!=nil { return billing.NormalizedEvent{},err }
	expectedEvent:="payment."+payment.Status
	if payment.Status=="canceled" { expectedEvent="payment.canceled" }
	if notice.Event!=expectedEvent {
		return billing.NormalizedEvent{},fmt.Errorf("webhook status does not match verified payment status")
	}
	if payment.Metadata.UserID=="" || payment.Metadata.PlanID=="" {
		return billing.NormalizedEvent{},fmt.Errorf("verified payment metadata is incomplete")
	}
	amountMinor,err:=decimalToMinor(payment.Amount.Value)
	if err!=nil { return billing.NormalizedEvent{},err }

	occurred:=payment.CreatedAt
	if payment.CapturedAt!=nil { occurred=*payment.CapturedAt }

	return billing.NormalizedEvent{
		Provider:a.Name(),
		ProviderEventID:notice.Event+":"+payment.ID,
		EventType:notice.Event,
		ProviderPaymentID:payment.ID,
		UserID:payment.Metadata.UserID,
		PlanID:payment.Metadata.PlanID,
		Status:normalizePaymentStatus(payment.Status),
		AmountMinor:amountMinor,
		Currency:payment.Amount.Currency,
		OccurredAt:occurred,
		RawPayload:append(append([]byte{},raw...),verifiedRaw...),
		PaymentMethodID:payment.PaymentMethod.ID,
		PaymentMethodSaved:payment.PaymentMethod.Saved,
		AutoRenewRequested:strings.EqualFold(payment.Metadata.AutoRenew,"true"),
		RenewalAttemptID:payment.Metadata.RenewalAttemptID,
	},nil
}

type refundObject struct {
	ID string `json:"id"`
	Status string `json:"status"`
	PaymentID string `json:"payment_id"`
	Amount struct {
		Value string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

func (a *Adapter) VerifyAndNormalizeRefundWebhook(ctx context.Context,raw []byte)(billing.NormalizedRefundEvent,error){
	var notice struct{
		Type string `json:"type"`
		Event string `json:"event"`
		Object struct{ID string `json:"id"`} `json:"object"`
	}
	if err:=json.Unmarshal(raw,&notice);err!=nil{return billing.NormalizedRefundEvent{},err}
	if notice.Type!="notification"||notice.Event!="refund.succeeded"||notice.Object.ID==""{
		return billing.NormalizedRefundEvent{},fmt.Errorf("invalid YooKassa refund notification")
	}
	refund,refundRaw,err:=a.fetchRefund(ctx,notice.Object.ID)
	if err!=nil{return billing.NormalizedRefundEvent{},err}
	if refund.Status!="succeeded"||refund.PaymentID==""{
		return billing.NormalizedRefundEvent{},fmt.Errorf("verified refund is not succeeded")
	}
	payment,paymentRaw,err:=a.fetchPayment(ctx,refund.PaymentID)
	if err!=nil{return billing.NormalizedRefundEvent{},err}
	if payment.Metadata.UserID==""||payment.Metadata.PlanID==""{
		return billing.NormalizedRefundEvent{},fmt.Errorf("refund payment metadata is incomplete")
	}
	refundMinor,err:=decimalToMinor(refund.Amount.Value);if err!=nil{return billing.NormalizedRefundEvent{},err}
	paymentMinor,err:=decimalToMinor(payment.Amount.Value);if err!=nil{return billing.NormalizedRefundEvent{},err}
	if refund.Amount.Currency!=payment.Amount.Currency{
		return billing.NormalizedRefundEvent{},fmt.Errorf("refund currency differs from payment")
	}
	payload:=append(append(append([]byte{},raw...),refundRaw...),paymentRaw...)
	return billing.NormalizedRefundEvent{
		Provider:a.Name(),
		ProviderEventID:"refund.succeeded:"+refund.ID,
		ProviderRefundID:refund.ID,
		ProviderPaymentID:payment.ID,
		UserID:payment.Metadata.UserID,
		PlanID:payment.Metadata.PlanID,
		Status:"succeeded",
		AmountMinor:refundMinor,
		PaymentAmountMinor:paymentMinor,
		Currency:refund.Amount.Currency,
		OccurredAt:refund.CreatedAt,
		RawPayload:payload,
	},nil
}

func (a *Adapter) fetchRefund(ctx context.Context,id string)(refundObject,[]byte,error){
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,apiBase+"/refunds/"+id,nil)
	if err!=nil{return refundObject{},nil,err}
	req.SetBasicAuth(a.shopID,a.secretKey)
	req.Header.Set("Accept","application/json")
	resp,err:=a.do(req)
	if err!=nil{return refundObject{},nil,fmt.Errorf("verify YooKassa refund: %w",err)}
	defer resp.Body.Close()
	raw,err:=io.ReadAll(io.LimitReader(resp.Body,1<<20));if err!=nil{return refundObject{},nil,err}
	if resp.StatusCode!=http.StatusOK{
		if resp.StatusCode==http.StatusTooManyRequests||resp.StatusCode>=500{
			return refundObject{},nil,fmt.Errorf("%w: YooKassa refund status %d",resilience.ErrDependencyUnavailable,resp.StatusCode)
		}
		return refundObject{},nil,fmt.Errorf("verify YooKassa refund status %d",resp.StatusCode)
	}
	var refund refundObject
	if err:=json.Unmarshal(raw,&refund);err!=nil{return refundObject{},nil,err}
	if refund.ID==""||refund.Status==""||refund.PaymentID==""||refund.Amount.Value==""{
		return refundObject{},nil,fmt.Errorf("incomplete YooKassa refund object")
	}
	return refund,raw,nil
}

type paymentObject struct {
	ID string `json:"id"`
	Status string `json:"status"`
	Amount struct {
		Value string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	Confirmation struct {
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
	Metadata struct {
		UserID string `json:"user_id"`
		PlanID string `json:"plan_id"`
		AutoRenew string `json:"auto_renew"`
		RenewalAttemptID string `json:"renewal_attempt_id"`
	} `json:"metadata"`
	PaymentMethod struct {
		ID string `json:"id"`
		Saved bool `json:"saved"`
	} `json:"payment_method"`
	CreatedAt time.Time `json:"created_at"`
	CapturedAt *time.Time `json:"captured_at"`
}

func (a *Adapter) fetchPayment(ctx context.Context,id string) (paymentObject,[]byte,error) {
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,apiBase+"/payments/"+id,nil)
	if err!=nil { return paymentObject{},nil,err }
	req.SetBasicAuth(a.shopID,a.secretKey)
	req.Header.Set("Accept","application/json")
	resp,err:=a.do(req)
	if err!=nil { return paymentObject{},nil,fmt.Errorf("verify YooKassa payment: %w",err) }
	defer resp.Body.Close()
	raw,err:=io.ReadAll(io.LimitReader(resp.Body,1<<20))
	if err!=nil { return paymentObject{},nil,err }
	if resp.StatusCode!=http.StatusOK {
		if resp.StatusCode==http.StatusTooManyRequests||resp.StatusCode>=500{
			return paymentObject{},nil,fmt.Errorf("%w: YooKassa payment status %d",resilience.ErrDependencyUnavailable,resp.StatusCode)
		}
		return paymentObject{},nil,fmt.Errorf("verify YooKassa payment status %d",resp.StatusCode)
	}
	payment,err:=parsePayment(raw)
	return payment,raw,err
}

func parsePayment(raw []byte) (paymentObject,error) {
	var payment paymentObject
	if err:=json.Unmarshal(raw,&payment); err!=nil { return paymentObject{},err }
	if payment.ID=="" || payment.Status=="" || payment.Amount.Value=="" || len(payment.Amount.Currency)!=3 {
		return paymentObject{},fmt.Errorf("incomplete YooKassa payment object")
	}
	return payment,nil
}

func normalizePaymentStatus(status string) string {
	switch status {
	case "succeeded":
		return "succeeded"
	case "canceled":
		return "cancelled"
	default:
		return "pending"
	}
}

func minorToDecimal(value int64) string {
	return fmt.Sprintf("%d.%02d",value/100,value%100)
}

func decimalToMinor(value string) (int64,error) {
	parts:=strings.Split(value,".")
	if len(parts)>2 || len(parts)==0 { return 0,fmt.Errorf("invalid money value") }
	whole,err:=strconv.ParseInt(parts[0],10,64)
	if err!=nil || whole<0 { return 0,fmt.Errorf("invalid money value") }
	fraction:="00"
	if len(parts)==2 {
		if len(parts[1])==1 { fraction=parts[1]+"0" } else { fraction=parts[1] }
	}
	if len(fraction)!=2 { return 0,fmt.Errorf("money must have at most 2 decimal places") }
	cents,err:=strconv.ParseInt(fraction,10,64)
	if err!=nil || cents<0 || cents>99 { return 0,fmt.Errorf("invalid money fraction") }
	return whole*100+cents,nil
}

func randomID() (string,error) {
	raw:=make([]byte,16)
	if _,err:=rand.Read(raw); err!=nil { return "",err }
	return hex.EncodeToString(raw),nil
}

var _ = base64.RawURLEncoding
