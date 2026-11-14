// Calls every Insights API endpoint N times (default 200, sequentially, so
// each timing is one request on a warm server) and prints p50 / p95 / p99.
// Usage: npm run bench -- [baseUrl] [iterations]
const base = (process.argv[2] ?? process.env.INSIGHTS_URL ?? 'http://localhost:8090').replace(/\/+$/, '');
const iterations = Number(process.argv[3] ?? 200);
const TARGET_P95_MS = 50;

const models = ['gpt-4o', 'gpt-4o-mini', 'claude-3-5-sonnet', 'claude-3-haiku', 'gemini-1.5-pro', 'gemini-1.5-flash'];
const tags = ['json_extraction', 'summarization', 'support_chat', 'classification'];
const terms = ['python', 'openai', 'node lts', 'iphone', 'postgres', 'support chat', 'nightly', 'ceo'];
const pick = <T>(xs: T[], i: number) => xs[i % xs.length]!;

let captureCursor: string | null = null;

const endpoints: [string, (i: number) => string][] = [
  ['GET /api/health', () => '/api/health'],
  ['GET /api/insights/overview', () => '/api/insights/overview'],
  ['GET /api/insights/replaylab/daily', (i) => `/api/insights/replaylab/daily?days=30${i % 2 ? `&model=${pick(models, i)}` : ''}`],
  ['GET /api/insights/replaylab/verdicts', () => '/api/insights/replaylab/verdicts?days=90'],
  ['GET /api/insights/stale/daily', () => '/api/insights/stale/daily?days=30'],
  ['GET /api/replaylab/captures', () => `/api/replaylab/captures?limit=50${captureCursor ? `&cursor=${captureCursor}` : ''}`],
  ['GET /api/replaylab/captures (model)', (i) => `/api/replaylab/captures?model=${pick(models, i)}&limit=50`],
  ['GET /api/replaylab/captures (tag+streamed)', (i) => `/api/replaylab/captures?tag=${pick(tags, i)}&streamed=true&limit=50`],
  ['GET /api/replaylab/captures (problem)', () => '/api/replaylab/captures?state=problem&limit=50'],
  ['GET /api/replaylab/experiments', (i) => `/api/replaylab/experiments?limit=50${i % 3 === 0 ? '&status=completed' : ''}`],
  ['GET /api/replaylab/experiments (q)', (i) => `/api/replaylab/experiments?q=${encodeURIComponent(pick(['haiku', 'flash', 'nightly', 'json'], i))}&limit=50`],
  ['GET /api/stale/reports', (i) => `/api/stale/reports?limit=50${i % 2 ? '&min_score=20&max_score=60' : ''}`],
  ['GET /api/stale/reports (q)', (i) => `/api/stale/reports?q=${encodeURIComponent(pick(terms, i))}&limit=50`],
  ['GET /api/stale/claims', (i) => `/api/stale/claims?limit=50&verdict=${pick(['STALE', 'CURRENT', 'UNVERIFIABLE'], i)}`],
  ['GET /api/stale/claims (entity)', (i) => `/api/stale/claims?limit=50&entity=${pick(['Python', 'OpenAI', 'Apple'], i)}&claim_type=version`],
  ['GET /api/search', (i) => `/api/search?q=${encodeURIComponent(pick(terms, i))}&limit=20`],
];

function pct(sorted: number[], p: number) {
  return sorted[Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length))]!;
}

async function main() {
  const rows: { endpoint: string; p50: number; p95: number; p99: number; ok: boolean }[] = [];
  for (const [name, path] of endpoints) {
    for (let w = 0; w < 5; w++) await fetch(base + path(w)).then((r) => r.arrayBuffer()); // warm up
    captureCursor = null;
    const times: number[] = [];
    for (let i = 0; i < iterations; i++) {
      const t0 = performance.now();
      const res = await fetch(base + path(i));
      const body = (await res.json()) as { next_cursor?: string | null };
      times.push(performance.now() - t0);
      if (!res.ok) throw new Error(`${name}: HTTP ${res.status} ${JSON.stringify(body)}`);
      if (name === 'GET /api/replaylab/captures') captureCursor = body.next_cursor ?? null; // walk pages
    }
    times.sort((a, b) => a - b);
    const p95 = pct(times, 95);
    rows.push({ endpoint: name, p50: pct(times, 50), p95, p99: pct(times, 99), ok: p95 < TARGET_P95_MS });
  }
  const w = Math.max(...rows.map((r) => r.endpoint.length));
  console.log(`${'endpoint'.padEnd(w)}   p50 ms   p95 ms   p99 ms`);
  for (const r of rows) {
    console.log(
      `${r.endpoint.padEnd(w)} ${r.p50.toFixed(1).padStart(8)} ${r.p95.toFixed(1).padStart(8)} ${r.p99.toFixed(1).padStart(8)}${r.ok ? '' : '  <- over target'}`,
    );
  }
  const failed = rows.filter((r) => !r.ok);
  console.log(`\n${iterations} requests per endpoint against ${base}. Target p95 < ${TARGET_P95_MS} ms: ${failed.length ? `${failed.length} over` : 'all met'}.`);
  if (failed.length) process.exitCode = 1;
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
