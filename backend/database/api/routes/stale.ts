import type { FastifyInstance } from 'fastify';
import { z } from 'zod';
import type { Sql } from '../../lib/db.js';
import { decodeCursor, iso, page, PageQuery, parseQuery } from '../paging.js';
import { tsQuery } from '../text.js';

const ReportQuery = PageQuery.extend({
  min_score: z.coerce.number().min(0).max(100).optional(),
  max_score: z.coerce.number().min(0).max(100).optional(),
  mode: z.enum(['llm', 'heuristic']).optional(),
  q: z.string().max(200).optional(),
});

const ClaimQuery = PageQuery.extend({
  // SKIPPED selects claims triage skipped (their verdict is null).
  verdict: z.enum(['CURRENT', 'STALE', 'PARTIALLY_STALE', 'UNVERIFIABLE', 'SKIPPED']).optional(),
  claim_type: z.string().max(100).optional(),
  entity: z.string().max(200).optional(),
  q: z.string().max(200).optional(),
});

export function staleRoutes(app: FastifyInstance, sql: Sql) {
  app.get('/api/stale/reports', async (req) => {
    const q = parseQuery(ReportQuery, req.query);
    const after = decodeCursor(q.cursor, 2);
    const rows = await sql<ReportRow[]>`
      SELECT report_id, checked_at, checked_at::text AS k, question, overall_freshness_score,
             verification_mode, current_count, stale_count, partially_stale_count, unverifiable_count,
             skipped_count, total_claims
      FROM sad_reports
      WHERE true
        ${q.min_score != null ? sql`AND overall_freshness_score >= ${q.min_score}` : sql``}
        ${q.max_score != null ? sql`AND overall_freshness_score <= ${q.max_score}` : sql``}
        ${q.mode ? sql`AND verification_mode = ${q.mode}` : sql``}
        ${q.q ? sql`AND (to_tsvector('simple', coalesce(question, '')) @@ to_tsquery('simple', ${tsQuery(q.q) ?? ''}) OR report_id = ${q.q})` : sql``}
        ${after ? sql`AND (checked_at, report_id) < (${String(after[0])}::timestamptz, ${String(after[1])})` : sql``}
      ORDER BY checked_at DESC, report_id DESC
      LIMIT ${q.limit + 1}`;
    const p = page(rows, q.limit, (r) => [r.k, r.report_id]);
    return {
      items: p.items.map(({ k: _k, checked_at, ...r }) => ({
        ...r,
        checked_at: iso(checked_at),
        summary: {
          current: r.current_count,
          stale: r.stale_count,
          partially_stale: r.partially_stale_count,
          unverifiable: r.unverifiable_count,
          skipped: r.skipped_count,
          total_claims: r.total_claims,
        },
      })),
      next_cursor: p.next_cursor,
    };
  });

  app.get('/api/stale/claims', async (req) => {
    const q = parseQuery(ClaimQuery, req.query);
    const after = decodeCursor(q.cursor, 3);
    const rows = await sql<ClaimRow[]>`
      SELECT report_id, claim_id, checked_at, checked_at::text AS k, text, entity, claim_type,
             staleness_risk, skipped, skip_reason, verdict, freshness_score, confidence, current_value,
             evidence_count, official_evidence_count
      FROM sad_claims
      WHERE true
        ${q.verdict === 'SKIPPED' ? sql`AND skipped` : q.verdict ? sql`AND verdict = ${q.verdict}` : sql``}
        ${q.claim_type ? sql`AND claim_type = ${q.claim_type}` : sql``}
        ${q.entity ? sql`AND lower(entity) = lower(${q.entity})` : sql``}
        ${q.q ? sql`AND to_tsvector('simple', text || ' ' || entity) @@ to_tsquery('simple', ${tsQuery(q.q) ?? ''})` : sql``}
        ${after
          ? sql`AND (checked_at, report_id, claim_id) < (${String(after[0])}::timestamptz, ${String(after[1])}, ${String(after[2])})`
          : sql``}
      ORDER BY checked_at DESC, report_id DESC, claim_id DESC
      LIMIT ${q.limit + 1}`;
    const p = page(rows, q.limit, (r) => [r.k, r.report_id, r.claim_id]);
    return {
      items: p.items.map(({ k: _k, checked_at, ...r }) => ({ ...r, checked_at: iso(checked_at) })),
      next_cursor: p.next_cursor,
    };
  });
}

interface ReportRow {
  report_id: string;
  k: string;
  checked_at: Date;
  current_count: number;
  stale_count: number;
  partially_stale_count: number;
  unverifiable_count: number;
  skipped_count: number;
  total_claims: number;
  [key: string]: unknown;
}

interface ClaimRow {
  report_id: string;
  claim_id: string;
  k: string;
  checked_at: Date;
  [key: string]: unknown;
}
