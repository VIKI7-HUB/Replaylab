# Insights: the read database

A **PostgreSQL 17** copy of everything Replay Lab and the Stale-Answer Detector store, built for fast reads, plus the **sync worker** that fills it and the read-only **Insights API** (port `8090`) that the console uses for search, lists and analytics.

Each service keeps its own SQLite database as the **source of truth**. This database is a copy filled only through their **HTTP APIs**: it never opens their `.db` files, and neither service knows it exists.

```
 Replay Lab (Go)              Stale-Answer Detector (Python)
 /replaylab/captures          /v1/history
 /replaylab/experiments       /v1/history/{id}
 /replaylab/config
        │  HTTP, newest first            │
        └──────────────┬─────────────────┘
                       ▼
             sync worker (sync/)         every SYNC_INTERVAL_MS; idempotent upserts,
                       │                 backoff per backend, hourly retention reconcile
                       ▼
             PostgreSQL 17               rl_captures (monthly partitions), rl_experiments,
                       │                 rl_experiment_results, sad_reports, sad_claims,
                       │                 3 materialized rollups, sync_state
                       ▼
             Insights API (api/) :8090   read-only, keyset pagination
                       │
                       ▼
             console (VITE_INSIGHTS_URL): command-palette search, report history search
```

## Run it

Start the backends first (see [`../README.md`](../README.md); Replay Lab on `:8086`, Stale on `:8000` by default), then:

```bash
cd backend/database
docker compose up --build
```

That starts Postgres, applies the migrations to an empty database, and runs the sync worker and the API. Open http://localhost:8090/api/health. Point the console at it with `VITE_INSIGHTS_URL=http://localhost:8090` in `frontend/.env.local`.

Backends on other ports or hosts: `REPLAYLAB_URL=http://host.docker.internal:8085 STALE_URL=... docker compose up`.

Without Docker (Node 22+, any Postgres 17):

```bash
npm install
cp .env.example .env              # then set DATABASE_URL etc., or export them
npm run dev                       # migrate, then sync + API in one process (tsx)
```

`main.ts` takes a mode: `migrate`, `sync`, `api`, `all` (both in one process, what the free Render service runs), or `once` (one sync cycle, then exit; exit code 1 if a stream failed).

### Configuration

| Variable | Default | |
| :--- | :--- | :--- |
| `DATABASE_URL` | `postgres://insights:insights@localhost:5432/insights` | |
| `DATABASE_SSL` | `prefer` | `disable`, `prefer` (TLS unless the host is local), `require` |
| `REPLAYLAB_URL` | `http://localhost:8086` | |
| `STALE_URL` | `http://localhost:8000` | |
| `STALE_API_KEY` | unset | sent as `X-API-Key` when set |
| `SYNC_INTERVAL_MS` | `5000` | |
| `API_PORT` | `8090` (or `PORT`) | |
| `SYNC_REPLAYLAB`, `SYNC_STALE` | `true` | set `false` to skip a backend entirely |
| `HTTP_TIMEOUT_MS` | `60000` | long enough for a sleeping free-tier backend to wake |
| `DB_POOL_MAX` | `10` | |

## Insights API

Read-only, CORS open, JSON everywhere. Errors are `{ "error": "message" }`: `400` for bad parameters, `404` for unknown routes. Times are ISO 8601 UTC; days are UTC dates (`YYYY-MM-DD`).

**Lists** use keyset pagination: `?limit=` (default 50, max 200) and `?cursor=` (opaque, from the previous page), returning `{ "items": [...], "next_cursor": "..." | null }`. Empty query parameters count as unset.

