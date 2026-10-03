BEGIN;

CREATE TABLE IF NOT EXISTS refunds (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  payment_id UUID NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  provider_refund_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('succeeded','failed','cancelled')),
  amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
  currency CHAR(3) NOT NULL,
  refunded_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(provider,provider_refund_id)
);

CREATE INDEX IF NOT EXISTS refunds_payment_idx
  ON refunds(payment_id,created_at DESC);

CREATE TABLE IF NOT EXISTS subscription_credits (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
  payment_id UUID UNIQUE REFERENCES payments(id) ON DELETE CASCADE,
  credit_seconds BIGINT NOT NULL CHECK (credit_seconds > 0),
  source TEXT NOT NULL DEFAULT 'payment',
  granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS subscription_credits_subscription_idx
  ON subscription_credits(subscription_id,granted_at);

COMMIT;
