// Loads bench/seed.sql (1M captures, 5k experiments, 50k reports) into DATABASE_URL.
// `npm run seed -- --clean` deletes the synthetic rows (ids starting with "bench-") instead.
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadConfig } from '../lib/config.js';
import { connect } from '../lib/db.js';
import { migrate } from '../lib/migrate.js';

async function main() {
  const sql = connect(loadConfig(), 'insights-seed');
  await migrate(sql);
  const t0 = Date.now();
  if (process.argv.includes('--clean')) {
    await sql`DELETE FROM rl_captures WHERE id LIKE 'bench-%'`;
    await sql`DELETE FROM rl_experiments WHERE id LIKE 'bench-%'`;
    await sql`DELETE FROM sad_reports WHERE report_id LIKE 'bench-%'`;
    for (const v of ['mv_rl_daily', 'mv_rl_verdicts', 'mv_sad_daily']) {
      await sql.unsafe(`REFRESH MATERIALIZED VIEW CONCURRENTLY ${v}`);
    }
    console.log(`removed synthetic rows in ${Date.now() - t0} ms`);
  } else {
    // seed.sql sits next to this file in source, and two levels up from dist/bench/.
    const here = path.dirname(fileURLToPath(import.meta.url));
    const candidates = [path.resolve(here, 'seed.sql'), path.resolve(here, '../../bench/seed.sql')];
    let body: string | undefined;
    for (const f of candidates) body ??= await readFile(f, 'utf8').catch(() => undefined);
    if (!body) throw new Error('bench/seed.sql not found');
    await sql.unsafe(body).simple();
    console.log(`seeded in ${((Date.now() - t0) / 1000).toFixed(1)} s`);
  }
  await sql.end();
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
