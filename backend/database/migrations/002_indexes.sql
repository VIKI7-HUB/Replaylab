-- 002: indexes, one per read path in the Insights API. Every list endpoint pages
-- by keyset (sort key DESC, id DESC), so each index ends with the id tiebreak.

-- Captures (partitioned: each index is created on every partition).
CREATE INDEX rl_captures_ts_id_idx      ON rl_captures (ts DESC, id DESC);
CREATE INDEX rl_captures_model_ts_idx   ON rl_captures (model, ts DESC, id DESC);
CREATE INDEX rl_captures_tag_ts_idx     ON rl_captures (tag, ts DESC, id DESC);
CREATE INDEX rl_captures_vkey_ts_idx    ON rl_captures (virtual_key, ts DESC, id DESC);
CREATE INDEX rl_captures_ts_brin        ON rl_captures USING brin (ts) WITH (pages_per_range = 32);
CREATE INDEX rl_captures_problem_ts_idx ON rl_captures (ts DESC, id DESC)
  WHERE completion_state <> 'complete' OR status_code >= 400;

-- Experiments.
CREATE INDEX rl_experiments_created_idx ON rl_experiments (created_at DESC, id DESC);
CREATE INDEX rl_experiments_running_idx ON rl_experiments (id) WHERE status = 'running';
CREATE INDEX rl_experiments_status_idx  ON rl_experiments (status, created_at DESC, id DESC);
-- Text search: word / word-prefix matching on GIN full-text indexes (the
-- 'simple' config: no stemming or stop words, so model names and versions
-- match as typed). Matches are then ranked by pg_trgm word similarity. The
-- expressions here must stay identical to the ones in api/ (search.ts, q= filters).
CREATE INDEX rl_experiments_name_fts    ON rl_experiments USING gin (to_tsvector('simple', name));

-- Verdict analytics read results by experiment day.
CREATE INDEX rl_results_created_idx ON rl_experiment_results (experiment_created_at);

-- Reports.
CREATE INDEX sad_reports_checked_idx    ON sad_reports (checked_at DESC, report_id DESC);
CREATE INDEX sad_reports_score_idx      ON sad_reports (overall_freshness_score);
CREATE INDEX sad_reports_question_fts   ON sad_reports USING gin (to_tsvector('simple', coalesce(question, '')));

-- Claims.
CREATE INDEX sad_claims_checked_idx     ON sad_claims (checked_at DESC, report_id DESC, claim_id DESC);
CREATE INDEX sad_claims_type_verdict    ON sad_claims (claim_type, verdict);
CREATE INDEX sad_claims_verdict_idx     ON sad_claims (verdict, checked_at DESC);
CREATE INDEX sad_claims_entity_idx      ON sad_claims (lower(entity), checked_at DESC, report_id DESC, claim_id DESC);
CREATE INDEX sad_claims_text_fts        ON sad_claims USING gin (to_tsvector('simple', text || ' ' || entity));
