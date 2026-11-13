# LLM Reliability: backend

Every backend service for the LLM Reliability Console, in one repo. Each service lives in its own folder, builds on its own and deploys on its own, so a change in one can't break another.

| Folder | Service | Stack | Docs |
| :--- | :--- | :--- | :--- |
| [`replaylab/`](replaylab/) | **Bifrost Replay Lab**: captures gateway traffic, replays it against cheaper models under a budget cap, and returns a switch verdict with projected savings | Go, Bifrost plugin | [README](replaylab/README.md), [API](replaylab/docs/replaylab.md) |
| [`stale/`](stale/) | **Stale-Answer Detector**: checks the claims in an LLM answer against the live web with Tavily and returns a scored report and a corrected answer | Python, FastAPI | [README](stale/README.md), [API](stale/docs/API.md) |
| [`database/`](database/) | **Insights** read database and API (in progress, database team) | PostgreSQL, TypeScript | [README](database/README.md) |

The console (frontend) is a separate repo. It talks to these services only over HTTP, so frontend redesigns never touch this repo.

## The contract with the frontend

The console depends on these, and nothing else:

- **Replay Lab**: the `/replaylab/*` endpoints and the capturing proxy `POST /v1/chat/completions` ([docs/replaylab.md](replaylab/docs/replaylab.md)).
- **Stale Detector**: the `/v1/*` endpoints ([docs/API.md](stale/docs/API.md)).
- **Insights**: the `/api/*` endpoints in [database/README.md](database/README.md).
- CORS is open on every service, and no service needs a key from the browser.

Adding fields to a response is safe. Renaming or removing one, or changing an endpoint path, breaks the console: change the console's `src/api/*.ts` in the same go.

## Run locally (no API keys)

```bash
cd replaylab
go run ./cmd/mockprovider                                         # mock OpenAI-compatible provider on :8081
go run ./cmd/replaylab -port=8086 -gateway=http://localhost:8081  # Replay Lab API and capture proxy on :8086
```

```bash
cd stale
cp .env.example .env            # add TAVILY_API_KEY (and LLM_API_KEY for LLM verification)
pip install -r requirements.txt
uvicorn app.main:app --port 8001
```

## Test

```bash
cd replaylab && go vet ./... && go test ./tests/...
cd stale && pytest -q
```

CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) runs a secret scan, then each service's tests in its own job. `benchmark.yml` runs the Stale Detector's labeled benchmark against a deployed API, by hand.

## Deploy

[`render.yaml`](render.yaml) is a Render Blueprint with one service per folder. Each service has a `buildFilter`, so a push rebuilds only the services whose folder changed. Secrets (`GEMINI_API_KEY`, `TAVILY_API_KEY`, `LLM_API_KEY`) are set in Render's dashboard, never in git.
