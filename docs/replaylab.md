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

Streaming requests (`"stream": true`) are captured too. The proxy relays each upstream chunk to the client as it arrives, byte for byte, and parses the SSE stream on the side, so capture adds no delay. It never modifies the request: in particular it does not add `stream_options.include_usage`. When the stream carries a usage chunk (the client asked for one), its token counts are stored. Otherwise tokens are estimated from content length (about 4 bytes per token) and the capture has `"usage_estimated": true`. See [Streamed captures](#streamed-captures).

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

The loadable plugin also reports `"target_gateway_version"`, the bifrost-http release it was built for (for example `"v1.6.11"`).

Returns `503` with `"status": "degraded"` if storage is unreachable.

`GET /replaylab/metrics` returns Prometheus text with these counters: `bifrost_replaylab_captures_total`, `_captures_dropped_total`, `_capture_write_errors_total`, `_captures_stored`, `_replays_total`, `_replays_failed_total`, `_experiments_active`, `_stream_captures_total` (streamed captures stored) and `_stream_captures_truncated_total` (streams cut off at `max_stream_capture_bytes`).

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
| `max_stream_capture_bytes` | int, assistant content kept per stream; 0 = default 262144 (256 KiB). Longer streams are stored as `truncated` | yes |
| `db_path`, `gateway_url`, `max_queue_size`, `worker_count`, `api_listen_addr` | | startup only (ignored on PUT) |

## Models

`GET /replaylab/models` returns `[{ "model": "gpt-4o", "input_per_1m_tokens": 2.5, "output_per_1m_tokens": 10 }, ...]`. These are the prices used for every cost figure. Provider-prefixed (`openai/gpt-4o`) and dated (`gpt-4o-2024-08-06`) names resolve to these entries.

## Captures

`GET /replaylab/captures?model=gpt-4o&tag=json_extraction&virtual_key=vk_1&streamed=true&limit=50&offset=0` returns newest first. `limit` is 1-1000 (default 50). `streamed` is `true` (streamed captures only), `false` (non-streamed only) or omitted / `any` (both); anything else is a `400`.

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
  "hash_only": false,
  "completion_state": "complete",
  "usage_estimated": false
}
```

A streamed capture also carries `time_to_first_token_ms`:

```json
{
  "is_streaming": true,
  "latency_ms": 840,
  "time_to_first_token_ms": 95,
  "completion_state": "complete",
  "usage_estimated": true
}
```

`response_json` holds the assistant's message content, not the raw provider envelope.

### Streamed captures

These fields are additive; older captures read as `"completion_state": "complete"`, `"usage_estimated": false` and no `time_to_first_token_ms`.

| Field | Meaning |
| :--- | :--- |
| `is_streaming` | `true` when the client requested `"stream": true` |
| `response_json` | for a stream, the content deltas concatenated (assistant text only; tool-call and reasoning deltas are not kept) |
| `latency_ms` | for a stream, total duration from request to the end of the stream |
| `time_to_first_token_ms` | streams only: time to the first non-empty content delta. Omitted when no content arrived |
| `usage_estimated` | `true` when the stream carried no usage and tokens (and `cost_estimate`) were estimated from content length |
| `completion_state` | `complete`, `error`, `abandoned` or `truncated` (below) |

| `completion_state` | When | `status_code` |
| :--- | :--- | :--- |
| `complete` | The whole response, or an error that arrived before any content, was recorded | upstream status |
| `error` | The stream failed after some content arrived; `response_json` keeps what arrived and `error_message` says why | `502` (or the provider's error status) |
| `abandoned` | The client disconnected, or the stream never finished (pending streams are swept after 10 minutes) | `499` (`504` for a gateway timeout) |
| `truncated` | The content exceeded `max_stream_capture_bytes`; `response_json` keeps the first part, cut on a UTF-8 boundary | upstream status |

Only `complete` captures with a `2xx` status are replayed. Streamed captures are replayed as ordinary non-streaming requests (`"stream": false`), so candidates are scored on the complete answer.

## Experiments

### Start

`POST /replaylab/experiments`

```json
{
  "name": "Cost reduction trial",
  "original_model": "gpt-4o",
  "candidate_models": ["gpt-4o-mini", "claude-3-haiku"],
  "filter_tag": "json_extraction",
  "streamed": true,
  "sample_size": 50,
  "budget_cap_dollars": 10.0,
  "fixed_temperature": 0.0,
  "monthly_request_volume": 2000000,
  "dry_run": false,
  "judge": {
    "model": "gpt-4o",
    "rubric": "Factual accuracy matters more than tone.",
    "max_judge_samples": 30
  }
}
```

| Field | Rules |
| :--- | :--- |
| `candidate_models` | required, 1-10 models, duplicates removed |
| `original_model` | optional; empty means "any model" and the first capture's model is used as the baseline |
| `streamed` | optional; `true` samples only streamed captures, `false` only non-streamed. Omitted samples both. Savings projections always count all of the baseline model's traffic, streamed or not |
| `sample_size` | default 50, max 1000; the most recent matching captures are used |
| `budget_cap_dollars` | default from config; the run stops before any replay that would exceed it |
| `fixed_temperature` | 0-2; default is the captured temperature, else 0 |
| `monthly_request_volume` | optional; default extrapolates from the captured traffic rate |
| `dry_run` | `true` returns a cost estimate (`200`) without calling any model |
| `judge` | optional; omit it to score with similarity only (today's behaviour). See [Judge](#judge) |
| `judge.model` | required when `judge` is set; must not be one of `candidate_models` (`400`) |
| `judge.rubric` | optional, at most 500 characters (`400` if longer); what the judge should weigh. Default: "Correctness first, then completeness and usefulness. Ignore differences in wording, ordering and length that do not change the answer's quality." |
| `judge.max_judge_samples` | optional, `>= 0`; judge only the first N successful replays per candidate. `0` or omitted judges all of them |

A real run returns `202` with the experiment in status `running`. A dry run returns `200`, with `spent_dollars` holding the estimate and `stop_reason` set if the estimate exceeds the cap. With a judge, a dry run also splits the estimate into `estimated_replay_dollars` and `estimated_judge_dollars` (they sum to `spent_dollars`). The judge estimate assumes every replay succeeds and is judged, up to `max_judge_samples`.

### Judge

When `judge` is set, every successful replay (up to `max_judge_samples` per candidate) is judged against the baseline response for the same prompt. The judge is asked twice, with the baseline as response A and then as response B, at temperature 0 with a 256-token reply cap. The prompt and both responses are passed as fenced, untrusted blocks.

- Each order scores 100 when the candidate wins or ties, 0 when it loses. A replay's judge score is the mean of its two orders: 0, 50 or 100.
- A **self-disagreement** is a replay where the two orders picked opposite winners.
- A judge call that errors or returns anything but `{"winner": "A" | "B" | "tie", ...}` leaves that replay unjudged. It keeps its similarity score and the experiment carries on.
- Judge calls go through the same gateway and count against `budget_cap_dollars`. The check before each replay includes both projected judge calls, and each judge call is skipped if a conservative estimate of its cost could cross the cap. In that case the run stops with `budget_exhausted`.
- **Unknown judge prices:** a judge model missing from the price book is accepted, but its cost (and the budget check) uses the fallback rate of $1.00 in / $3.00 out per 1M tokens. The experiment then has `"judge_price_estimated": true` and the report shows a warning.

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
  "judge": {"model": "gpt-4o", "rubric": "Factual accuracy matters more than tone.", "max_judge_samples": 30},
  "judge_spent_dollars": 0.0011,
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
      "judge_score": 91.0,
      "judge_wins": 12,
      "judge_ties": 75,
      "judge_losses": 13,
      "judge_coverage": 100,
      "judge_disagreement_rate": 6,
      "judge_spent_dollars": 0.0006,
      "judge_samples": [
        {
          "capture_id": "cap-...",
          "prompt_excerpt": "user: Summarise this ticket ...",
          "baseline_excerpt": "The customer reports ...",
          "candidate_excerpt": "Customer says ...",
          "judge_score": 100,
          "orders": [
            {"baseline_position": "A", "winner": "tie", "candidate_outcome": "tie", "reason": "Both summaries cover the issue."},
            {"baseline_position": "B", "winner": "A", "candidate_outcome": "win", "reason": "Response A is more precise."}
          ]
        }
      ],
      "recommendation": "SAFE_TO_SWITCH",
      "reasoning": "Judge rates the candidate as good as the baseline (judge score 91.0/100); Latency improved by 42.7%; Cost savings of 94.1%",
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

- `status`: `running`, `completed`, `cancelled`, `budget_exhausted` or `failed`. The last four are terminal. A `budget_exhausted` or `cancelled` run keeps the results for every replay that ran. Runs live in memory, so a run that is still `running` when Replay Lab stops (a gateway restart, a crash) can't resume. On the next start it becomes `failed`, with `stop_reason` starting `interrupted:`, and keeps the progress saved so far.
- `json_validity_rate` is only meaningful when `json_samples > 0`. Show "n/a" otherwise.
- `confidence` grows with sample size (`n/(n+20)`, reduced by the error rate). It is not a statistical significance value.
- `avg_quality_score` is the **similarity score**: lexical and structural overlap with the baseline response (0-100). It is always reported, with or without a judge.

Judge fields. All are omitted when the experiment has no judge, so "not judged" is never confused with "judged as 0".

| Field | Meaning |
| :--- | :--- |
| `judge` | the judge configuration used, with the default rubric filled in |
| `judge_price_estimated` | experiment; `true` when the judge model has no known price (fallback rate used). Omitted when `false` |
| `judge_spent_dollars` | experiment and per candidate; judge spend, already included in `spent_dollars` |
| `estimated_replay_dollars`, `estimated_judge_dollars` | dry runs with a judge only; the split of the `spent_dollars` estimate |
| `judge_score` | 0-100, mean per-replay judge score. Omitted when no replay was judged |
| `judge_wins`, `judge_ties`, `judge_losses` | judgements (one per order, so two per judged replay) the candidate won, tied or lost |
| `judge_coverage` | 0-100, judged replays / successful replays. `max_judge_samples` lowers it on purpose |
| `judge_disagreement_rate` | 0-100, judged replays whose two orders picked opposite winners. Omitted when no replay was judged |
| `judge_samples` | up to 5 judged replays per candidate for auditing: excerpts (300 characters) of the prompt and both responses, and each order's winner, candidate outcome and reason |

### Verdict rules

| Verdict | When |
| :--- | :--- |
| `DO_NOT_SWITCH` | quality < 70, error rate > 5%, JSON validity < 70%, or no successful replays |
| `SWITCH_WITH_CAUTION` | quality 70-85, JSON validity < 95%, latency > +40%, more expensive, or fewer than 20 successful replays |
| `SAFE_TO_SWITCH` | otherwise |

"Quality" is the judge score when the candidate has one, else the similarity score. Judge scores use their own thresholds: `DO_NOT_SWITCH` below 60, `SWITCH_WITH_CAUTION` from 60 to 80. With a judge, two more rules cap the verdict at `SWITCH_WITH_CAUTION`: judge coverage below 80%, and judge disagreement above 30%. All other rules apply unchanged.

### Live progress (SSE)

`GET /replaylab/experiments/{id}/events`

The first event is the current state. If the experiment has already finished, that first event is final and the stream ends. The server sends a `: keep-alive` comment every 15 seconds.

```
event: progress
data: {"experiment_id":"exp-...","status":"running","candidate_model":"gpt-4o-mini","completed_count":12,"total_count":100,"progress_percent":12,"current_quality":93.1,"spent_dollars":0.0004,"message":"Replayed cap-... -> gpt-4o-mini","final":false}
```

`current_quality` is the running similarity score. With a judge, events also carry `current_judge_score`, the current candidate's running judge score, once one of its replays has been judged.

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
- `GET /replaylab/experiments/{id}/report?format=markdown` returns `text/markdown`. Without `format`, it returns the experiment JSON. With a judge, the Markdown report names the judge model, rubric and judge spend, and adds judge score, W/T/L, disagreement and coverage columns.
