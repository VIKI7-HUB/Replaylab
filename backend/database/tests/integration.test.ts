// End-to-end: fake Replay Lab and Stale backends over HTTP -> sync worker ->
// Postgres -> Insights API. Needs TEST_DATABASE_URL pointing at a database whose
// name contains "test"; that database's public schema is wiped first.
import http from 'node:http';
import type { AddressInfo } from 'node:net';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { FastifyInstance } from 'fastify';
import { buildServer } from '../api/server.js';
import { loadConfig, type Config } from '../lib/config.js';
import { connect, type Sql } from '../lib/db.js';
import { migrate } from '../lib/migrate.js';
import { SyncWorker } from '../sync/worker.js';

const url = process.env.TEST_DATABASE_URL;
const suite = url && /test/.test(new URL(url).pathname) ? describe : describe.skip;

// --- fake backends ---------------------------------------------------------

const now = Date.now();
const captures = Array.from({ length: 1150 }, (_, i) => ({
  id: `cap-${String(i).padStart(5, '0')}`,
  request_id: `req-${i}`,
  // newest first, like the real API
  timestamp: new Date(now - i * 60_000).toISOString(),
  virtual_key: `vk_${i % 3}`,
  provider: 'openai',
  model: i % 2 ? 'gpt-4o' : 'gpt-4o-mini',
  tag: i % 5 === 0 ? 'json_extraction' : 'support_chat',
  messages_json: JSON.stringify([{ role: 'user', content: `prompt ${i}` }]),
  response_json: JSON.stringify({ choices: [{ message: { content: `answer ${i}` } }] }),
  prompt_tokens: 10, completion_tokens: 20, total_tokens: 30, latency_ms: 100 + (i % 50),
  cost_estimate: 0.0001, status_code: i % 97 === 0 ? 500 : 200, is_streaming: i % 3 === 0,
  hash_only: false, completion_state: i % 97 === 0 ? 'error' : 'complete', usage_estimated: false,
}));

const experiment = {
  id: 'exp-1', name: 'support chat to mini', status: 'running', original_model: 'gpt-4o',
  candidate_models: ['gpt-4o-mini'], sample_size: 10, budget_cap_dollars: 1, monthly_request_volume: 1000,
  total_replays: 10, completed_replays: 3, spent_dollars: 0.01, spent_tokens: 300, dry_run: false,
  created_at: new Date(now - 3_600_000).toISOString(), results: [] as unknown[], recommendations: [] as unknown[],
};

const report = {
  report_id: 'rpt-1', checked_at: new Date(now - 120_000).toISOString(), question: 'What is the latest Python?',
  original_answer: 'Python 3.11 is the latest.', overall_freshness_score: 12.5, verification_mode: 'llm',
  summary: { current: 0, stale: 1, partially_stale: 0, unverifiable: 0, skipped: 0, total_claims: 1 },
  usage: { tavily_calls: 1, tavily_calls_saved: 0, llm_calls: 2, latency_ms: 900 },
  corrected_answer: 'Python 3.14 is the latest.', changes: [],
  claims: [{ id: 'claim-1', text: 'Python 3.11 is the latest release', entity: 'Python', claim_type: 'version',
    staleness_risk: 0.9, skipped: false, verdict: 'STALE', freshness_score: 12.5, confidence: 0.8,
    current_value: '3.14', evidence: [{ url: 'https://python.org', title: 'x', quote: 'q', is_official: true }] }],
};

let staleUp = true;

function serve(handler: (path: URL) => unknown): Promise<http.Server> {
  const server = http.createServer((req, res) => {
    const u = new URL(req.url ?? '/', 'http://x');
    const body = handler(u);
    if (body === 503) {
      res.writeHead(503).end('down');
      return;
    }
    res.writeHead(body === undefined ? 404 : 200, { 'content-type': 'application/json' });
    res.end(JSON.stringify(body ?? { error: 'not found' }));
  });
  return new Promise((r) => server.listen(0, '127.0.0.1', () => r(server)));
}

const page = <T>(xs: T[], u: URL) => {
  const limit = Number(u.searchParams.get('limit') ?? 50);
  const offset = Number(u.searchParams.get('offset') ?? 0);
  return xs.slice(offset, offset + limit);
};

// --- the suite -------------------------------------------------------------

