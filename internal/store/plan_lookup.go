package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Store) PlanByID(ctx context.Context,id string) (Plan,error) {
	var p Plan
	err:=s.DB.QueryRow(ctx,`
		SELECT id::text,code,version,name,price_minor,currency,billing_period_days,
		       device_limit,trial_days,grace_days,sale_enabled,created_at
		FROM plans WHERE id=$1
	`,id).Scan(&p.ID,&p.Code,&p.Version,&p.Name,&p.PriceMinor,&p.Currency,
		&p.BillingPeriodDays,&p.DeviceLimit,&p.TrialDays,&p.GraceDays,&p.SaleEnabled,&p.CreatedAt)
	if errors.Is(err,pgx.ErrNoRows) { return Plan{},ErrNotFound }
	return p,err
}
