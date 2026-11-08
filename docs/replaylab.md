# Replay Lab API reference

Base URL (local): `http://localhost:8085`. All `/replaylab/*` responses are JSON unless noted. CORS is open (`*`).

Errors always look like:

```json
{ "error": "candidate_models must list at least one model" }
```

| Status | Meaning |
| :--- | :--- |
| `400` | Malformed JSON, unknown field, or invalid value |
| `404` | Capture or experiment not found |
| `409` | Cancelling an experiment that already finished |
| `422` | Valid request, but nothing to replay (no matching successful captures) |

## Capture proxy

`POST /v1/chat/completions` accepts any OpenAI chat request and forwards it to `GATEWAY_URL`. The response is returned unchanged.

Optional headers:

| Header | Stored as |
| :--- | :--- |
| `X-Replaylab-Tag` | `tag` (use it to group traffic, e.g. `json_extraction`) |
| `X-Replaylab-Tenant` | `tenant` |
| `X-Bf-Vk` | `virtual_key` (falls back to the `Authorization` bearer token) |

Streaming requests (`"stream": true`) are forwarded but not captured.

## Health and metrics

`GET /replaylab/health`

```json
{
  "status": "healthy",
  "service": "bifrost-replaylab",
  "version": "1.1.0",
  "uptime_seconds": 42,
  "stored_captures": 200,
  "dropped_captures": 0,
  "active_experiments": 1,
  "capture_enabled": true
}
```

Returns `503` with `"status": "degraded"` if storage is unreachable.

`GET /replaylab/metrics` returns Prometheus text with these counters: `bifrost_replaylab_captures_total`, `_captures_dropped_total`, `_capture_write_errors_total`, `_captures_stored`, `_replays_total`, `_replays_failed_total` and `_experiments_active`.

## Config

`GET /replaylab/config` returns the config currently in effect.

`PUT /replaylab/config` merges the fields you send. It takes effect immediately and returns the new config.

```json
{ "sampling_rate": 0.2, "redact_pii": true, "opt_out_keys": ["vk_internal"] }
```

| Field | Type | Runtime-editable |
| :--- | :--- | :--- |
| `enabled` | bool | yes |
| `sampling_rate` | 0.0 - 1.0 | yes |
| `opt_in_keys`, `opt_out_keys` | string[] | yes |
| `redact_pii` | bool | yes |
| `store_hashes_only` | bool | yes (hashed captures cannot be replayed) |
| `retention_days` | int, 0 = keep forever | yes |
| `default_budget_cap` | USD | yes |
| `db_path`, `gateway_url`, `max_queue_size`, `worker_count`, `api_listen_addr` | | startup only (ignored on PUT) |

## Models

`GET /replaylab/models` returns `[{ "model": "gpt-4o", "input_per_1m_tokens": 2.5, "output_per_1m_tokens": 10 }, ...]`. These are the prices used for every cost figure. Provider-prefixed (`openai/gpt-4o`) and dated (`gpt-4o-2024-08-06`) names resolve to these entries.

## Captures

`GET /replaylab/captures?model=gpt-4o&tag=json_extraction&virtual_key=vk_1&limit=50&offset=0` returns newest first. `limit` is 1-1000 (default 50).

`GET /replaylab/captures/{id}`

```json
{
  "id": "cap-1a2b3c4d5e6f",
  "request_id": "req-...",
  "timestamp": "2026-11-08T09:00:00Z",
  "virtual_key": "vk_demo_prod",
  "tenant": "acme",
  "provider": "openai",
  "model": "gpt-4o",
  "messages_json": "[{\"role\":\"user\",\"content\":\"Contact [REDACTED_EMAIL] ...\"}]",
  "params_json": "{\"response_format\":{\"type\":\"json_object\"}}",
  "response_json": "{\"name\":\"...\"}",
  "prompt_tokens": 40,
  "completion_tokens": 18,
  "total_tokens": 58,
  "latency_ms": 172,
  "cost_estimate": 0.00028,
  "status_code": 200,
  "is_streaming": false,
  "tag": "json_extraction",
  "hash_only": false
}
```

`response_json` holds the assistant's message content, not the raw provider envelope.

## Experiments

### Start

`POST /replaylab/experiments`

```json
{
  "name": "Cost reduction trial",
  "original_model": "gpt-4o",
  "candidate_models": ["gpt-4o-mini", "claude-3-haiku"],
  "filter_tag": "json_extraction",
  "sample_size": 50,
  "budget_cap_dollars": 10.0,
  "fixed_temperature": 0.0,
  "monthly_request_volume": 2000000,
  "dry_run": false
}
```

