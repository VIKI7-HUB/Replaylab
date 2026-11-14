import cors from '@fastify/cors';
import Fastify, { type FastifyInstance } from 'fastify';
import { ZodError } from 'zod';
import type { Sql } from '../lib/db.js';
import { BadRequest } from './paging.js';
import { healthRoutes } from './routes/health.js';
import { insightsRoutes } from './routes/insights.js';
import { replaylabRoutes } from './routes/replaylab.js';
import { searchRoutes } from './routes/search.js';
import { staleRoutes } from './routes/stale.js';

/** The read-only Insights API. Every route is a GET; errors are `{ "error": "..." }`. */
export async function buildServer(sql: Sql, opts: { logger?: boolean } = {}): Promise<FastifyInstance> {
  const app = Fastify({
    logger: opts.logger ? { level: 'info' } : false,
  });
  await app.register(cors, { origin: true, methods: ['GET', 'HEAD', 'OPTIONS'] });

  app.setErrorHandler((err, _req, reply) => {
    if (err instanceof BadRequest || err instanceof ZodError) {
      return reply.code(400).send({ error: err.message });
    }
    const status = (err as { statusCode?: number }).statusCode;
    if (status && status >= 400 && status < 500) {
      return reply.code(status).send({ error: (err as Error).message });
    }
    app.log.error(err);
    return reply.code(500).send({ error: 'internal error' });
  });
  app.setNotFoundHandler((req, reply) => reply.code(404).send({ error: `no route ${req.method} ${req.url.split('?')[0]}` }));

  app.get('/', async () => ({ service: 'llm-reliability-insights', docs: '/api/health' }));
  healthRoutes(app, sql);
  insightsRoutes(app, sql);
  replaylabRoutes(app, sql);
  staleRoutes(app, sql);
  searchRoutes(app, sql);
  return app;
}
