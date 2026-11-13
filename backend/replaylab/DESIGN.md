# Replay Lab design

This note explains why Replay Lab exists and how it's built. For usage, see the [README](README.md). For the API, see [docs/replaylab.md](docs/replaylab.md).

## The gap in Bifrost

Bifrost ships plugins for:
- wire compatibility (`compat`)
- governance: budgets, rate limits and virtual keys
- observability: `logging`, `telemetry` and `otel`
- mocking (`mocker`)
- semantic caching (`semanticcache`)
- routing

None of them answers the question operators ask before changing a route: *"Can I move this traffic from `gpt-4o` to a cheaper model without breaking production?"* Today that takes guesswork or one-off scripts. Replay Lab answers it with the operator's own recorded traffic and a verdict per candidate.

## Principles

- **Never slow the gateway.** Capture is off the hot path. If the buffer is full, the capture is dropped and counted, and the request goes through untouched.
- **Never spend more than allowed.** Every replay, and every judge call, is checked against the experiment's budget cap first. A run stops with partial results rather than overspend.
- **Never store what shouldn't be stored.** Redaction happens before anything reaches disk, and hash-only mode stores no text at all.
- **One core, several deployments.** Capture, replay, scoring and the API live in `plugins/replaylab`. The SDK plugin, the bifrost-http plugin and the sidecar differ only in wiring.

## Components

1. **Capture** (`bifrost.go`, `proxy.go`, `capture.go`, `stream.go`)
   - In Bifrost, `PreLLMHook` stashes the chat request and `PostLLMHook` pairs it with the response or provider error. Streamed responses are reassembled from their chunks.
   - Records go to a bounded queue drained by a small pool of SQLite writers. The sidecar proxy feeds the same queue.
   - Sampling rate and per-virtual-key opt-in or opt-out apply at capture time.
   - Replays carry `X-Replaylab-Replay`, so they are never captured.
2. **Protect** (`redaction.go`)
   - Redacts email addresses, phone numbers, card numbers and API tokens (`sk-`, `tvly-`, `ghp_`, bearer tokens) before storage.
   - Optionally stores SHA-256 digests instead of text.
   - Captures older than `retention_days` are purged in the background.
3. **Replay** (`replay.go`)
   - Replays a sample of captures against each candidate through the gateway's OpenAI-compatible endpoint, with the original parameters at temperature 0.
   - Supports a dry run that estimates cost before spending anything.
   - Progress is stored as the run goes. A run cut short by a shutdown is marked as interrupted on the next start.
4. **Score** (`scoring.go`, `judge.go`)
   - Deterministic metrics: JSON-contract preservation, lexical similarity, length ratio, latency (p50 and p95), error rate and cost.
   - Optionally, an LLM judge compares each replay with the baseline in both orders, which cancels position bias. Judge calls count against the budget.
5. **Recommend** (`recommender.go`)
   - Gives each candidate `SAFE_TO_SWITCH`, `SWITCH_WITH_CAUTION` or `DO_NOT_SWITCH`.
   - Projects monthly savings from the observed traffic rate.
   - Every threshold is a named constant in this one file. For example, quality ≥ 85 is safe, 70 to 85 means caution, and an error rate above 5% means don't switch.
6. **Serve** (`api.go`, `metrics.go`)
   - A REST API covering health, config, captures, experiments and reports (JSON or Markdown).
   - Live progress over SSE, and Prometheus metrics.

## Deployment modes

| Mode | Wiring | Notes |
| :--- | :--- | :--- |
| SDK plugin | `replaylab.NewPlugin` registered in `LLMPlugins` | Built against Bifrost core v1.15.1 |
| bifrost-http plugin | `loadable/`, a shared object loaded from the gateway config | Pinned to one gateway release; see [ADR-0001](docs/adr/0001-loadable-plugin-target-gateway.md) |
| Sidecar | `cmd/replaylab`, a capturing proxy | Works in front of any OpenAI-compatible gateway |
