# Bifrost Replay Lab: Design Note (Step 0)

## 1. Existing Feature Audit
We conducted a comprehensive audit of `maximhq/bifrost`'s codebase:
- Checked existing plugins in `/plugins`:
  - `compat`: OpenAI/Anthropic/Gemini wire compatibility translation.
  - `governance`: Budget controls, rate limits, virtual key authentication.
  - `logging` / `telemetry` / `otel`: Standard observability and structured request/response export.
  - `mocker`: Mocking canned responses for development.
  - `semanticcache`: Cache lookups via hash matching and vector embeddings.
  - `routing`: Rule-based and latency/cost-based routing.
- **Finding**: Bifrost has **no feature** for recording production traffic and replaying it against candidate models to quantify latency, cost, and quality differentials. Operators currently have to guess or manually stitch custom scripts to answer: *"Can I safely switch my route from GPT-4o to GPT-4o-mini or Claude 3.5 Haiku?"*

## 2. Bifrost Plugin Hook Signatures
From `core/schemas/plugin.go`:
- `BasePlugin`:
  - `GetName() string`
  - `Cleanup() error`
- `LLMPlugin`:
  - `PreRequestHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) error`
  - `PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error)`
  - `PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error)`
- `HTTPTransportPlugin`:
  - `HTTPTransportPreHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error)`
  - `HTTPTransportPostHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest, resp *schemas.HTTPResponse) error`

## 3. Chosen Architecture: Dual-Mode Embedded & Sidecar

> Status: both modes are implemented. Embedded: `replaylab.Plugin` implements `schemas.LLMPlugin` and `schemas.HTTPTransportPlugin` (Bifrost core v1.15.1, which added `PreRequestHook` and `HTTPTransportPreAuthHook` to the signatures below); see `plugins/replaylab/bifrost.go` and `tests/bifrost_plugin_test.go`. Sidecar: the capturing proxy in `cmd/replaylab`.
We designed `Replay Lab` to support both native embedded execution and decoupled sidecar deployment:
1. **Asynchronous Non-Blocking Capture**:
   - `PreLLMHook` captures request timestamp and metadata, stashing a lightweight context snapshot.
   - `PostLLMHook` records response, token counts, latency, and cost.
   - Payloads are pushed to a ring-buffered channel (`chan *CaptureRecord`).
   - If the ring buffer fills up, incoming requests are dropped with an atomic counter `dropped_captures_total` without stalling the gateway hot path (measured ~2.4 µs per capture; see README).
   - A dedicated worker pool performs regex PII redaction and batch SQLite commits.
2. **Privacy-by-Design Redaction**:
   - High-throughput regex scrubbers wipe:
     - Email addresses (`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
     - Phone numbers (`(\+?\d{1,3}[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}`)
     - API Keys & Tokens (`sk-[a-zA-Z0-9]{20,}`, `tvly-[a-zA-Z0-9_-]{20,}`, Bearer tokens)
     - Credit Card numbers (Luhn/regex)
   - Configurable sampling rate (0.0–1.0) and virtual-key opt-in/opt-out.
   - Configurable TTL retention policy with background auto-purge.
3. **Budget-Capped Replay Engine**:
   - Reads captured prompts matching filters (model, date range, tags, sample size).
   - Re-sends prompts via HTTP client to candidate models/providers through Bifrost's OpenAI-compatible endpoint.
   - Enforces a strict dollar budget cap: before each replay, the projected cost is checked against the remaining budget; the run stops with partial results rather than overspend.
   - Provides dry-run cost estimation before executing.
4. **Scoring & Recommendation Engine**:
   - Dual-tier scoring:
     - Tier 1 (Deterministic/Cheap): Response length ratio, JSON structural validity preservation, Lexical/Jaccard similarity.
     - Tier 2 (LLM-as-a-Judge, **planned, not implemented**): judge model evaluation with positional swap (A/B vs B/A) to cancel position bias.
   - Generates automated actionable verdicts:
     - `SAFE_TO_SWITCH`: Quality preserved ($\ge 85\%$), latency stable, positive savings.
     - `SWITCH_WITH_CAUTION`: Marginal quality or latency trade-offs ($70\% \le \text{Quality} < 85\%$).
     - `DO_NOT_SWITCH`: Severe degradation ($< 70\%$) or errors detected.
   - Calculates projected monthly savings based on observed volume.
5. **API & UI-Ready SSE**:
   - Exposes REST endpoints (`/replaylab/config`, `/replaylab/captures`, `/replaylab/experiments`, `/replaylab/metrics`, `/replaylab/health`).
   - Streams live progress via Server-Sent Events (`/replaylab/experiments/{id}/events`).
   - Returns both machine-readable JSON reports and formatted markdown tables (`?format=markdown`).