| Method | Path | Returns |
| :--- | :--- | :--- |
| GET | `/api/health` | `{ status, db: "ok" \| "down", sync: [{ source, last_success_at, last_error, lag_seconds }] }`; `503` when the database is down |
| GET | `/api/insights/overview` | `{ replaylab: { captures_total, captures_24h, cost_24h_usd, error_rate_24h, experiments_by_status: {status: n}, safe_switch_monthly_savings_usd }, stale: { reports_total, reports_7d, avg_freshness_7d, stale_claims_7d, verification_mode_split: { llm, heuristic } } }` |
| GET | `/api/insights/replaylab/daily?days=30&model=` | `[{ day, model, captures, tokens, cost_usd, avg_latency_ms, p95_latency_ms, error_rate, streamed_share }]` |
| GET | `/api/insights/replaylab/verdicts?days=90` | `[{ day, candidate_model, safe, caution, do_not, projected_monthly_savings_usd }]` |
| GET | `/api/insights/stale/daily?days=30` | `[{ day, claim_type, claims, stale, partially_stale, current, unverifiable, skipped, avg_freshness }]` |
| GET | `/api/replaylab/captures?model=&tag=&virtual_key=&streamed=&state=&from=&to=` | paged capture summaries (every capture field except messages, params and response text) |
| GET | `/api/replaylab/experiments?status=&q=` | paged experiment summaries, each with `results[]` (`candidate_model`, `verdict`, `confidence`, `avg_quality_score`, `judge_score`, `error_rate`, `p95_latency_ms`, `cost_savings_percent`, `cost_savings_usd`, `projected_monthly_savings`) |
| GET | `/api/stale/reports?min_score=&max_score=&mode=&q=` | paged report summaries: `report_id`, `checked_at`, `question`, `overall_freshness_score`, `verification_mode`, `stale_count`, `current_count`, the other counts, and `summary {...}` |
| GET | `/api/stale/claims?verdict=&claim_type=&entity=&q=` | paged claims, each with `report_id` |
| GET | `/api/search?q=&limit=20` | `[{ kind: "capture" \| "experiment" \| "report" \| "claim", id, title, snippet, at }]` (claims also carry `report_id`) |

Parameter details:

- `streamed`: `true`, `false` or `any`. `state`: `complete`, `error`, `abandoned`, `truncated`, or `problem` (anything not complete, or HTTP status ≥ 400). `from` / `to`: ISO dates, `from` inclusive, `to` exclusive.
- `verdict` on claims: `CURRENT`, `STALE`, `PARTIALLY_STALE`, `UNVERIFIABLE`, or `SKIPPED` (claims triage skipped; their verdict is `null`). `entity`: exact, case-insensitive.
- `q` (lists and search): every word must match a word in the text; words of 3+ characters match as prefixes, so `pyth` finds "Python". Versions and hyphenated names stay whole (`3.11`, `gpt-4o`). An exact record id also matches.
- Search ranks exact-id hits first, then by trigram word similarity, then by recency.

For one record's full detail (a capture's messages, an experiment's judge samples, a report's evidence), the console calls the owning **backend** directly; this API serves lists, search and analytics.

## Schema

Migrations are plain numbered SQL files in [`migrations/`](migrations/), applied in order at startup under an advisory lock and recorded in `schema_migrations`. Add a new file to change the schema; never edit an applied one.

| Table | Key | Notes |
| :--- | :--- | :--- |
| `rl_captures` | `(id, ts)` | **Range-partitioned by month on `ts`**. The sync worker creates the partitions a batch needs (`rl_ensure_capture_partitions`); the migration creates the current month ± 3. Typed columns for everything filtered or aggregated; `messages` and `params` as `jsonb`; `response_text` is the assistant text extracted from the response. |
| `rl_experiments` | `id` | typed summary columns, the full API object in `raw jsonb` |
| `rl_experiment_results` | `(experiment_id, candidate_model)` | every metric, the verdict (from the result, else the recommendation) and its confidence, nullable judge metrics |
| `sad_reports` | `report_id` | summary and usage counts, the full report in `raw jsonb` |
| `sad_claims` | `(report_id, claim_id)` | verdict (null when skipped), scores, evidence and official-evidence counts |
| `sync_state` | `source` | cursor, `last_success_at`, `last_error`, `rows_upserted` per stream |
| `mv_rl_daily`, `mv_rl_verdicts`, `mv_sad_daily` | unique `(day, …)` | dashboard rollups, refreshed `CONCURRENTLY` after each cycle that wrote rows |

Indexes ([`002_indexes.sql`](migrations/002_indexes.sql)) exist for the read paths above and no others: each list's keyset order (`sort key DESC, id DESC`), per-filter composites on captures (`model`, `tag`, `virtual_key`), a partial index for problem captures and one for running experiments, BRIN on `ts`, `lower(entity)` for claims, and GIN full-text indexes for `q` and search. `pg_trgm` supplies the similarity ranking; `pg_stat_statements` is on when the server preloads it.

