BEGIN;

CREATE TABLE IF NOT EXISTS security_counters_daily (
  day DATE NOT NULL,
  counter TEXT NOT NULL,
  count BIGINT NOT NULL DEFAULT 0 CHECK (count >= 0),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(day,counter)
);

CREATE INDEX IF NOT EXISTS security_counters_daily_day_idx
  ON security_counters_daily(day DESC);

COMMIT;
