import type { FastifyInstance } from 'fastify';
import { z } from 'zod';
import type { Sql } from '../../lib/db.js';
import { parseQuery } from '../paging.js';

const Days = (def: number) => z.coerce.number().int().min(1).max(366).default(def);

export function insightsRoutes(app: FastifyInstance, sql: Sql) {
  app.get('/api/insights/overview', async () => {
    const [rl, statuses, savings, sad, modes] = await Promise.all([
      sql<{ captures_total: number; captures_24h: number; cost_24h_usd: number; error_rate_24h: number }[]>`
        SELECT
          (SELECT coalesce(sum(captures), 0)::float8 FROM mv_rl_daily)          AS captures_total,
          count(*)::float8                                                       AS captures_24h,
          coalesce(sum(cost_usd), 0)::float8                                     AS cost_24h_usd,
          coalesce((count(*) FILTER (WHERE status_code >= 400 OR completion_state = 'error'))::float8
                   / nullif(count(*), 0), 0)                                     AS error_rate_24h
        FROM rl_captures WHERE ts >= now() - interval '24 hours'`,
      sql<{ status: string; n: number }[]>`
        SELECT status, count(*)::int AS n FROM rl_experiments GROUP BY status`,
      // For each original -> candidate pair, the newest finished experiment decides;
      // sum the projected savings of the pairs it calls safe.
      sql<{ usd: number }[]>`
        SELECT coalesce(sum(projected_monthly_savings), 0)::float8 AS usd FROM (
          SELECT DISTINCT ON (e.original_model, r.candidate_model) r.verdict, r.projected_monthly_savings
          FROM rl_experiment_results r JOIN rl_experiments e ON e.id = r.experiment_id
          WHERE e.status <> 'running' AND NOT e.dry_run
          ORDER BY e.original_model, r.candidate_model, e.created_at DESC
        ) latest WHERE verdict = 'SAFE_TO_SWITCH'`,
      sql<{ reports_total: number; reports_7d: number; avg_freshness_7d: number | null; stale_claims_7d: number }[]>`
        SELECT
          (SELECT count(*)::int FROM sad_reports)                                AS reports_total,
          count(*)::int                                                          AS reports_7d,
          avg(overall_freshness_score)::float8                                   AS avg_freshness_7d,
          (SELECT count(*)::int FROM sad_claims
             WHERE verdict = 'STALE' AND checked_at >= now() - interval '7 days') AS stale_claims_7d
        FROM sad_reports WHERE checked_at >= now() - interval '7 days'`,
      sql<{ mode: string; n: number }[]>`
        SELECT verification_mode AS mode, count(*)::int AS n FROM sad_reports GROUP BY 1`,
    ]);
    const r = rl[0]!;
    const s = sad[0]!;
    const split = { llm: 0, heuristic: 0 } as Record<string, number>;
    for (const m of modes) split[m.mode] = m.n;
    return {
      replaylab: {
        captures_total: r.captures_total,
        captures_24h: r.captures_24h,
        cost_24h_usd: r.cost_24h_usd,
        error_rate_24h: r.error_rate_24h,
        experiments_by_status: Object.fromEntries(statuses.map((x) => [x.status, x.n])),
        safe_switch_monthly_savings_usd: savings[0]?.usd ?? 0,
      },
      stale: {
        reports_total: s.reports_total,
        reports_7d: s.reports_7d,
        avg_freshness_7d: s.avg_freshness_7d,
        stale_claims_7d: s.stale_claims_7d,
        verification_mode_split: split,
      },
    };
  });

  app.get('/api/insights/replaylab/daily', async (req) => {
    const q = parseQuery(z.object({ days: Days(30), model: z.string().max(200).optional() }), req.query);
    return sql`
      SELECT to_char(day, 'YYYY-MM-DD') AS day, model, captures::float8 AS captures, tokens::float8 AS tokens,
             cost_usd::float8 AS cost_usd, avg_latency_ms, p95_latency_ms, error_rate, streamed_share
      FROM mv_rl_daily
      WHERE day > (now() AT TIME ZONE 'UTC')::date - ${q.days}::int
        ${q.model ? sql`AND model = ${q.model}` : sql``}
      ORDER BY day, model`;
  });

  app.get('/api/insights/replaylab/verdicts', async (req) => {
    const q = parseQuery(z.object({ days: Days(90) }), req.query);
    return sql`
      SELECT to_char(day, 'YYYY-MM-DD') AS day, candidate_model, safe::float8 AS safe, caution::float8 AS caution,
             do_not::float8 AS do_not, projected_monthly_savings_usd::float8 AS projected_monthly_savings_usd
      FROM mv_rl_verdicts
      WHERE day > (now() AT TIME ZONE 'UTC')::date - ${q.days}::int
      ORDER BY day, candidate_model`;
  });

  app.get('/api/insights/stale/daily', async (req) => {
    const q = parseQuery(z.object({ days: Days(30) }), req.query);
    return sql`
      SELECT to_char(day, 'YYYY-MM-DD') AS day, claim_type, claims::float8 AS claims, stale::float8 AS stale,
             partially_stale::float8 AS partially_stale, current::float8 AS current,
             unverifiable::float8 AS unverifiable, skipped::float8 AS skipped, avg_freshness
      FROM mv_sad_daily
      WHERE day > (now() AT TIME ZONE 'UTC')::date - ${q.days}::int
      ORDER BY day, claim_type`;
  });
}
