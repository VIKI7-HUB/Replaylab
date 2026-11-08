# Bifrost Replay Lab

> **Can I switch from `gpt-4o` to a cheaper model without breaking production?**
> Replay Lab answers that with your own traffic: it captures real gateway requests, replays them against candidate models under a hard budget cap, and returns a `SAFE_TO_SWITCH` / `SWITCH_WITH_CAUTION` / `DO_NOT_SWITCH` verdict with projected monthly savings.

Built for the Bifrost track: Bifrost has routing, caching, governance and logging plugins, but nothing that turns recorded traffic into a model-switch decision (see [DESIGN.md](DESIGN.md)).

## How it works

```mermaid
flowchart LR
    C[Client] -->|/v1/chat/completions| P[Capture proxy]
    P --> G[Bifrost / any OpenAI-compatible gateway]
    P -. async, non-blocking .-> Q[Ring buffer] --> R[PII redaction / hashing] --> DB[(SQLite)]
    DB --> E[Replay engine<br/>hard budget cap] --> G
    E --> S[Scoring: JSON contract,<br/>similarity, latency, cost] --> V[Verdict + report<br/>REST, SSE, Markdown]
```

1. **Capture.** The proxy forwards each request and records it off the hot path. If the buffer is full the capture is dropped and counted; the request is never slowed. Sampling rate and per-key opt-in/opt-out are configurable at runtime.
2. **Protect.** Emails, phone numbers, card numbers and API tokens are redacted before anything reaches disk. Hash-only mode stores SHA-256 digests instead. Captures expire after `retention_days`.
3. **Replay.** Captured prompts are replayed with their original parameters (`max_tokens`, `response_format`, ...) at temperature 0. Before every call the engine checks that the projected cost still fits the budget, and stops with partial results if it doesn't.
4. **Score and decide.** Each candidate is scored on JSON-contract preservation, lexical similarity, latency (p50/p95), error rate and cost. Thresholds live in one place ([recommender.go](plugins/replaylab/recommender.go)). Savings are projected from the traffic rate actually observed.

## Quickstart (no API keys needed)

Requires Go 1.26+.

```bash
go run ./demo/run_experiment
```

This starts a mock provider and Replay Lab in-process. It sends 200 `gpt-4o` requests through the capture proxy (a quarter of them containing synthetic PII), then replays 50 against three candidates. Output from a real run:

| Candidate | Quality | JSON valid | p50 / p95 latency | Cost savings | Verdict |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `gpt-4o-mini` | 94.7 | 100% of 12 | 23ms / 29ms (-43%) | 94.1% | **SAFE TO SWITCH** |
| `claude-3-haiku` | 71.9 | 100% of 12 | 16ms / 19ms (-60%) | 91.6% | *SWITCH WITH CAUTION* |
| `meta-llama-3.1-8b` | 34.9 | 50% of 12 | 11ms / 14ms (-73%) | 99.6% | DO NOT SWITCH |

The mock provider is deterministic and gives each model tier distinct behaviour (rewording, truncation, broken JSON, occasional 503s); see [internal/mockprovider](internal/mockprovider/mockprovider.go). Flags: `-requests`, `-sample`, `-budget`, `-candidates`, `-latency`.

## Run the server

```bash
docker compose up --build          # mock provider on :8081, Replay Lab on :8085
go run ./demo/generate_traffic     # send 200 requests through the proxy
```

Without Docker (Windows): `run.bat mock` in one terminal, `run.bat` in another, then `run.bat traffic`.

Point any OpenAI SDK at `http://localhost:8085/v1` to capture its traffic. To replay through a real gateway, set `GATEWAY_URL` (and `REPLAY_AUTH_TOKEN` if replays need a key other than the captured virtual key).

## API

Full reference with request/response shapes: [docs/replaylab.md](docs/replaylab.md). Postman collection: [demo/replaylab.postman_collection.json](demo/replaylab.postman_collection.json).

| Method | Path | Purpose |
| :--- | :--- | :--- |
| `POST` | `/v1/chat/completions` | Capturing proxy (OpenAI-compatible) |
| `GET` | `/replaylab/health` | Status, stored/dropped captures, active experiments |
| `GET` | `/replaylab/metrics` | Prometheus metrics |
| `GET` / `PUT` | `/replaylab/config` | Read / partially update runtime config |
| `GET` | `/replaylab/models` | Price book used for cost estimates |
| `GET` | `/replaylab/captures`, `/replaylab/captures/{id}` | Browse captures |
| `POST` | `/replaylab/experiments` | Start an experiment (`"dry_run": true` to estimate cost) |
| `GET` | `/replaylab/experiments`, `/replaylab/experiments/{id}` | List / poll experiments |
| `POST` | `/replaylab/experiments/{id}/cancel` | Stop a running experiment |
| `GET` | `/replaylab/experiments/{id}/events` | Live progress (SSE); always ends with `"final": true` |
| `GET` | `/replaylab/experiments/{id}/report?format=markdown` | Shareable report |

## Performance

`go test -bench=. -benchmem ./tests` on an Intel i5-12450H:

| Benchmark | Result |
| :--- | :--- |
| Capture hot path (enqueue only) | ~2.4 µs/op, 946 B/op |
| Enqueue with SQLite writers running on the same machine | ~57 µs/op (~17k captures/s sustained) |

## Testing

```bash
go vet ./...
go test ./tests/...
```

The tests cover redaction, scoring, recommendations, budget-cap enforcement, upstream failures, proxy capture, SSE termination, config updates and pricing lookup. CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) also runs them under the race detector.

## Limitations

- **Quality is a similarity score, not a semantic judgement.** A candidate that answers correctly in different words scores lower than one that paraphrases closely. An LLM-as-judge tier is designed (DESIGN.md) but not implemented.
- **Streaming requests are proxied but not captured.**
- **Embedding inside Bifrost** needs a thin adapter that calls `RecordRequestWithParams` / `RecordResponse` from Bifrost's `PreLLMHook` / `PostLLMHook` and routes `/replaylab/*` through `InterceptHTTPRequest`. That adapter is not in this repo yet; the sidecar proxy is the supported deployment today.
- Prices in the cost book are list prices and should be checked before quoting real savings.
