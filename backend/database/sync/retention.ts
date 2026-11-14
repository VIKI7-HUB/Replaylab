import type { Sql } from '../lib/db.js';
import { log } from '../lib/log.js';
import type { ReplayLabClient } from './sources.js';
import { readCursor, recordSuccess } from './state.js';

const SOURCE = 'replaylab.captures' as const;
const EVERY_MS = 60 * 60 * 1000;

/**
 * Hourly: Replay Lab deletes captures older than its retention_days, so delete
 * the same ones here. Whole months past the cutoff are dropped as partitions
 * (instant, no bloat); the boundary month is trimmed with a DELETE.
 */
export async function reconcileRetention(sql: Sql, client: ReplayLabClient, force = false): Promise<number> {
  const cursor = await readCursor(sql, SOURCE);
  const last = cursor.retention_checked_at ? Date.parse(cursor.retention_checked_at) : 0;
  if (!force && Date.now() - last < EVERY_MS) return 0;

  const days = await client.retentionDays();
  let removed = 0;
  if (days > 0) {
    const cutoff = new Date(Date.now() - days * 86_400_000);
    const parts = await sql<{ name: string; upper: Date }[]>`
      SELECT c.relname AS name,
             (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'TO \\(''([^'']+)''\\)'))[1]::timestamptz AS upper
      FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
      WHERE i.inhparent = 'rl_captures'::regclass`;
    for (const p of parts) {
      if (p.upper <= cutoff) {
        if (!/^rl_captures_\d{4}_\d{2}$/.test(p.name)) continue;
        const counted = await sql.unsafe(`SELECT count(*)::int AS n FROM "${p.name}"`);
        const n = Number(counted[0]?.n ?? 0);
        await sql.unsafe(`DROP TABLE "${p.name}"`);
        removed += n;
        log.info('retention dropped partition', { partition: p.name, rows: n });
      }
    }
    removed += (await sql`DELETE FROM rl_captures WHERE ts < ${cutoff}`).count;
  }
  await recordSuccess(sql, SOURCE, { ...cursor, retention_checked_at: new Date().toISOString(), retention_days: days }, removed);
  return removed;
}
