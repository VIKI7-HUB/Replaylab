import type { Sql } from '../lib/db.js';
import { HttpError } from '../lib/http.js';
import { upsertRows } from '../lib/upsert.js';
import { experimentResultRows, experimentRow, isFinal } from './map.js';
import type { Experiment, ReplayLabClient } from './sources.js';
import { readCursor, recordSuccess } from './state.js';

const PAGE = 200;
const SOURCE = 'replaylab.experiments' as const;

/** Writes an experiment and replaces its results in one transaction. Returns rows changed. */
export async function writeExperiment(sql: Sql, e: Experiment): Promise<number> {
  const results = experimentResultRows(e);
  return sql.begin(async (tx) => {
    let n = await upsertRows(tx, 'rl_experiments', ['id'], [experimentRow(e)], { touch: 'synced_at' });
    const keep = results.map((r) => r.candidate_model);
    const del = await tx`
      DELETE FROM rl_experiment_results
      WHERE experiment_id = ${e.id} AND NOT (candidate_model = ANY(${keep}))`;
    n += del.count;
    n += await upsertRows(tx, 'rl_experiment_results', ['experiment_id', 'candidate_model'], results);
    return n;
  });
}

/**
 * Upserts every listed experiment, newest first, stopping at the first page in
 * which every experiment is already stored with a final status (final
 * experiments never change). Then re-fetches each experiment still stored as
 * `running`, so progress keeps updating until it reaches a final status.
 */
export async function syncExperiments(sql: Sql, client: ReplayLabClient): Promise<number> {
  const cursor = await readCursor(sql, SOURCE);
  let written = 0;
  const seen = new Set<string>();

  for (let offset = 0; ; offset += PAGE) {
    const page = await client.experiments(PAGE, offset);
    if (page.length === 0) break;
    const ids = page.map((e) => e.id);
    const stored = await sql<{ id: string; status: string }[]>`
      SELECT id, status FROM rl_experiments WHERE id = ANY(${ids})`;
    const finalStored = new Set(stored.filter((r) => isFinal(r.status)).map((r) => r.id));

    for (const e of page) {
      seen.add(e.id);
      if (finalStored.has(e.id)) continue;
      written += await writeExperiment(sql, e);
    }
    if (page.length < PAGE) break;
    if (cursor.backfill_done && page.every((e) => finalStored.has(e.id))) break;
  }

  const running = await sql<{ id: string }[]>`SELECT id FROM rl_experiments WHERE status = 'running'`;
  for (const { id } of running) {
    if (seen.has(id)) continue;
    try {
      written += await writeExperiment(sql, await client.experiment(id));
    } catch (err) {
      // Deleted at the source (or the source restarted with an empty store):
      // it can never finish, so stop polling it.
      if (err instanceof HttpError && err.status === 404) {
        written += (await sql`
          UPDATE rl_experiments SET status = 'failed', stop_reason = coalesce(stop_reason, 'missing at source')
          WHERE id = ${id}`).count;
      } else {
        throw err;
      }
    }
  }

  await recordSuccess(sql, SOURCE, { ...cursor, backfill_done: true }, written);
  return written;
}
