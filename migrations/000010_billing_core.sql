BEGIN;

CREATE TABLE IF NOT EXISTS payments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_id UUID NOT NULL REFERENCES plans(id),
  provider TEXT NOT NULL,
  provider_payment_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending','succeeded','failed','refunded','cancelled')),
  amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
  currency CHAR(3) NOT NULL,
  paid_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(provider,provider_payment_id)
);

CREATE INDEX IF NOT EXISTS payments_user_created_idx
  ON payments(user_id,created_at DESC);

CREATE TABLE IF NOT EXISTS billing_events (
  id BIGSERIAL PRIMARY KEY,
  provider TEXT NOT NULL,
  provider_event_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload_hash BYTEA NOT NULL,
  payment_id UUID REFERENCES payments(id),
  processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(provider,provider_event_id)
);

CREATE INDEX IF NOT EXISTS billing_events_processed_idx
  ON billing_events(processed_at DESC);

COMMIT;
