import type { FastifyInstance } from 'fastify';
import type { Sql } from '../../lib/db.js';
import { iso } from '../paging.js';

interface SyncRow {
  source: string;
  last_success_at: Date | null;
  last_error: string | null;
  lag_seconds: number | null;
}

export function healthRoutes(app: FastifyInstance, sql: Sql) {
  app.get('/api/health', async (_req, reply) => {
    try {
      const rows = await sql<SyncRow[]>`
        SELECT source, last_success_at, last_error,
               extract(epoch FROM now() - last_success_at)::float8 AS lag_seconds
        FROM sync_state ORDER BY source`;
      return {
        status: 'ok',
        db: 'ok',
        sync: rows.map((r) => ({
          source: r.source,
          last_success_at: iso(r.last_success_at),
          last_error: r.last_error,
          lag_seconds: r.lag_seconds == null ? null : Math.round(r.lag_seconds),
        })),
      };
    } catch {
      return reply.code(503).send({ status: 'down', db: 'down', sync: [] });
    }
  });
}