suite('sync + api against Postgres', () => {
  let sql: Sql;
  let rl: http.Server;
  let st: http.Server;
  let worker: SyncWorker;
  let app: FastifyInstance;
  let cfg: Config;

  const get = async (path: string) => {
    const res = await app.inject({ method: 'GET', url: path });
    return { status: res.statusCode, body: res.json() as any };
  };
  const count = async (table: string) => Number((await sql.unsafe(`SELECT count(*)::int AS n FROM ${table}`))[0]!.n);

  beforeAll(async () => {
    rl = await serve((u) => {
      if (u.pathname === '/replaylab/captures') return page(captures, u);
      if (u.pathname === '/replaylab/experiments') return page([experiment], u);
      if (u.pathname === '/replaylab/experiments/exp-1') return experiment;
      if (u.pathname === '/replaylab/config') return { retention_days: 30 };
      return undefined;
    });
    st = await serve((u) => {
      if (!staleUp) return 503;
      if (u.pathname === '/v1/history') return page([{ report_id: 'rpt-1', checked_at: report.checked_at }], u);
      if (u.pathname === '/v1/history/rpt-1') return report;
      return undefined;
    });
    cfg = loadConfig({
      DATABASE_URL: url,
      DATABASE_SSL: 'disable',
      REPLAYLAB_URL: `http://127.0.0.1:${(rl.address() as AddressInfo).port}`,
      STALE_URL: `http://127.0.0.1:${(st.address() as AddressInfo).port}`,
    });
    sql = connect(cfg, 'insights-test');
    await sql.unsafe('DROP SCHEMA public CASCADE; CREATE SCHEMA public').simple();
    await migrate(sql);
    worker = new SyncWorker(sql, cfg);
    app = await buildServer(sql);
  }, 60_000);

  afterAll(async () => {
    await app?.close();
    await sql?.end();
    rl?.close();
    st?.close();
  });

  it('copies everything in one cycle', async () => {
    const counts = await worker.cycle();
    expect(counts).toMatchObject({ 'replaylab.captures': 1150, 'stale.reports': 2 });
    expect(await count('rl_captures')).toBe(1150);
    expect(await count('rl_experiments')).toBe(1);
    expect(await count('sad_claims')).toBe(1);
  });

  it('is idempotent: a second cycle writes nothing', async () => {
    // exp-1 is still running, so it is re-fetched, but unchanged rows are skipped.
    expect(await worker.cycle()).toEqual({ 'replaylab.captures': 0, 'replaylab.experiments': 0, 'stale.reports': 0 });
    expect(await count('rl_captures')).toBe(1150);
  });

  it('picks up new captures with shifted offsets', async () => {
    for (let i = 0; i < 30; i++) {
      captures.unshift({ ...captures[0]!, id: `cap-new-${i}`, timestamp: new Date(now + (i + 1) * 1000).toISOString() });
    }
    const counts = await worker.cycle();
    expect(counts['replaylab.captures']).toBe(30);
    expect(await count('rl_captures')).toBe(1180);
  });

  it('updates a running experiment until it is final', async () => {
    Object.assign(experiment, {
      status: 'completed', completed_replays: 10, completed_at: new Date().toISOString(),
      results: [{ candidate_model: 'gpt-4o-mini', total_requests: 10, successful_requests: 10, avg_quality_score: 91,
        recommendation: 'SAFE_TO_SWITCH', projected_monthly_savings: 42.5 }],
      recommendations: [{ candidate_model: 'gpt-4o-mini', verdict: 'SAFE_TO_SWITCH', confidence: 0.9 }],
    });
    await worker.cycle();
    const [e] = await sql`SELECT status, completed_replays FROM rl_experiments WHERE id = 'exp-1'`;
    expect(e).toMatchObject({ status: 'completed', completed_replays: 10 });
    const { body } = await get('/api/insights/overview');
    expect(body.replaylab.safe_switch_monthly_savings_usd).toBe(42.5);
    expect(body.replaylab.experiments_by_status).toEqual({ completed: 1 });
  });

  it('keeps syncing Replay Lab while Stale is down, and reports it', async () => {
    staleUp = false;
    captures.unshift({ ...captures[0]!, id: 'cap-while-down', timestamp: new Date(now + 99_000).toISOString() });
    const counts = await worker.cycle();
    expect(counts['stale.reports']).toBe('error');
    expect(counts['replaylab.captures']).toBe(1);
    const { body } = await get('/api/health');
    const stale = body.sync.find((s: any) => s.source === 'stale.reports');
    expect(stale.last_error).toMatch(/503/);
    expect(body.sync.find((s: any) => s.source === 'replaylab.captures').last_error).toBeNull();
    staleUp = true;
  });

  it('pages captures by keyset with no gaps or repeats', async () => {
    const ids = new Set<string>();
    let cursor: string | null = null;
    do {
      const { body }: { body: { items: { id: string }[]; next_cursor: string | null } } =
        await get(`/api/replaylab/captures?limit=200${cursor ? `&cursor=${cursor}` : ''}`);
      for (const c of body.items) ids.add(c.id);
      cursor = body.next_cursor;
    } while (cursor);
    expect(ids.size).toBe(1181);
  });

  it('filters, searches and serves rollups', async () => {
    const problems = await get('/api/replaylab/captures?state=problem&limit=200');
    expect(problems.body.items.every((c: any) => c.status_code >= 400 || c.completion_state !== 'complete')).toBe(true);
    expect(problems.body.items.length).toBeGreaterThan(0);

    const reports = await get('/api/stale/reports?q=python');
    expect(reports.body.items[0]).toMatchObject({ report_id: 'rpt-1', stale_count: 1, current_count: 0 });

    const claims = await get('/api/stale/claims?verdict=STALE&entity=python');
    expect(claims.body.items[0]).toMatchObject({ report_id: 'rpt-1', official_evidence_count: 1 });

    const search = await get('/api/search?q=pyth');
    expect(search.body.map((h: any) => h.kind)).toEqual(expect.arrayContaining(['report', 'claim']));
    const byId = await get('/api/search?q=cap-00007');
    expect(byId.body[0]).toMatchObject({ kind: 'capture', id: 'cap-00007' });

    const daily = await get('/api/insights/replaylab/daily?days=7');
    expect(daily.body.reduce((n: number, d: any) => n + d.captures, 0)).toBe(1181);
    const verdicts = await get('/api/insights/replaylab/verdicts');
    expect(verdicts.body[0]).toMatchObject({ candidate_model: 'gpt-4o-mini', safe: 1 });
    const sad = await get('/api/insights/stale/daily');
    expect(sad.body[0]).toMatchObject({ claim_type: 'version', stale: 1 });
  });

  it('rejects bad parameters with 400 and unknown routes with 404', async () => {
    expect((await get('/api/replaylab/captures?limit=500')).status).toBe(400);
    expect((await get('/api/replaylab/captures?cursor=nope')).status).toBe(400);
    expect((await get('/api/replaylab/captures?from=yesterday')).status).toBe(400);
    expect((await get('/api/search')).status).toBe(400);
    expect((await get('/api/missing')).status).toBe(404);
  });
});
