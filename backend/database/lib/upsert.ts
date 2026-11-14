import type { Sql, Tx } from './db.js';

const ident = (s: string) => {
  if (!/^[a-z_][a-z0-9_]*$/.test(s)) throw new Error(`bad identifier ${s}`);
  return `"${s}"`;
};

/**
 * Idempotent batch upsert. The whole batch travels as one JSON parameter and is
 * expanded by json_populate_recordset, so one round trip writes up to
 * `batchSize` rows with every column typed by the table itself (timestamptz,
 * numeric, text[], jsonb) and no per-row parameter limits.
 *
 * Rows identical to what is stored are skipped by the WHERE clause, so a
 * re-sync of unchanged data writes nothing and returns 0.
 */
export async function upsertRows(
  tx: Tx,
  table: string,
  conflict: string[],
  rows: Record<string, unknown>[],
  opts: { batchSize?: number; touch?: string } = {},
): Promise<number> {
  if (rows.length === 0) return 0;
  const columns = Object.keys(rows[0]!);
  const cols = columns.map(ident).join(', ');
  const updates = columns.filter((c) => !conflict.includes(c));
  const set = [
    ...updates.map((c) => `${ident(c)} = EXCLUDED.${ident(c)}`),
    ...(opts.touch ? [`${ident(opts.touch)} = now()`] : []),
  ].join(', ');
  const changed = updates.length
    ? `WHERE (${updates.map((c) => `t.${ident(c)}`).join(', ')}) IS DISTINCT FROM (${updates
        .map((c) => `EXCLUDED.${ident(c)}`)
        .join(', ')})`
    : '';
  const action = updates.length ? `DO UPDATE SET ${set} ${changed}` : 'DO NOTHING';
  const text = `INSERT INTO ${ident(table)} AS t (${cols})
    SELECT ${cols} FROM json_populate_recordset(NULL::${ident(table)}, $1::json)
    ON CONFLICT (${conflict.map(ident).join(', ')}) ${action}`;

  const size = opts.batchSize ?? 1000;
  let written = 0;
  for (let i = 0; i < rows.length; i += size) {
    const batch = rows.slice(i, i + size);
    // The parameter is typed json, so the driver serializes the array itself.
    const res = await tx.unsafe(text, [batch as never]);
    written += res.count;
  }
  return written;
}
