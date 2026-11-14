import { loadConfig } from './lib/config.js';
import { connect } from './lib/db.js';
import { errMessage, log } from './lib/log.js';
import { migrate } from './lib/migrate.js';
import { buildServer } from './api/server.js';
import { SyncWorker } from './sync/worker.js';

/**
 * One entry point, four modes:
 *   migrate  apply pending migrations and exit
 *   sync     migrate, then run the sync worker
 *   api      migrate, then serve the Insights API
 *   all      migrate, then run both in one process (what a single free host runs)
 *   once     migrate, run one sync cycle, exit (CI and manual backfills)
 */
async function main() {
  const mode = process.argv[2] ?? 'all';
  if (!['migrate', 'sync', 'api', 'all', 'once'].includes(mode)) {
    throw new Error(`unknown mode "${mode}"; use migrate | sync | api | all | once`);
  }
  const cfg = loadConfig();
  const sql = connect(cfg, `insights-${mode}`);

  // The database may still be starting (compose, or a host waking up).
  for (let attempt = 1; ; attempt++) {
    try {
      await sql`SELECT 1`;
      break;
    } catch (err) {
      if (attempt >= 30) throw err;
      log.warn('waiting for database', { attempt, error: errMessage(err) });
      await new Promise((r) => setTimeout(r, 2000));
    }
  }
  const applied = await migrate(sql);
  log.info('migrations up to date', { applied });
  if (mode === 'migrate' || mode === 'once') {
    if (mode === 'once') {
      const counts = await new SyncWorker(sql, cfg).cycle();
      if (Object.values(counts).includes('error')) process.exitCode = 1;
    }
    await sql.end();
    return;
  }

  const stops: (() => Promise<void>)[] = [];
  if (mode === 'api' || mode === 'all') {
    const app = await buildServer(sql);
    await app.listen({ port: cfg.apiPort, host: cfg.apiHost });
    log.info('insights api listening', { port: cfg.apiPort });
    stops.push(() => app.close());
  }
  if (mode === 'sync' || mode === 'all') {
    const worker = new SyncWorker(sql, cfg);
    void worker.run();
    stops.push(async () => worker.stop());
  }

  const shutdown = async (signal: string) => {
    log.info('shutting down', { signal });
    for (const stop of stops) await stop().catch(() => {});
    await sql.end({ timeout: 5 });
    process.exit(0);
  };
  process.on('SIGTERM', () => void shutdown('SIGTERM'));
  process.on('SIGINT', () => void shutdown('SIGINT'));
}

main().catch((err) => {
  log.error('fatal', { error: errMessage(err) });
  process.exit(1);
});