Postgres is tuned for a read-heavy 2 GB container in [`postgres/postgresql.conf`](postgres/postgresql.conf) (`shared_buffers` 512 MB, `effective_cache_size` 1.5 GB, `work_mem` 24 MB, `random_page_cost` 1.1, `jit` off, `max_wal_size` 4 GB). Scale the first two with the container's RAM.

## How the sync works

One stream per source, each with its own exponential backoff (1 s doubling to 60 s, with jitter). A failed stream is retried when its backoff runs out, even when `SYNC_INTERVAL_MS` is much longer, so a free-tier backend that answers `429`/`502` while it wakes up is caught a few seconds later. The two Replay Lab streams run in sequence and the Stale stream runs beside them, so one backend being down never stops the other; `/api/health` shows the error and the growing lag. Every write is a batched `INSERT … ON CONFLICT DO UPDATE … WHERE row IS DISTINCT FROM`, so running a sync twice changes nothing and reports 0 rows. Each cycle logs one JSON line with the row counts.

- **Captures** (`sync/captures.ts`). The API has no `since` filter, so it pages newest first, 1000 at a time. Until one full pass completes it reads every page (so an interrupted backfill resumes correctly); after that it stops at the first page holding an id it already has, plus one more page of overlap, because captures arriving mid-paging shift the offsets.
- **Experiments** (`sync/experiments.ts`). Upserts listed experiments, newest first, skipping ones stored with a final status, until a page holds only final, known experiments. Then it re-fetches each experiment still stored as `running` until it is final. An experiment and its results are replaced in one transaction.
- **Reports** (`sync/reports.ts`). Reports are immutable: it fetches only unknown ids, writing each report with its claims in one transaction, and stops paging at the first known one.
- **Retention** (`sync/retention.ts`, hourly). Reads `retention_days` from `GET /replaylab/config` and deletes captures older than that, so the copy matches the source. Whole months past the cutoff are dropped as partitions; the boundary month is trimmed with a `DELETE`.

## Tests and benchmark

```bash
npm run typecheck
npm test                                  # unit tests; the integration suite also runs when:
TEST_DATABASE_URL=postgres://insights:insights@localhost:5432/insights_test npm test
```

The integration suite ([`tests/integration.test.ts`](tests/integration.test.ts)) runs the sync worker against fake backends over HTTP and the API against a real Postgres: full copy in one cycle, idempotent second cycle, offsets shifted by new captures, a running experiment updated to final, Replay Lab still syncing while Stale returns 503, keyset paging with no gaps or repeats, filters, search, rollups and error codes. It wipes the test database's `public` schema first, so it only runs against a database whose name contains `test`. CI runs it on every pull request (the `Insights database (Postgres)` job).

The benchmark seeds **1,000,000 captures** over 90 days, **5,000 experiments** (3 candidates each) and **50,000 reports** with 250,000 claims, then calls every endpoint 200 times:

```bash
npm run seed                              # ~40 s; `npm run seed -- --clean` removes the synthetic rows
npm run api &                             # or `docker compose up`
npm run bench                             # [baseUrl] [iterations]
```

Results on a laptop (Windows 11, Docker Desktop, Postgres 17.6 with the config above; 1,001,250 captures). **Every endpoint meets the p95 < 50 ms target.**

```
endpoint                                     p50 ms   p95 ms   p99 ms
GET /api/health                                 1.5      2.1      2.9
GET /api/insights/overview                     25.7     32.4     45.6
GET /api/insights/replaylab/daily               2.0      2.9      3.6
GET /api/insights/replaylab/verdicts            2.3      2.9      4.4
GET /api/insights/stale/daily                   1.6      2.2      2.5
GET /api/replaylab/captures                     2.7      3.5      5.4
GET /api/replaylab/captures (model)             2.6      3.1      4.4
GET /api/replaylab/captures (tag+streamed)      2.7      3.4      5.4
GET /api/replaylab/captures (problem)           2.6      3.0      3.5
GET /api/replaylab/experiments                  3.4      4.3      9.0
GET /api/replaylab/experiments (q)             15.8     17.0     21.4
GET /api/stale/reports                          1.8      2.2      3.2
GET /api/stale/reports (q)                      7.4      8.2    148.3
GET /api/stale/claims                           1.9      2.4      3.7
GET /api/stale/claims (entity)                 18.5     22.2     24.1
GET /api/search                                22.6     25.6    442.1

200 requests per endpoint against http://127.0.0.1:8093. Target p95 < 50 ms: all met.
```

