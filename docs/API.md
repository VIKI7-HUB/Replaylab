# Stale-Answer Detector API reference

Base URL (local): `http://localhost:8000`. Interactive docs: `/docs`. CORS is open. If `API_KEY` is set, send `X-API-Key: <key>` on every `/v1` request (otherwise `401`).

Validation errors return `422` (FastAPI format). `/v1/ask` returns `503` when no LLM is configured.

## POST /v1/check

```json
{
  "answer": "OpenAI GPT-4 costs $0.03 per 1K input tokens. Python 3.9 is the latest release.",
  "question": "optional original question",
  "answer_date": "2023-06-01",
  "options": {
    "max_claims": 10,
    "depth": "advanced",
    "domains_allow": ["openai.com"],
    "domains_block": ["reddit.com"],
    "force_fresh": false
  }
}
```

| Field | Rules |
| :--- | :--- |
| `answer` | 5 - 20,000 characters |
| `answer_date` | `YYYY-MM-DD`; answers older than a year get a higher staleness risk |
| `options.max_claims` | 1 - 30 |
| `options.depth` | `basic` or `advanced` |
| `options.domains_allow` | replaces the default domain restriction for every claim |
| `options.force_fresh` | skip the cache |

### Response (CheckReport)

```json
{
  "report_id": "rpt-1a2b3c4d5e",
  "checked_at": "2026-11-08T09:00:00+00:00",
  "overall_freshness_score": 22.4,
  "verification_mode": "llm",
  "summary": { "current": 0, "stale": 2, "partially_stale": 0, "unverifiable": 0, "skipped": 0, "total_claims": 2 },
  "claims": [
    {
      "id": "claim-1",
      "text": "OpenAI GPT-4 costs $0.03 per 1K input tokens.",
      "entity": "OpenAI",
      "claim_type": "pricing",
      "quoted_span": { "start": 0, "end": 46 },
      "staleness_risk": 0.88,
      "risk_reasons": ["Domain volatility prior (pricing): 0.88"],
      "skipped": false,
      "skip_reason": null,
      "verdict": "STALE",
      "freshness_score": 4.6,
      "score_breakdown": {
        "verdict_score": 0, "evidence_agreement": 85, "source_authority": 100,
        "recency_factor": 100, "tavily_relevance": 88,
        "formula": "verdict_multiplier * (0.35*agreement + 0.30*authority + 0.20*recency + 0.15*relevance)"
      },
      "current_value": "...",
      "explanation": "...",
      "confidence": 0.9,
      "evidence": [
        { "url": "https://openai.com/api/pricing/", "title": "...", "published_date": null,
          "quote": "under 25 words", "domain": "openai.com", "is_official": true, "relevance_score": 0.88 }
      ]
    }
  ],
  "corrected_answer": "... [1] ...\n\n### Verified Sources\n- [1] [Title](url)",
  "changes": [
    { "claim_id": "claim-1", "original_claim": "...", "updated_claim": "...", "reason": "...", "citation_index": 1 }
  ],
  "usage": { "tavily_calls": 3, "tavily_calls_saved": 1, "llm_calls": 4, "latency_ms": 2140.5 },
  "question": null,
  "original_answer": "..."
}
```

UI notes:

- `quoted_span` gives character offsets into `original_answer`. Use it to highlight claims in place.
- `verdict` is `CURRENT`, `STALE`, `PARTIALLY_STALE`, `UNVERIFIABLE`, or `null` when `skipped` is true. Render a skipped claim as "not checked", not as verified.
- `freshness_score`: 0 - 100, higher is fresher. `overall_freshness_score` weights claims by staleness risk.

## POST /v1/check/stream

Same body as `/v1/check`. The response is `text/event-stream` with these events, in order:

| Event | Data |
| :--- | :--- |
| `pipeline_started` | `{report_id, answer_len, question}` |
| `claims_extracted` | `{count, claims: [Claim]}` |
| `triage_completed` | `{total, active, skipped}` |
| `claim_verifying` | `{claim_id, text, entity}`, once per checked claim |
| `claim_verified` | `{claim_id, verdict, freshness_score, current_value}`, once per checked claim |
| `complete` | the full CheckReport |
| `error` | `{detail}`, sent instead of `complete` if the pipeline fails |

Lines starting with `:` are keep-alive comments. It is a POST, so use `fetch` with a stream reader rather than `EventSource`:

```js
const res = await fetch("/v1/check/stream", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ answer }) });
const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
let buf = "";
for (;;) {
  const { value, done } = await reader.read();
  if (done) break;
  buf += value;
  let i;
  while ((i = buf.indexOf("\n\n")) >= 0) {
    const block = buf.slice(0, i); buf = buf.slice(i + 2);
    const event = /^event: (.*)$/m.exec(block)?.[1];
    const data = /^data: (.*)$/m.exec(block)?.[1];
    if (event && data) onEvent(event, JSON.parse(data));
  }
}
```

## POST /v1/wrap

`{"question": "...", "model_answer": "...", "options": {...}}` returns a CheckReport.

## POST /v1/ask

`{"question": "..."}`. Generates an answer with the configured LLM and audits it. Returns `503` if no LLM is configured or the call fails.

## GET /v1/history?limit=20&offset=0

```json
[{ "report_id": "rpt-...", "checked_at": "...", "question": "...", "overall_freshness_score": 22.4, "stale_count": 2, "current_count": 0 }]
```

`GET /v1/history/{report_id}` returns the stored CheckReport, or `404`.

## GET /v1/health

```json
{
  "status": "healthy", "service": "stale-answer-detector", "version": "1.1.0",
  "tavily_configured": true, "llm_configured": false, "verification_mode": "heuristic",
  "database": "sqlite", "tavily_calls_total": 12, "tavily_calls_saved": 4, "llm_calls_total": 0
}
```
