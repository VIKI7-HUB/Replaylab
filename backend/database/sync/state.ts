import type { Sql } from '../lib/db.js';

export type Source = 'replaylab.captures' | 'replaylab.experiments' | 'stale.reports';

export interface StreamCursor {
  /** True once a full pass over the source has completed at least once. */
  backfill_done?: boolean;
  newest_ts?: string;
  newest_id?: string;
  retention_checked_at?: string;
  retention_days?: number;
}

export async function readCursor(sql: Sql, source: Source): Promise<StreamCursor> {
  const rows = await sql<{ cursor: StreamCursor }[]>`SELECT cursor FROM sync_state WHERE source = ${source}`;
  return rows[0]?.cursor ?? {};
}

export async function recordSuccess(sql: Sql, source: Source, cursor: StreamCursor, rows: number) {
  await sql`
    INSERT INTO sync_state (source, cursor, last_success_at, last_attempt_at, last_error, rows_upserted)
    VALUES (${source}, ${sql.json(cursor as never)}, now(), now(), NULL, ${rows})
    ON CONFLICT (source) DO UPDATE SET
      cursor = EXCLUDED.cursor,
      last_success_at = now(),
      last_attempt_at = now(),
      last_error = NULL,
      rows_upserted = sync_state.rows_upserted + ${rows}`;
}

export async function recordError(sql: Sql, source: Source, error: string) {
  await sql`
    INSERT INTO sync_state (source, last_attempt_at, last_error)
    VALUES (${source}, now(), ${error})
    ON CONFLICT (source) DO UPDATE SET last_attempt_at = now(), last_error = EXCLUDED.last_error`;
}