The two p99 outliers are single slow requests out of 200 (a common prefix such as `e` on a cold plan); p95 holds.

### Slowest queries

**Overview, last 24 hours of captures** (~12,400 rows aggregated on each call; pruned to one partition):

```
 Aggregate (actual time=32.658..32.661 rows=1 loops=1)
   Buffers: shared hit=11378
   ->  Append (actual time=0.050..30.232 rows=12415 loops=1)
         Subplans Removed: 3
         ->  Index Scan using rl_captures_2026_11_ts_id_idx on rl_captures_2026_11 rl_captures_1 (actual time=0.049..29.270 rows=12415 loops=1)
               Index Cond: (ts >= (now() - '24:00:00'::interval))
 Planning Time: 2.291 ms
 Execution Time: 32.722 ms
```

**Search, claims branch** (`q=python`: 50,000 matching claims; the 200 newest are taken off the keyset index and ranked by similarity):

```
 Limit (actual time=15.623..17.884 rows=20 loops=1)
   ->  Sort (actual time=15.623..17.881 rows=20 loops=1)
         Sort Key: (GREATEST(word_similarity('python'::text, c.text), word_similarity('python'::text, c.entity))) DESC, c.checked_at DESC
         ->  Subquery Scan on c (actual time=12.308..17.815 rows=200 loops=1)
               ->  Limit (actual time=12.282..16.895 rows=200 loops=1)
                     ->  Gather Merge (actual time=12.281..16.880 rows=200 loops=1)
                           Workers Launched: 2
                           ->  Parallel Index Scan using sad_claims_checked_idx on sad_claims (actual time=0.293..8.953 rows=144 loops=3)
                                 Filter: (to_tsvector('simple'::regconfig, ((text || ' '::text) || entity)) @@ '''python'':*'::tsquery)
 Execution Time: 18.208 ms
```

**Claims by entity** (`entity=Python&claim_type=version`):

```
 Limit (actual time=0.064..0.381 rows=51 loops=1)
   Buffers: shared hit=55
   ->  Index Scan using sad_claims_entity_idx on sad_claims (actual time=0.063..0.375 rows=51 loops=1)
         Index Cond: (lower(entity) = 'python'::text)
         Filter: (claim_type = 'version'::text)
 Execution Time: 0.397 ms
```

Design note: trigram indexes (GIN, then GiST with similarity-ordered scans) were tried first for search. On data where one phrase repeats tens of thousands of times, a near-miss query ("node lts") made either index recheck ~50,000 rows (250-500 ms). Full-text indexes match whole words, so their work is bounded by real matches; trigram similarity still ranks the short list.

## Deploy (Render, free tier)

[`../../render.yaml`](../../render.yaml) describes it: a free **Postgres 17** (`insights-db`) and a Docker web service (`insights-api`, `all` mode) that builds only when `backend/database/**` changes and deploys only after CI passes. In the Render dashboard: **New → Blueprint** on this repo (or create the two resources by hand with those settings), then set `VITE_INSIGHTS_URL` to the service's URL in the console's Vercel project and redeploy the console.

- `SYNC_INTERVAL_MS` is 20 minutes there: free services sleep after 15 idle minutes, and polling more often would keep both backends awake around the clock. Every start syncs at once, so data is fresh when the service wakes.
- Render's free Postgres expires 30 days after creation; upgrade it or recreate it (the sync refills it from the backends on the next start).
- Render wipes the backends' SQLite on every redeploy. The copy here keeps what it already synced, except that captures still follow Replay Lab's retention window.

## Known limitations

- **Captures have no `since` filter**, so each cycle re-reads at least one page (1000 captures) to find where it left off. Cheap at this scale; a `since` parameter in Replay Lab would make it constant.
- **Deletions at the source are not mirrored**, except captures past retention. A capture or report deleted by hand in a backend stays here. An experiment that vanishes while `running` is marked `failed` with `stop_reason` "missing at source".
- **A backend whose store was wiped** (a Render redeploy) keeps its old rows here; new ones sync normally because ids are unique.
- **`captures_total`** comes from the daily rollup, so it can trail by one sync cycle.
- Search reads the 200 newest matches per kind before ranking, so on very common words an older, closer match can be left out.
