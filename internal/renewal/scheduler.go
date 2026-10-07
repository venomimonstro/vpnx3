package renewal

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/venomimonstro/vpnx3/internal/billing"
	"github.com/venomimonstro/vpnx3/internal/billing/yookassa"
	"github.com/venomimonstro/vpnx3/internal/store"
)

type Scheduler struct {
	store *store.Store
	billing *billing.Service
	provider *yookassa.Adapter
	logger *slog.Logger
	interval time.Duration
}

func New(s *store.Store,b *billing.Service,p *yookassa.Adapter,logger *slog.Logger,interval time.Duration)*Scheduler{
	if interval<time.Minute{interval=5*time.Minute}
	return &Scheduler{store:s,billing:b,provider:p,logger:logger,interval:interval}
}

func (s *Scheduler) Run(ctx context.Context){
	if s.provider==nil{return}
	t:=time.NewTicker(s.interval);defer t.Stop()
	s.runBatch(ctx)
	for{
		select{
		case <-ctx.Done():return
		case <-t.C:s.runBatch(ctx)
		}
	}
}

func (s *Scheduler) runBatch(parent context.Context){
	recoveryCtx,recoveryCancel:=context.WithTimeout(parent,30*time.Second)
	stale,err:=s.store.StaleRenewalAttempts(recoveryCtx,time.Now().UTC(),20)
	if err!=nil{
		s.logger.Warn("stale renewal lookup failed","error",err)
	}else{
		for _,attempt:=range stale{
			if attempt.ProviderPaymentID==nil||*attempt.ProviderPaymentID==""{
				_ = s.store.MarkRenewalFailure(recoveryCtx,attempt.ID,"renewal creation timeout")
				continue
			}
			event,fetchErr:=s.provider.FetchPaymentEvent(recoveryCtx,*attempt.ProviderPaymentID)
			if fetchErr!=nil{
				s.logger.Warn("stale renewal provider reconciliation failed","attempt_id",attempt.ID,"error",fetchErr)
				continue
			}
			if _,applyErr:=s.billing.ApplyVerifiedEvent(recoveryCtx,event);applyErr!=nil{
				s.logger.Warn("stale renewal apply failed","attempt_id",attempt.ID,"error",applyErr)
				continue
			}
			if event.Status=="failed"||event.Status=="cancelled"{
				_ = s.store.MarkRenewalFailure(recoveryCtx,attempt.ID,"provider confirmed renewal failure")
			}
		}
	}
	recoveryCancel()

	for i:=0;i<20;i++{
		if parent.Err()!=nil{return}
		ctx,cancel:=context.WithTimeout(parent,20*time.Second)
		attempt,err:=s.store.ClaimDueRenewal(ctx,time.Now().UTC())
		if errors.Is(err,store.ErrNotFound){cancel();return}
		if err!=nil{
			cancel()
			s.logger.Error("renewal claim failed","error",err)
			return
		}
		result,createErr:=s.provider.CreateRecurringPayment(
			ctx,attempt.UserID,attempt.Plan,attempt.PaymentMethodID,attempt.ID,attempt.IdempotenceKey,
		)
		if createErr!=nil{
			_ = s.store.MarkRenewalFailure(ctx,attempt.ID,createErr.Error())
			cancel()
			s.logger.Warn("automatic renewal payment creation failed","subscription_id",attempt.SubscriptionID,"attempt",attempt.AttemptNo,"error",createErr)
			continue
		}
		if _,err:=s.billing.ApplyVerifiedEvent(ctx,result.Event);err!=nil{
			_ = s.store.MarkRenewalFailure(ctx,attempt.ID,err.Error())
			cancel()
			s.logger.Error("automatic renewal payment apply failed","subscription_id",attempt.SubscriptionID,"attempt",attempt.AttemptNo,"error",err)
			continue
		}
		cancel()
		s.logger.Info("automatic renewal payment submitted","subscription_id",attempt.SubscriptionID,"attempt",attempt.AttemptNo)
	}
}
