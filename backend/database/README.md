# database/ (reserved for the database team)

This folder is where the read-optimized analytics database lands: PostgreSQL, a sync worker that copies data out of the two services over HTTP, and a read-only **Insights API** on port `8090` that the console uses for search, lists and analytics.

The full brief is `ANTIGRAVITY_DATABASE_PROMPT.md` in the team workspace. The parts that keep everyone's work from breaking each other:

## Rules for code in this folder

1. **Don't change `backend/replaylab/`, `backend/stale/` or `frontend/`.** Each service owns its SQLite database, which stays the source of truth. Read them only through their HTTP APIs (`replaylab/docs/replaylab.md`, `stale/docs/API.md`).
2. **The console already speaks your API.** It calls the Insights API at `VITE_INSIGHTS_URL` and expects exactly the contract in section 5.4 of the brief (`/api/health`, `/api/insights/*`, `/api/search`, paged lists with `{ items, next_cursor }`). Change that contract only together with the console's `src/api/insights.ts`.
3. **Until `VITE_INSIGHTS_URL` is set, the console makes no requests here** and shows Insights as "not configured". So you can merge work in progress without breaking the deployed console.
4. **Deploying:** add your services to the root `render.yaml` with a `buildFilter` on `backend/database/**`, so your pushes don't rebuild the other services.
5. No secrets in git. CI fails on any tracked `.env` or `.db` file.

## Expected layout

```
database/
  README.md            what it is, how to run, API contract, benchmark results
  docker-compose.yml   postgres + sync + api
  .env.example         REPLAYLAB_URL, STALE_URL, STALE_API_KEY, DATABASE_URL, SYNC_INTERVAL_MS, API_PORT
  migrations/          001_init.sql, 002_indexes.sql, ...
  sync/                sync worker
  api/                 read API (port 8090)
  bench/               seed + benchmark scripts
```

When this lands, add a `database` job to the root `.github/workflows/ci.yml`.
