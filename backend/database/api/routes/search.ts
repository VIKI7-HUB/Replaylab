import type { FastifyInstance } from 'fastify';
import { z } from 'zod';
import type { Sql } from '../../lib/db.js';
import { iso, parseQuery } from '../paging.js';
import { tsQuery } from '../text.js';

const SearchQuery = z.object({
  q: z.string().trim().min(1).max(200),
  limit: z.coerce.number().int().min(1).max(50).default(20),
});

/** Matches read per kind (newest first) before ranking; bounds the work of any query. */
const POOL = 200;

interface Hit {
  kind: 'capture' | 'experiment' | 'report' | 'claim';
  id: string;
  report_id: string | null;
  title: string;
  snippet: string | null;
  at: Date;
  score: number;
}

/**
 * Global search across experiments, reports, claims and captures.
 *
 * - Text: every word of the query must match a word (3+ letters: a word prefix)
 *   through the GIN full-text indexes. Each kind takes its POOL newest matches,
 *   then ranks them by pg_trgm word similarity.
 * - Ids: an exact capture, experiment or report id is a primary-key lookup and
 *   ranks first.
 *
 * Results rank by similarity, then recency.
 */
export function searchRoutes(app: FastifyInstance, sql: Sql) {
  app.get('/api/search', async (req) => {
    const { q, limit } = parseQuery(SearchQuery, req.query);
    const tsq = tsQuery(q);
    const text = tsq
      ? sql`
      UNION ALL
      (SELECT 'experiment', id, NULL, coalesce(nullif(name, ''), id),
              original_model || ' -> ' || array_to_string(candidate_models, ', '),
              created_at, word_similarity(${q}, name)
       FROM (SELECT * FROM rl_experiments
             WHERE to_tsvector('simple', name) @@ to_tsquery('simple', ${tsq})
             ORDER BY created_at DESC LIMIT ${POOL}) e
       ORDER BY 7 DESC, created_at DESC LIMIT ${limit})
      UNION ALL
      (SELECT 'report', report_id, report_id, coalesce(nullif(question, ''), left(original_answer, 120)),
              left(original_answer, 160), checked_at, word_similarity(${q}, question)
       FROM (SELECT * FROM sad_reports
             WHERE to_tsvector('simple', coalesce(question, '')) @@ to_tsquery('simple', ${tsq})
             ORDER BY checked_at DESC LIMIT ${POOL}) r
       ORDER BY 7 DESC, checked_at DESC LIMIT ${limit})
      UNION ALL
      (SELECT 'claim', claim_id, report_id, coalesce(nullif(entity, ''), claim_type), left(text, 160), checked_at,
              greatest(word_similarity(${q}, text), word_similarity(${q}, entity))
       FROM (SELECT * FROM sad_claims
             WHERE to_tsvector('simple', text || ' ' || entity) @@ to_tsquery('simple', ${tsq})
             ORDER BY checked_at DESC LIMIT ${POOL}) c
       ORDER BY 7 DESC, checked_at DESC LIMIT ${limit})`
      : sql``;

    const rows = await sql<Hit[]>`
      (SELECT 'capture' AS kind, id, NULL AS report_id, model AS title,
              nullif(concat_ws(' · ', nullif(tag, ''), provider), '') AS snippet, ts AS at, 2::real AS score
       FROM rl_captures WHERE id = ${q} LIMIT 1)
      UNION ALL
      (SELECT 'experiment', id, NULL, coalesce(nullif(name, ''), id), original_model, created_at, 2
       FROM rl_experiments WHERE id = ${q})
      UNION ALL
      (SELECT 'report', report_id, report_id, coalesce(nullif(question, ''), report_id), left(original_answer, 160),
              checked_at, 2
       FROM sad_reports WHERE report_id = ${q})
      ${text}`;

    rows.sort((a, b) => b.score - a.score || b.at.getTime() - a.at.getTime());
    const seen = new Set<string>();
    const unique = rows.filter((r) => {
      const key = `${r.kind}:${r.report_id ?? ''}:${r.id}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
    return unique.slice(0, limit).map((r) => ({
      kind: r.kind,
      id: r.id,
      ...(r.kind === 'claim' && r.report_id ? { report_id: r.report_id } : {}),
      title: r.title,
      snippet: r.snippet,
      at: iso(r.at),
    }));
  });
}
