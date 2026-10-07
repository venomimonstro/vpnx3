BEGIN;

CREATE TABLE IF NOT EXISTS subscription_renewal_attempts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
  cycle_expires_at TIMESTAMPTZ NOT NULL,
  attempt_no INTEGER NOT NULL CHECK (attempt_no BETWEEN 1 AND 3),
  provider TEXT NOT NULL,
  provider_payment_id TEXT,
  status TEXT NOT NULL CHECK (status IN ('claimed','pending','succeeded','failed')),
  idempotence_key TEXT NOT NULL UNIQUE,
  error_summary TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(subscription_id,cycle_expires_at,attempt_no)
);

CREATE UNIQUE INDEX IF NOT EXISTS subscription_renewal_provider_payment_uq
  ON subscription_renewal_attempts(provider,provider_payment_id)
  WHERE provider_payment_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS subscription_renewal_attempts_due_idx
  ON subscription_renewal_attempts(subscription_id,cycle_expires_at,updated_at DESC);

COMMIT;