| Field | Rules |
| :--- | :--- |
| `candidate_models` | required, 1-10 models, duplicates removed |
| `original_model` | optional; empty means "any model" and the first capture's model is used as the baseline |
| `sample_size` | default 50, max 1000; the most recent matching captures are used |
| `budget_cap_dollars` | default from config; the run stops before any replay that would exceed it |
| `fixed_temperature` | 0-2; default is the captured temperature, else 0 |
| `monthly_request_volume` | optional; default extrapolates from the captured traffic rate |
| `dry_run` | `true` returns a cost estimate (`200`) without calling any model |

A real run returns `202` with the experiment in status `running`. A dry run returns `200`, with `spent_dollars` holding the estimate and `stop_reason` set if the estimate exceeds the cap.

### Experiment object

`GET /replaylab/experiments/{id}` (progress is saved every 10 replays). `GET /replaylab/experiments?limit=20` lists them.

```json
{
  "id": "exp-29ae0198-6",
  "name": "Cost reduction trial",
  "status": "completed",
  "stop_reason": "",
  "original_model": "gpt-4o",
  "candidate_models": ["gpt-4o-mini", "claude-3-haiku"],
  "sample_size": 50,
  "budget_cap_dollars": 10,
  "monthly_request_volume": 6000,
  "total_replays": 100,
  "completed_replays": 100,
  "spent_dollars": 0.0028,
  "spent_tokens": 9100,
  "created_at": "...",
  "completed_at": "...",
  "results": [
    {
      "candidate_model": "gpt-4o-mini",
      "total_requests": 50,
      "successful_requests": 50,
      "failed_requests": 0,
      "error_rate": 0,
      "avg_quality_score": 94.7,
      "json_validity_rate": 100,
      "json_samples": 12,
      "p50_latency_ms": 23,
      "p95_latency_ms": 29,
      "original_avg_latency_ms": 41.2,
      "candidate_avg_latency_ms": 23.6,
      "latency_delta_percent": -42.7,
      "original_cost_dollars": 0.0283,
      "candidate_cost_dollars": 0.0017,
      "cost_savings_dollars": 0.0266,
      "cost_savings_percent": 94.1,
      "recommendation": "SAFE_TO_SWITCH",
      "reasoning": "High quality retention (94.7/100); Latency improved by 42.7%; Cost savings of 94.1%",
      "projected_monthly_savings": 3.19
    }
  ],
  "recommendations": [
    {
      "original_model": "gpt-4o",
      "candidate_model": "gpt-4o-mini",
      "verdict": "SAFE_TO_SWITCH",
      "confidence": 0.71,
      "projected_monthly_savings": 3.19,
      "reasoning": "..."
    }
  ],
  "summary_markdown": "# Bifrost Replay Lab: Experiment Report ..."
}
```

- `status`: `running`, `completed`, `cancelled`, `budget_exhausted` or `failed`. The last four are terminal. A `budget_exhausted` or `cancelled` run keeps the results for every replay that ran.
- `json_validity_rate` is only meaningful when `json_samples > 0`. Show "n/a" otherwise.
- `confidence` grows with sample size (`n/(n+20)`, reduced by the error rate). It is not a statistical significance value.

### Verdict rules

| Verdict | When |
| :--- | :--- |
| `DO_NOT_SWITCH` | quality < 70, error rate > 5%, JSON validity < 70%, or no successful replays |
| `SWITCH_WITH_CAUTION` | quality 70-85, JSON validity < 95%, latency > +40%, more expensive, or fewer than 20 successful replays |
| `SAFE_TO_SWITCH` | otherwise |

### Live progress (SSE)

`GET /replaylab/experiments/{id}/events`

The first event is the current state. If the experiment has already finished, that first event is final and the stream ends. The server sends a `: keep-alive` comment every 15 seconds.

```
event: progress
data: {"experiment_id":"exp-...","status":"running","candidate_model":"gpt-4o-mini","completed_count":12,"total_count":100,"progress_percent":12,"current_quality":93.1,"spent_dollars":0.0004,"message":"Replayed cap-... -> gpt-4o-mini","final":false}
```

The last event has `"final": true` and a terminal `status`. Close the `EventSource` when you receive it.

```js
const es = new EventSource(`/replaylab/experiments/${id}/events`);
es.addEventListener("progress", (e) => {
  const ev = JSON.parse(e.data);
  render(ev);
  if (ev.final) es.close();
});
```

### Cancel and report

- `POST /replaylab/experiments/{id}/cancel` returns `202 {"cancelled": true, "experiment_id": "..."}`, or `409` if the experiment already finished.
- `GET /replaylab/experiments/{id}/report?format=markdown` returns `text/markdown`. Without `format`, it returns the experiment JSON.
