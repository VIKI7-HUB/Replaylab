-- 003: dashboard rollups. The sync worker refreshes these CONCURRENTLY after
-- each cycle that wrote rows, which needs a unique index on each view.
-- Days are UTC calendar days.

CREATE MATERIALIZED VIEW mv_rl_daily AS
SELECT
  (ts AT TIME ZONE 'UTC')::date                                        AS day,
  model,
  count(*)::bigint                                                     AS captures,
  sum(total_tokens)::bigint                                            AS tokens,
  sum(cost_usd)::numeric(14,6)                                         AS cost_usd,
  avg(latency_ms)::double precision                                    AS avg_latency_ms,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms)::double precision AS p95_latency_ms,
  (count(*) FILTER (WHERE status_code >= 400 OR completion_state = 'error'))::double precision
    / count(*)                                                         AS error_rate,
  (count(*) FILTER (WHERE is_streaming))::double precision / count(*)  AS streamed_share
FROM rl_captures
GROUP BY 1, 2
WITH DATA;

CREATE UNIQUE INDEX mv_rl_daily_key ON mv_rl_daily (day, model);

CREATE MATERIALIZED VIEW mv_rl_verdicts AS
SELECT
  (experiment_created_at AT TIME ZONE 'UTC')::date                     AS day,
  candidate_model,
  count(*) FILTER (WHERE verdict = 'SAFE_TO_SWITCH')::bigint           AS safe,
  count(*) FILTER (WHERE verdict = 'SWITCH_WITH_CAUTION')::bigint      AS caution,
  count(*) FILTER (WHERE verdict = 'DO_NOT_SWITCH')::bigint            AS do_not,
  coalesce(sum(projected_monthly_savings), 0)::numeric(16,2)           AS projected_monthly_savings_usd
FROM rl_experiment_results
GROUP BY 1, 2
WITH DATA;

CREATE UNIQUE INDEX mv_rl_verdicts_key ON mv_rl_verdicts (day, candidate_model);

CREATE MATERIALIZED VIEW mv_sad_daily AS
SELECT
  (checked_at AT TIME ZONE 'UTC')::date                                AS day,
  claim_type,
  count(*)::bigint                                                     AS claims,
  count(*) FILTER (WHERE verdict = 'STALE')::bigint                    AS stale,
  count(*) FILTER (WHERE verdict = 'PARTIALLY_STALE')::bigint          AS partially_stale,
  count(*) FILTER (WHERE verdict = 'CURRENT')::bigint                  AS current,
  count(*) FILTER (WHERE verdict = 'UNVERIFIABLE')::bigint             AS unverifiable,
  count(*) FILTER (WHERE skipped)::bigint                              AS skipped,
  avg(freshness_score)::double precision                               AS avg_freshness
FROM sad_claims
GROUP BY 1, 2
WITH DATA;

CREATE UNIQUE INDEX mv_sad_daily_key ON mv_sad_daily (day, claim_type);
