import { z } from 'zod';

const EnvSchema = z.object({
  DATABASE_URL: z.string().min(1).default('postgres://insights:insights@localhost:5432/insights'),
  REPLAYLAB_URL: z.string().default('http://localhost:8086'),
  STALE_URL: z.string().default('http://localhost:8000'),
  STALE_API_KEY: z.string().optional(),
  SYNC_INTERVAL_MS: z.coerce.number().int().min(250).default(5000),
  // Render and most hosts set PORT; API_PORT wins when both are set.
  API_PORT: z.coerce.number().int().positive().optional(),
  PORT: z.coerce.number().int().positive().optional(),
  API_HOST: z.string().default('0.0.0.0'),
  DB_POOL_MAX: z.coerce.number().int().positive().default(10),
  // Set to "require" for managed Postgres that needs TLS from outside its network.
  DATABASE_SSL: z.enum(['disable', 'require', 'prefer']).default('prefer'),
  HTTP_TIMEOUT_MS: z.coerce.number().int().positive().default(60000),
  // Turn off a source entirely (e.g. when only one backend exists).
  SYNC_REPLAYLAB: z.enum(['true', 'false']).default('true'),
  SYNC_STALE: z.enum(['true', 'false']).default('true'),
});

export type Config = {
  databaseUrl: string;
  replaylabUrl: string;
  staleUrl: string;
  staleApiKey: string | undefined;
  syncIntervalMs: number;
  apiPort: number;
  apiHost: string;
  dbPoolMax: number;
  databaseSsl: 'disable' | 'require' | 'prefer';
  httpTimeoutMs: number;
  syncReplaylab: boolean;
  syncStale: boolean;
};

const trimSlash = (u: string) => u.replace(/\/+$/, '');

export function loadConfig(env: NodeJS.ProcessEnv = process.env): Config {
  const e = EnvSchema.parse(env);
  return {
    databaseUrl: e.DATABASE_URL,
    replaylabUrl: trimSlash(e.REPLAYLAB_URL),
    staleUrl: trimSlash(e.STALE_URL),
    staleApiKey: e.STALE_API_KEY || undefined,
    syncIntervalMs: e.SYNC_INTERVAL_MS,
    apiPort: e.API_PORT ?? e.PORT ?? 8090,
    apiHost: e.API_HOST,
    dbPoolMax: e.DB_POOL_MAX,
    databaseSsl: e.DATABASE_SSL,
    httpTimeoutMs: e.HTTP_TIMEOUT_MS,
    syncReplaylab: e.SYNC_REPLAYLAB === 'true',
    syncStale: e.SYNC_STALE === 'true',
  };
}
