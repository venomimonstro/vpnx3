package store

import (
	"context"
	"fmt"
	"time"
)

type FinanceDay struct {
	Day time.Time `json:"day"`
	CapturedMinor int64 `json:"captured_minor"`
	RefundedMinor int64 `json:"refunded_minor"`
	NetMinor int64 `json:"net_minor"`
	SucceededPayments int64 `json:"succeeded_payments"`
	RefundCount int64 `json:"refund_count"`
}

type FinanceSummary struct {
	Days int `json:"days"`
	Currency string `json:"currency"`
	CapturedMinor int64 `json:"captured_minor"`
	RefundedMinor int64 `json:"refunded_minor"`
	NetMinor int64 `json:"net_minor"`
	SucceededPayments int64 `json:"succeeded_payments"`
	RefundCount int64 `json:"refund_count"`
	Daily []FinanceDay `json:"daily"`
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

	rows,err:=s.DB.Query(ctx,`
		WITH days AS (
		  SELECT generate_series(
		    current_date-($1::int-1),
		    current_date,
		    interval '1 day'
		  )::date AS day
		),
		pay AS (
		  SELECT paid_at::date AS day,
		         sum(amount_minor)::bigint AS captured_minor,
		         count(*)::bigint AS succeeded_payments
		  FROM payments
		  WHERE paid_at>=current_date-($1::int-1)
		    AND status IN ('succeeded','refunded')
		    AND currency='RUB'
		  GROUP BY paid_at::date
		),
		ref AS (
		  SELECT refunded_at::date AS day,
		         sum(amount_minor)::bigint AS refunded_minor,
		         count(*)::bigint AS refund_count
		  FROM refunds
		  WHERE refunded_at>=current_date-($1::int-1)
		    AND status='succeeded'
		    AND currency='RUB'
		  GROUP BY refunded_at::date
		)
		SELECT d.day,
		       COALESCE(p.captured_minor,0),
		       COALESCE(r.refunded_minor,0),
		       COALESCE(p.captured_minor,0)-COALESCE(r.refunded_minor,0),
		       COALESCE(p.succeeded_payments,0),
		       COALESCE(r.refund_count,0)
		FROM days d
		LEFT JOIN pay p ON p.day=d.day
		LEFT JOIN ref r ON r.day=d.day
		ORDER BY d.day
	`,days)
	if err!=nil{return FinanceSummary{},fmt.Errorf("finance daily series: %w",err)}
	defer rows.Close()
	f.Daily=make([]FinanceDay,0,days)
	for rows.Next(){
		var d FinanceDay
		if err:=rows.Scan(&d.Day,&d.CapturedMinor,&d.RefundedMinor,&d.NetMinor,&d.SucceededPayments,&d.RefundCount);err!=nil{
			return FinanceSummary{},fmt.Errorf("scan finance daily series: %w",err)
		}
		f.Daily=append(f.Daily,d)
	}
	if err:=rows.Err();err!=nil{return FinanceSummary{},fmt.Errorf("finance daily rows: %w",err)}
	return f,nil
}
