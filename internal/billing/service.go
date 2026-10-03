package billing

import (
	"context"
	"net/http"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type NormalizedEvent struct {
	Provider          string
	ProviderEventID   string
	EventType         string
	ProviderPaymentID string
	UserID            string
	PlanID            string
	Status            string
	AmountMinor       int64
	Currency          string
	OccurredAt        time.Time
	RawPayload        []byte
}

type Provider interface {
	Name() string
	VerifyAndNormalizeWebhook(context.Context,http.Header,[]byte) (NormalizedEvent,error)
}

type Service struct {
	store *store.Store
}

func New(s *store.Store) *Service { return &Service{store:s} }

func (s *Service) ApplyVerifiedEvent(ctx context.Context,event NormalizedEvent) (bool,error) {
	return s.store.ApplyPaymentEvent(ctx,store.PaymentEvent{
		Provider:event.Provider,
		ProviderEventID:event.ProviderEventID,
		EventType:event.EventType,
		ProviderPaymentID:event.ProviderPaymentID,
		UserID:event.UserID,
		PlanID:event.PlanID,
		Status:event.Status,
		AmountMinor:event.AmountMinor,
		Currency:event.Currency,
		OccurredAt:event.OccurredAt,
		RawPayload:event.RawPayload,
	})
}


type NormalizedRefundEvent struct {
	Provider string
	ProviderEventID string
	ProviderRefundID string
	ProviderPaymentID string
	UserID string
	PlanID string
	Status string
	AmountMinor int64
	PaymentAmountMinor int64
	Currency string
	OccurredAt time.Time
	RawPayload []byte
}

func (s *Service) ApplyVerifiedRefund(ctx context.Context,event NormalizedRefundEvent)(bool,error){
	return s.store.ApplyRefundEvent(ctx,store.RefundEvent{
		Provider:event.Provider,ProviderEventID:event.ProviderEventID,
		ProviderRefundID:event.ProviderRefundID,ProviderPaymentID:event.ProviderPaymentID,
		UserID:event.UserID,PlanID:event.PlanID,Status:event.Status,
		AmountMinor:event.AmountMinor,PaymentAmountMinor:event.PaymentAmountMinor,
		Currency:event.Currency,OccurredAt:event.OccurredAt,RawPayload:event.RawPayload,
	})
}
