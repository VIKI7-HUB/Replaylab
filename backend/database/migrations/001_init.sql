-- 001: tables. This database is a read-optimized copy of the two services.
-- Each service's own SQLite stays the source of truth; the sync worker fills
-- these tables only through the services' HTTP APIs.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- pg_stat_statements needs shared_preload_libraries (set in postgres/postgresql.conf).
-- On a managed Postgres that doesn't allow it, carry on without it.
DO $$
BEGIN
  CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
EXCEPTION WHEN OTHERS THEN
  RAISE NOTICE 'pg_stat_statements not available: %', SQLERRM;
END $$;

-- ---------------------------------------------------------------------------
-- Replay Lab
-- ---------------------------------------------------------------------------

-- Captures are immutable and arrive with their timestamp, so (id, ts) is a
-- stable key and the table can be range-partitioned by month on ts.
CREATE TABLE rl_captures (
  id                 text        NOT NULL,
  ts                 timestamptz NOT NULL,
  request_id         text        NOT NULL DEFAULT '',
  virtual_key        text        NOT NULL DEFAULT '',
  tenant             text        NOT NULL DEFAULT '',
  provider           text        NOT NULL DEFAULT '',
  model              text        NOT NULL,
  tag                text        NOT NULL DEFAULT '',
  prompt_tokens      integer     NOT NULL DEFAULT 0,
  completion_tokens  integer     NOT NULL DEFAULT 0,
  total_tokens       integer     NOT NULL DEFAULT 0,
  latency_ms         bigint      NOT NULL DEFAULT 0,
  ttft_ms            bigint,
  cost_usd           numeric(12,6) NOT NULL DEFAULT 0,
  status_code        integer     NOT NULL DEFAULT 0,
  error_message      text,
  is_streaming       boolean     NOT NULL DEFAULT false,
  completion_state   text        NOT NULL DEFAULT 'complete'
                     CHECK (completion_state IN ('complete', 'error', 'abandoned', 'truncated')),
  usage_estimated    boolean     NOT NULL DEFAULT false,
  hash_only          boolean     NOT NULL DEFAULT false,
  messages           jsonb,
  params             jsonb,
  response_text      text,
  synced_at          timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (id, ts)
) PARTITION BY RANGE (ts);

-- Creates one partition per calendar month (UTC) covering [from_ts, to_ts].
-- Idempotent; the sync worker calls it before each batch, and the seed script
-- before bulk loads.
CREATE OR REPLACE FUNCTION rl_ensure_capture_partitions(from_ts timestamptz, to_ts timestamptz)
RETURNS integer
LANGUAGE plpgsql AS $$
DECLARE
  m       timestamptz := date_trunc('month', from_ts AT TIME ZONE 'UTC') AT TIME ZONE 'UTC';
  last_m  timestamptz := date_trunc('month', to_ts AT TIME ZONE 'UTC') AT TIME ZONE 'UTC';
  pname   text;
  created integer := 0;
BEGIN
  WHILE m <= last_m LOOP
    pname := 'rl_captures_' || to_char(m AT TIME ZONE 'UTC', 'YYYY_MM');
    IF to_regclass(pname) IS NULL THEN
      EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF rl_captures FOR VALUES FROM (%L) TO (%L)',
        pname, m, m + interval '1 month');
      created := created + 1;
    END IF;
    m := m + interval '1 month';
  END LOOP;
  RETURN created;
END $$;

-- The current month plus the next three exist from the start.
SELECT rl_ensure_capture_partitions(now() - interval '3 months', now() + interval '3 months');

