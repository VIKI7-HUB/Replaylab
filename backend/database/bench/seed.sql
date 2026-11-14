-- Synthetic load for the benchmark: 1,000,000 captures over 90 days,
-- 5,000 experiments (3 candidates each) and 50,000 reports (5 claims each).
-- Every synthetic id starts with "bench-", so `npm run seed -- --clean` removes them.
-- Run with `npm run seed`, or `psql -v ON_ERROR_STOP=1 -f bench/seed.sql`.
SET synchronous_commit = off;

SELECT rl_ensure_capture_partitions(now() - interval '91 days', now() + interval '1 day');

INSERT INTO rl_captures (id, ts, request_id, virtual_key, tenant, provider, model, tag,
  prompt_tokens, completion_tokens, total_tokens, latency_ms, ttft_ms, cost_usd, status_code,
  error_message, is_streaming, completion_state, usage_estimated, hash_only, messages, params, response_text)
SELECT
  'bench-cap-' || g,
  now() - (random() * interval '90 days'),
  'bench-req-' || g,
  'vk_' || (g % 25),
  'tenant-' || (g % 7),
  (ARRAY['openai','anthropic','google'])[1 + g % 3],
  (ARRAY['gpt-4o','gpt-4o-mini','claude-3-5-sonnet','claude-3-haiku','gemini-1.5-pro','gemini-1.5-flash'])[1 + g % 6],
  (ARRAY['json_extraction','summarization','support_chat','classification','regex_generation','translation','code_review','sql_generation'])[1 + (g / 7) % 8],
  pt, ct, pt + ct,
  (80 + random() * 900)::int,
  CASE WHEN g % 3 = 0 THEN (40 + random() * 300)::int END,
  round(((pt + ct) * 0.000004)::numeric, 6),
  CASE WHEN g % 97 = 0 THEN 500 WHEN g % 211 = 0 THEN 429 ELSE 200 END,
  CASE WHEN g % 97 = 0 THEN 'upstream error' END,
  g % 3 = 0,
  CASE WHEN g % 97 = 0 THEN 'error' WHEN g % 503 = 0 THEN 'truncated' WHEN g % 701 = 0 THEN 'abandoned' ELSE 'complete' END,
  g % 3 = 0 AND g % 2 = 0,
  false,
  jsonb_build_array(jsonb_build_object('role', 'user', 'content', 'synthetic prompt ' || g)),
  '{"temperature":0.2}'::jsonb,
  'synthetic answer ' || g
FROM (SELECT g, (20 + random() * 600)::int AS pt, (10 + random() * 400)::int AS ct
      FROM generate_series(1, 1000000) g) s;

INSERT INTO rl_experiments (id, name, status, stop_reason, original_model, candidate_models, filter_tag,
  sample_size, budget_cap_usd, monthly_request_volume, spent_usd, spent_tokens, total_replays,
  completed_replays, has_judge, judge_model, dry_run, created_at, completed_at, raw)
SELECT
  'bench-exp-' || g,
  (ARRAY['nightly cost check','json extraction downgrade','support chat to haiku','summaries on flash','regex to mini'])[1 + g % 5] || ' #' || g,
  CASE WHEN g % 50 = 0 THEN 'running' WHEN g % 23 = 0 THEN 'budget_exhausted' WHEN g % 31 = 0 THEN 'cancelled'
       WHEN g % 41 = 0 THEN 'failed' ELSE 'completed' END,
  NULL,
  (ARRAY['gpt-4o','claude-3-5-sonnet','gemini-1.5-pro'])[1 + g % 3],
  ARRAY['gpt-4o-mini','claude-3-haiku','gemini-1.5-flash'],
  NULL, 100, 5, 30000, round((random() * 4)::numeric, 6), (random() * 100000)::int, 300, 300,
  g % 4 = 0, CASE WHEN g % 4 = 0 THEN 'gpt-4o' END, false,
  created, created + interval '10 minutes', '{}'::jsonb
FROM (SELECT g, now() - (random() * interval '90 days') AS created FROM generate_series(1, 5000) g) s;

INSERT INTO rl_experiment_results (experiment_id, candidate_model, experiment_created_at, total_requests,
  successful_requests, failed_requests, error_rate, avg_quality_score, json_validity_rate, json_samples,
  p50_latency_ms, p95_latency_ms, original_avg_latency_ms, candidate_avg_latency_ms, latency_delta_percent,
  original_cost_usd, candidate_cost_usd, cost_savings_usd, cost_savings_percent, verdict, confidence,
  reasoning, projected_monthly_savings)
SELECT e.id, c, e.created_at, 100, 98, 2, 0.02, q, 97, 40, 300, 900, 500, 350, -30,
  1.2, 0.3, 0.9, 75,
  CASE WHEN q > 85 THEN 'SAFE_TO_SWITCH' WHEN q > 70 THEN 'SWITCH_WITH_CAUTION' ELSE 'DO_NOT_SWITCH' END,
  0.8, 'synthetic', round((random() * 400)::numeric, 2)
FROM rl_experiments e
CROSS JOIN LATERAL unnest(e.candidate_models) c
CROSS JOIN LATERAL (SELECT 50 + random() * 50 AS q) qq
WHERE e.id LIKE 'bench-exp-%';

INSERT INTO sad_reports (report_id, checked_at, question, original_answer, overall_freshness_score,
  verification_mode, current_count, stale_count, partially_stale_count, unverifiable_count, skipped_count,
  total_claims, tavily_calls, tavily_calls_saved, llm_calls, latency_ms, corrected_answer, raw)
SELECT
  'bench-rpt-' || g, now() - (random() * interval '90 days'),
  (ARRAY['What is the latest Python release?','Who is the CEO of OpenAI?','What is the current Node LTS?',
         'What does an iPhone cost?','What is the newest Postgres version?','Which GPT model is newest?'])[1 + g % 6] || ' (' || g || ')',
  'Synthetic answer number ' || g || ' mentioning Python 3.11 and Node 18.',
  random() * 100, CASE WHEN g % 3 = 0 THEN 'heuristic' ELSE 'llm' END,
  2, 2, 0, 1, 0, 5, 5, 1, 2, 1200, NULL, '{}'::jsonb
FROM generate_series(1, 50000) g;

INSERT INTO sad_claims (report_id, claim_id, checked_at, text, entity, claim_type, staleness_risk, skipped,
  verdict, freshness_score, confidence, current_value, evidence_count, official_evidence_count)
SELECT r.report_id, 'claim-' || k, r.checked_at,
  (ARRAY['Python 3.11 is the latest release','Sam Altman is the CEO of OpenAI','Node 18 is the current LTS',
         'The iPhone starts at $799','Postgres 15 is the newest version'])[k] || ' (' || r.report_id || ')',
  (ARRAY['Python','OpenAI','Node.js','Apple','PostgreSQL'])[k],
  (ARRAY['version','leadership','version','pricing','version'])[k],
  random(), k = 5 AND random() < 0.2,
  (ARRAY['CURRENT','STALE','PARTIALLY_STALE','UNVERIFIABLE','CURRENT'])[k],
  random() * 100, random(), NULL, 3, 1
FROM sad_reports r CROSS JOIN generate_series(1, 5) k
WHERE r.report_id LIKE 'bench-rpt-%';

UPDATE sad_claims SET verdict = NULL WHERE skipped AND report_id LIKE 'bench-rpt-%';

ANALYZE;
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_rl_daily;
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_rl_verdicts;
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_sad_daily;
