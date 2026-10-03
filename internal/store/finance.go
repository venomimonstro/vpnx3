package store

import (
	"context"
	"fmt"
)

type FinanceSummary struct {
	Days int `json:"days"`
	Currency string `json:"currency"`
	CapturedMinor int64 `json:"captured_minor"`
	RefundedMinor int64 `json:"refunded_minor"`
	NetMinor int64 `json:"net_minor"`
	SucceededPayments int64 `json:"succeeded_payments"`
	RefundCount int64 `json:"refund_count"`
}

func (s *Store) FinanceSummary(ctx context.Context,days int)(FinanceSummary,error){
	if days<=0||days>365{days=30}
	var f FinanceSummary
	f.Days=days;f.Currency="RUB"
	err:=s.DB.QueryRow(ctx,`
		SELECT
		  COALESCE((SELECT sum(amount_minor) FROM payments
		    WHERE paid_at>=now()-($1::int * interval '1 day') AND status IN ('succeeded','refunded') AND currency='RUB'),0),
		  COALESCE((SELECT sum(r.amount_minor) FROM refunds r
		    WHERE r.refunded_at>=now()-($1::int * interval '1 day') AND r.status='succeeded' AND r.currency='RUB'),0),
		  COALESCE((SELECT count(*) FROM payments
		    WHERE paid_at>=now()-($1::int * interval '1 day') AND status IN ('succeeded','refunded') AND currency='RUB'),0),
		  COALESCE((SELECT count(*) FROM refunds
		    WHERE refunded_at>=now()-($1::int * interval '1 day') AND status='succeeded' AND currency='RUB'),0)
	`,days).Scan(&f.CapturedMinor,&f.RefundedMinor,&f.SucceededPayments,&f.RefundCount)
	if err!=nil{return FinanceSummary{},fmt.Errorf("finance summary: %w",err)}
	f.NetMinor=f.CapturedMinor-f.RefundedMinor
	return f,nil
}
