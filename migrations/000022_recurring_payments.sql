BEGIN;

CREATE TABLE IF NOT EXISTS payment_methods (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  provider_method_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ,
  UNIQUE(provider,provider_method_id)
);

CREATE INDEX IF NOT EXISTS payment_methods_user_idx
  ON payment_methods(user_id,status,created_at DESC);

ALTER TABLE payments
  ADD COLUMN IF NOT EXISTS auto_renew_requested BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE subscriptions
  ADD COLUMN IF NOT EXISTS renewal_payment_method_id UUID REFERENCES payment_methods(id),
  ADD COLUMN IF NOT EXISTS renewal_lock_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_renewal_attempt_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS renewal_failures INTEGER NOT NULL DEFAULT 0 CHECK (renewal_failures >= 0);

CREATE INDEX IF NOT EXISTS subscriptions_auto_renew_due_idx
  ON subscriptions(expires_at)
  WHERE auto_renew=true AND status IN ('active','grace');

COMMIT;
