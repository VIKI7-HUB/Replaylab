import type { Sql } from '../lib/db.js';
import { upsertRows } from '../lib/upsert.js';
import { captureRow, type CaptureRow } from './map.js';
import type { ReplayLabClient } from './sources.js';
import { readCursor, recordSuccess } from './state.js';

const PAGE = 1000;
const SOURCE = 'replaylab.captures' as const;

export async function writeCaptures(sql: Sql, rows: CaptureRow[]): Promise<number> {
  if (rows.length === 0) return 0;
  let min = rows[0]!.ts;
  let max = rows[0]!.ts;
  for (const r of rows) {
    if (r.ts < min) min = r.ts;
    if (r.ts > max) max = r.ts;
  }
  return sql.begin(async (tx) => {
    await tx`SELECT rl_ensure_capture_partitions(${min}, ${max})`;
    return upsertRows(tx, 'rl_captures', ['id', 'ts'], rows, { touch: 'synced_at' });
  });
}

async function knownIds(sql: Sql, ids: string[]): Promise<Set<string>> {
  if (ids.length === 0) return new Set();
  const rows = await sql<{ id: string }[]>`SELECT id FROM rl_captures WHERE id = ANY(${ids})`;
  return new Set(rows.map((r) => r.id));
}

/**
 * The captures API has no `since` filter, so this pages newest first.
 *
 * - Until one full pass has finished (`backfill_done`), it reads every page, so
 *   a backfill interrupted halfway resumes correctly next cycle.
 * - After that, it stops at the first page holding an id it already has, and
 *   reads one more page as overlap, because captures arriving while it pages
 *   shift every offset by the number of new rows.
 */
export async function syncCaptures(sql: Sql, client: ReplayLabClient): Promise<number> {
  const cursor = await readCursor(sql, SOURCE);
  let written = 0;
  let offset = 0;
  let overlapDone = false;
  let newest: CaptureRow | undefined;

  for (;;) {
    const page = await client.captures(PAGE, offset);
    if (page.length === 0) break;
    const rows = page.map(captureRow);
    if (!newest) newest = rows[0];

    const known = cursor.backfill_done ? await knownIds(sql, rows.map((r) => r.id)) : new Set<string>();
    written += await writeCaptures(sql, rows);

    if (page.length < PAGE || overlapDone) break;
    // Caught up: this page reached rows we already had. Read one more as overlap.
    if (known.size > 0) overlapDone = true;
    offset += PAGE;
  }

  await recordSuccess(sql, SOURCE, {
    ...cursor,
    backfill_done: true,
    newest_ts: newest ? newest.ts.toISOString() : cursor.newest_ts,
    newest_id: newest ? newest.id : cursor.newest_id,
  }, written);
  return written;
}