CREATE TABLE rl_experiments (
  id                      text PRIMARY KEY,
  name                    text        NOT NULL DEFAULT '',
  status                  text        NOT NULL
                          CHECK (status IN ('running', 'completed', 'cancelled', 'budget_exhausted', 'failed')),
  stop_reason             text,
  original_model          text        NOT NULL DEFAULT '',
  candidate_models        text[]      NOT NULL DEFAULT '{}',
  filter_tag              text,
  streamed                boolean,
  sample_size             integer     NOT NULL DEFAULT 0,
  budget_cap_usd          numeric(12,6) NOT NULL DEFAULT 0,
  monthly_request_volume  bigint      NOT NULL DEFAULT 0,
  spent_usd               numeric(12,6) NOT NULL DEFAULT 0,
  spent_tokens            bigint      NOT NULL DEFAULT 0,
  total_replays           integer     NOT NULL DEFAULT 0,
  completed_replays       integer     NOT NULL DEFAULT 0,
  has_judge               boolean     NOT NULL DEFAULT false,
  judge_model             text,
  judge_spent_usd         numeric(12,6),
  dry_run                 boolean     NOT NULL DEFAULT false,
  created_at              timestamptz NOT NULL,
  completed_at            timestamptz,
  raw                     jsonb       NOT NULL,
  synced_at               timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rl_experiment_results (
  experiment_id             text NOT NULL REFERENCES rl_experiments(id) ON DELETE CASCADE,
  candidate_model           text NOT NULL,
  experiment_created_at     timestamptz NOT NULL,
  total_requests            integer NOT NULL DEFAULT 0,
  successful_requests       integer NOT NULL DEFAULT 0,
  failed_requests           integer NOT NULL DEFAULT 0,
  error_rate                double precision NOT NULL DEFAULT 0,
  avg_quality_score         double precision NOT NULL DEFAULT 0,
  json_validity_rate        double precision NOT NULL DEFAULT 0,
  json_samples              integer NOT NULL DEFAULT 0,
  p50_latency_ms            double precision NOT NULL DEFAULT 0,
  p95_latency_ms            double precision NOT NULL DEFAULT 0,
  original_avg_latency_ms   double precision NOT NULL DEFAULT 0,
  candidate_avg_latency_ms  double precision NOT NULL DEFAULT 0,
  latency_delta_percent     double precision NOT NULL DEFAULT 0,
  original_cost_usd         numeric(14,6) NOT NULL DEFAULT 0,
  candidate_cost_usd        numeric(14,6) NOT NULL DEFAULT 0,
  cost_savings_usd          numeric(14,6) NOT NULL DEFAULT 0,
  cost_savings_percent      double precision NOT NULL DEFAULT 0,
  verdict                   text CHECK (verdict IN ('SAFE_TO_SWITCH', 'SWITCH_WITH_CAUTION', 'DO_NOT_SWITCH')),
  confidence                double precision,
  reasoning                 text,
  projected_monthly_savings numeric(14,2) NOT NULL DEFAULT 0,
  judge_score               double precision,
  judge_wins                integer,
  judge_ties                integer,
  judge_losses              integer,
  judge_coverage            double precision,
  judge_disagreement_rate   double precision,
  PRIMARY KEY (experiment_id, candidate_model)
);

-- ---------------------------------------------------------------------------
-- Stale-Answer Detector
-- ---------------------------------------------------------------------------

CREATE TABLE sad_reports (
  report_id               text PRIMARY KEY,
  checked_at              timestamptz NOT NULL,
  question                text,
  original_answer         text NOT NULL DEFAULT '',
  overall_freshness_score double precision NOT NULL DEFAULT 0,
  verification_mode       text NOT NULL DEFAULT 'heuristic',
  current_count           integer NOT NULL DEFAULT 0,
  stale_count             integer NOT NULL DEFAULT 0,
  partially_stale_count   integer NOT NULL DEFAULT 0,
  unverifiable_count      integer NOT NULL DEFAULT 0,
  skipped_count           integer NOT NULL DEFAULT 0,
  total_claims            integer NOT NULL DEFAULT 0,
  tavily_calls            integer NOT NULL DEFAULT 0,
  tavily_calls_saved      integer NOT NULL DEFAULT 0,
  llm_calls               integer NOT NULL DEFAULT 0,
  latency_ms              double precision NOT NULL DEFAULT 0,
  corrected_answer        text,
  raw                     jsonb NOT NULL,
  synced_at               timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sad_claims (
  report_id               text NOT NULL REFERENCES sad_reports(report_id) ON DELETE CASCADE,
  claim_id                text NOT NULL,
  checked_at              timestamptz NOT NULL,
  text                    text NOT NULL DEFAULT '',
  entity                  text NOT NULL DEFAULT '',
  claim_type              text NOT NULL DEFAULT 'other',
  staleness_risk          double precision NOT NULL DEFAULT 0,
  skipped                 boolean NOT NULL DEFAULT false,
  skip_reason             text,
  verdict                 text CHECK (verdict IN ('CURRENT', 'STALE', 'PARTIALLY_STALE', 'UNVERIFIABLE')),
  freshness_score         double precision,
  confidence              double precision NOT NULL DEFAULT 0,
  current_value           text,
  explanation             text,
  evidence_count          integer NOT NULL DEFAULT 0,
  official_evidence_count integer NOT NULL DEFAULT 0,
  PRIMARY KEY (report_id, claim_id)
);

-- ---------------------------------------------------------------------------
-- Sync bookkeeping
-- ---------------------------------------------------------------------------

CREATE TABLE sync_state (
  source          text PRIMARY KEY,
  cursor          jsonb       NOT NULL DEFAULT '{}'::jsonb,
  last_success_at timestamptz,
  last_attempt_at timestamptz,
  last_error      text,
  rows_upserted   bigint      NOT NULL DEFAULT 0
);

INSERT INTO sync_state (source) VALUES
  ('replaylab.captures'), ('replaylab.experiments'), ('stale.reports')
ON CONFLICT DO NOTHING;
