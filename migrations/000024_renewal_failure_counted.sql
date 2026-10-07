BEGIN;
ALTER TABLE subscription_renewal_attempts
  ADD COLUMN IF NOT EXISTS failure_counted BOOLEAN NOT NULL DEFAULT false;
COMMIT;
