BEGIN;

CREATE TABLE IF NOT EXISTS admin_login_throttle (
  email_normalized TEXT NOT NULL,
  source_ip INET NOT NULL,
  window_started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
  blocked_until TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(email_normalized,source_ip)
);

CREATE INDEX IF NOT EXISTS admin_login_throttle_blocked_idx
  ON admin_login_throttle(blocked_until)
  WHERE blocked_until IS NOT NULL;

COMMIT;
