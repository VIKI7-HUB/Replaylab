import type { Sql } from '../lib/db.js';
import { HttpError } from '../lib/http.js';
import { upsertRows } from '../lib/upsert.js';
import { claimRows, reportRow } from './map.js';
import type { Report, StaleClient } from './sources.js';
import { readCursor, recordSuccess } from './state.js';

const PAGE = 100; // the service's maximum
const SOURCE = 'stale.reports' as const;

/** Writes a report and its claims in one transaction. Returns rows changed. */
export async function writeReport(sql: Sql, r: Report): Promise<number> {
  const claims = claimRows(r);
  return sql.begin(async (tx) => {
    let n = await upsertRows(tx, 'sad_reports', ['report_id'], [reportRow(r)], { touch: 'synced_at' });
    n += await upsertRows(tx, 'sad_claims', ['report_id', 'claim_id'], claims);
    return n;
  });
}

/**
 * Reports are immutable, so only unknown ids are fetched. History pages newest
 * first; once a full pass has completed, paging stops at the first page that
 * contains a known report.
 */
export async function syncReports(sql: Sql, client: StaleClient): Promise<number> {
  const cursor = await readCursor(sql, SOURCE);
  let written = 0;

  for (let offset = 0; ; offset += PAGE) {
    const page = await client.history(PAGE, offset);
    if (page.length === 0) break;
    const ids = page.map((h) => h.report_id);
    const known = new Set(
      (await sql<{ report_id: string }[]>`SELECT report_id FROM sad_reports WHERE report_id = ANY(${ids})`).map(
        (r) => r.report_id,
      ),
    );
    for (const id of ids) {
      if (known.has(id)) continue;
      try {
        written += await writeReport(sql, await client.report(id));
      } catch (err) {
        if (err instanceof HttpError && err.status === 404) continue; // deleted between list and fetch
        throw err;
      }
    }
    if (page.length < PAGE) break;
    if (cursor.backfill_done && known.size > 0) break;
  }

  await recordSuccess(sql, SOURCE, { ...cursor, backfill_done: true }, written);
  return written;
}
