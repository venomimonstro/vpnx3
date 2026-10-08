BEGIN;

CREATE TABLE IF NOT EXISTS referral_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  code VARCHAR(16) NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS referral_redemptions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  referral_code_id UUID NOT NULL REFERENCES referral_codes(id),
  referrer_user_id UUID NOT NULL REFERENCES users(id),
  referred_user_id UUID NOT NULL UNIQUE REFERENCES users(id),
  status TEXT NOT NULL DEFAULT 'claimed'
    CHECK (status IN ('claimed','qualified','reversed','rejected')),
  reward_days INTEGER NOT NULL CHECK (reward_days > 0 AND reward_days <= 30),
  qualifying_payment_id UUID REFERENCES payments(id),
  claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  qualified_at TIMESTAMPTZ,
  reversed_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (referrer_user_id <> referred_user_id)
);

CREATE INDEX IF NOT EXISTS referral_redemptions_referrer_idx
  ON referral_redemptions(referrer_user_id,claimed_at DESC);
CREATE INDEX IF NOT EXISTS referral_redemptions_status_idx
  ON referral_redemptions(status,updated_at DESC);

CREATE TABLE IF NOT EXISTS referral_rewards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  redemption_id UUID NOT NULL REFERENCES referral_redemptions(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('referred','referrer')),
  reward_days INTEGER NOT NULL CHECK (reward_days > 0 AND reward_days <= 30),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','granted','revoked')),
  target_type TEXT CHECK (target_type IS NULL OR target_type IN ('subscription','trial')),
  subscription_id UUID REFERENCES subscriptions(id) ON DELETE SET NULL,
  target_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
  granted_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(redemption_id,role)
);

CREATE INDEX IF NOT EXISTS referral_rewards_user_status_idx
  ON referral_rewards(user_id,status,created_at);

COMMIT;
