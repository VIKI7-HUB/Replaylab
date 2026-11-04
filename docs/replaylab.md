# Bifrost Replay Lab

> **A Feature the Open-Source Bifrost Gateway Is Missing**  
> Record production LLM gateway traffic, replay it against cheaper or faster candidate models, enforce hard budget caps, and generate automated "Safe-to-Switch" ROI recommendations.

---

## The Problem
LLM infrastructure operators face a high-stakes question:
> *"Can I migrate from GPT-4o to GPT-4o-mini, Claude 3.5 Haiku, or a local fine-tuned model without hurting output quality or breaking JSON formatting for our users?"*

Today, engineers either manually test prompts or make blind production model changes and wait for user complaints. **Bifrost Replay Lab** solves this by providing automated, budget-capped shadow testing directly inside the Bifrost gateway.

---

## Core Capabilities

### 1. Asynchronous, Non-Blocking Capture
- Hooks into `PreLLMHook` and `PostLLMHook` with negligible latency (< 15 microseconds per request).
- Ring-buffered channel with dedicated worker pools flushes records in atomic SQLite batches.
- Saturation protection: drops incoming records with an atomic counter `dropped_captures_total` if the buffer fills, guaranteeing the gateway hot path is never blocked.

### 2. Privacy by Design
- **PII Scrubbing**: Built-in regex redactor strips emails, phone numbers, credit card numbers, and API tokens (`sk-`, `tvly-`, `ghp_`, Bearer tokens) *before* data hits the database.
- **Sampling Rate**: Configurable 0.0 to 1.0 (e.g. sample only 10% of traffic).
- **Virtual Key Scoping**: Explicit opt-in and opt-out key lists.
- **Hash-Only Mode**: Option to store SHA256 hashes only.
- **TTL Retention**: Automatic background purge of captures past the retention horizon.

### 3. Budget-Capped Replay Engine
- Selects captured prompts filtered by original model, tags, date range, or sample size.
- Dispatches prompts to candidate models with concurrency control and pacing.
- **Hard Budget Cap**: Monitors spent dollars in real time; halts execution immediately if the dollar or token cap is reached.
- **Dry-Run Estimation**: Calculates projected cost before running live inferences.

### 4. Objective Quality & Recommendation Scoring
- Evaluates:
  - **JSON Schema Preservation**: Verifies that structured JSON outputs remain parseable and retain required top-level keys.
  - **Length Ratio**: Flags truncated completions or runaway hallucinations.
  - **Lexical/Jaccard Similarity**: Measures content preservation.
  - **Latency Percentiles**: Computes p50 and p95 latency deltas.
  - **Cost Delta**: Calculates exact dollar savings and percentage reductions.
- Emits an actionable verdict:
  - `SAFE_TO_SWITCH`: Quality preserved ($\ge 85\%$), latency stable, positive savings.
  - `SWITCH_WITH_CAUTION`: Minor quality or latency compromises ($70\% \le \text{Score} < 85\%$).
  - `DO_NOT_SWITCH`: Noticeable degradation ($< 70\%$) or errors detected.
- Extrapolates **Projected Monthly Savings** based on observed production volume.

---

## Configuration Reference

```json
{
  "enabled": true,
  "sampling_rate": 0.20,
  "redact_pii": true,
  "store_hashes_only": false,
  "retention_days": 30,
  "max_queue_size": 10000,
  "worker_count": 4,
  "db_path": "replaylab.db",
  "gateway_url": "http://localhost:8080",
  "default_budget_cap": 50.0
}
```

---

## Integration with Bifrost Gateway

Add Replay Lab to your Bifrost configuration:

```go
cfg := replaylab.DefaultConfig()
replayPlugin, err := replaylab.NewPlugin(cfg)
if err != nil {
    log.Fatal(err)
}

bifrostConfig := &schemas.BifrostConfig{
    Plugins: []schemas.Plugin{
        replayPlugin,
    },
}
```

When deployed, the plugin intercepts `/replaylab/*` routes via `HTTPTransportPreHook` to serve REST management endpoints and Server-Sent Event progress streams directly through the gateway.
