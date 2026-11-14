import type { Sql } from '../lib/db.js';

export type View = 'mv_rl_daily' | 'mv_rl_verdicts' | 'mv_sad_daily';

/** Refreshes rollups without blocking readers (each view has a unique index). */
export async function refreshViews(sql: Sql, views: Iterable<View>): Promise<View[]> {
  const done: View[] = [];
  for (const v of new Set(views)) {
    await sql.unsafe(`REFRESH MATERIALIZED VIEW CONCURRENTLY ${v}`);
    done.push(v);
  }
  return done;
}
