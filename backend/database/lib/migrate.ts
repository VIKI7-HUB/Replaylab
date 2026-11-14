import { readdir, readFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import type { Sql } from './db.js';
import { log } from './log.js';

/** Finds migrations/ whether running from source (tsx) or from dist/. */
export function migrationsDir(): string {
  if (process.env.MIGRATIONS_DIR) return path.resolve(process.env.MIGRATIONS_DIR);
  const here = path.dirname(fileURLToPath(import.meta.url));
  for (const candidate of [path.resolve(here, '../migrations'), path.resolve(here, '../../migrations')]) {
    if (existsSync(candidate)) return candidate;
  }
  throw new Error('migrations/ directory not found; set MIGRATIONS_DIR');
}

/**
 * Applies every numbered .sql file in migrations/ that hasn't run yet, each in
 * its own transaction, under an advisory lock so concurrent starters (API and
 * sync booting together) don't race.
 */
export async function migrate(sql: Sql): Promise<string[]> {
  const dir = migrationsDir();
  const files = (await readdir(dir)).filter((f) => /^\d+_.*\.sql$/.test(f)).sort();
  const applied: string[] = [];
  await sql.begin(async (tx) => {
    await tx`SELECT pg_advisory_xact_lock(724001)`;
    await tx`CREATE TABLE IF NOT EXISTS schema_migrations (
      version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`;
    const done = new Set((await tx<{ version: string }[]>`SELECT version FROM schema_migrations`).map((r) => r.version));
    for (const file of files) {
      if (done.has(file)) continue;
      const body = await readFile(path.join(dir, file), 'utf8');
      await tx.unsafe(body).simple();
      await tx`INSERT INTO schema_migrations (version) VALUES (${file})`;
      applied.push(file);
      log.info('migration applied', { file });
    }
  });
  return applied;
}
