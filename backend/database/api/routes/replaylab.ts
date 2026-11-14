import type { FastifyInstance } from 'fastify';
import { z } from 'zod';
import type { Sql } from '../../lib/db.js';
import { decodeCursor, iso, isoDate, page, PageQuery, parseQuery } from '../paging.js';
import { tsQuery } from '../text.js';

const CaptureQuery = PageQuery.extend({
  model: z.string().max(200).optional(),
  tag: z.string().max(200).optional(),
  virtual_key: z.string().max(200).optional(),
  streamed: z.enum(['true', 'false', 'any']).optional(),
  // "problem" = anything not complete or with an HTTP error (served by a partial index).
  state: z.enum(['complete', 'error', 'abandoned', 'truncated', 'problem']).optional(),
  from: isoDate.optional(),
  to: isoDate.optional(),
});

const ExperimentQuery = PageQuery.extend({
  status: z.enum(['running', 'completed', 'cancelled', 'budget_exhausted', 'failed']).optional(),
  q: z.string().max(200).optional(),
});

export function replaylabRoutes(app: FastifyInstance, sql: Sql) {
  app.get('/api/replaylab/captures', async (req) => {
    const q = parseQuery(CaptureQuery, req.query);
    const after = decodeCursor(q.cursor, 2);
    const rows = await sql<CaptureListRow[]>`
      SELECT id, ts, ts::text AS k, request_id, virtual_key, tenant, provider, model, tag,
             prompt_tokens, completion_tokens, total_tokens, latency_ms::float8 AS latency_ms,
             ttft_ms::float8 AS ttft_ms, cost_usd::float8 AS cost_usd, status_code, error_message,
             is_streaming, completion_state, usage_estimated, hash_only
      FROM rl_captures
      WHERE true
        ${q.model ? sql`AND model = ${q.model}` : sql``}
        ${q.tag ? sql`AND tag = ${q.tag}` : sql``}
        ${q.virtual_key ? sql`AND virtual_key = ${q.virtual_key}` : sql``}
        ${q.streamed === 'true' ? sql`AND is_streaming` : q.streamed === 'false' ? sql`AND NOT is_streaming` : sql``}
        ${q.state === 'problem'
          ? sql`AND (completion_state <> 'complete' OR status_code >= 400)`
          : q.state ? sql`AND completion_state = ${q.state}` : sql``}
        ${q.from ? sql`AND ts >= ${q.from}` : sql``}
        ${q.to ? sql`AND ts < ${q.to}` : sql``}
        ${after ? sql`AND (ts, id) < (${String(after[0])}::timestamptz, ${String(after[1])})` : sql``}
      ORDER BY ts DESC, id DESC
      LIMIT ${q.limit + 1}`;
    const p = page(rows, q.limit, (r) => [r.k, r.id]);
    return {
      items: p.items.map(({ k: _k, ts, ...r }) => ({ ...r, timestamp: iso(ts) })),
      next_cursor: p.next_cursor,
    };
  });

  app.get('/api/replaylab/experiments', async (req) => {
    const q = parseQuery(ExperimentQuery, req.query);
    const after = decodeCursor(q.cursor, 2);
    const rows = await sql<ExperimentListRow[]>`
      SELECT e.id, e.name, e.status, e.stop_reason, e.original_model, e.candidate_models, e.filter_tag,
             e.sample_size, e.budget_cap_usd::float8 AS budget_cap_usd, e.spent_usd::float8 AS spent_usd,
             e.spent_tokens::float8 AS spent_tokens, e.total_replays, e.completed_replays, e.has_judge,
             e.judge_model, e.dry_run, e.created_at, e.created_at::text AS k, e.completed_at,
             coalesce(r.results, '[]'::json) AS results
      FROM rl_experiments e
      LEFT JOIN LATERAL (
        SELECT json_agg(json_build_object(
                 'candidate_model', x.candidate_model,
                 'verdict', x.verdict,
                 'confidence', x.confidence,
                 'avg_quality_score', x.avg_quality_score,
                 'judge_score', x.judge_score,
                 'error_rate', x.error_rate,
                 'p95_latency_ms', x.p95_latency_ms,
                 'cost_savings_percent', x.cost_savings_percent,
                 'cost_savings_usd', x.cost_savings_usd::float8,
                 'projected_monthly_savings', x.projected_monthly_savings::float8
               ) ORDER BY x.candidate_model) AS results
        FROM rl_experiment_results x WHERE x.experiment_id = e.id
      ) r ON true
      WHERE true
        ${q.status ? sql`AND e.status = ${q.status}` : sql``}
        ${q.q ? sql`AND (to_tsvector('simple', e.name) @@ to_tsquery('simple', ${tsQuery(q.q) ?? ''}) OR e.id = ${q.q})` : sql``}
        ${after ? sql`AND (e.created_at, e.id) < (${String(after[0])}::timestamptz, ${String(after[1])})` : sql``}
      ORDER BY e.created_at DESC, e.id DESC
      LIMIT ${q.limit + 1}`;
    const p = page(rows, q.limit, (r) => [r.k, r.id]);
    return {
      items: p.items.map(({ k: _k, created_at, completed_at, ...r }) => ({
        ...r,
        created_at: iso(created_at),
        completed_at: iso(completed_at),
      })),
      next_cursor: p.next_cursor,
    };
  });
}

interface CaptureListRow {
  id: string;
  ts: Date;
  k: string;
  [key: string]: unknown;
}

interface ExperimentListRow {
  id: string;
  k: string;
  created_at: Date;
  completed_at: Date | null;
  [key: string]: unknown;
}
