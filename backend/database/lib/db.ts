import postgres from 'postgres';
import type { Config } from './config.js';

export type Sql = postgres.Sql<Record<string, never>>;
export type Tx = Sql | postgres.TransactionSql<Record<string, never>>;

export function connect(cfg: Pick<Config, 'databaseUrl' | 'dbPoolMax' | 'databaseSsl'>, appName = 'insights'): Sql {
  const local = /@(localhost|127\.0\.0\.1|postgres|db)[:/]/.test(cfg.databaseUrl);
  const ssl = cfg.databaseSsl === 'disable' || (cfg.databaseSsl === 'prefer' && local)
    ? false
    : cfg.databaseSsl === 'require' ? 'require' : 'prefer';
  return postgres(cfg.databaseUrl, {
    max: cfg.dbPoolMax,
    ssl,
    idle_timeout: 30,
    connect_timeout: 10,
    onnotice: () => {},
    connection: { application_name: appName },
    transform: { undefined: null },
  }) as Sql;
}
