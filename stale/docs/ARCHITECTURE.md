# Architecture

How a single `/v1/check` request turns an LLM answer into scored, cited, corrected output.

```
POST /v1/check  { "answer": "..." }
        │
        ▼
┌───────────────────┐
│ 1 · EXTRACT       │  app/extractor.py
│ LLM → atomic      │  atomic claims + character spans
│ claims, regex     │  (regex fallback when no LLM key)
│ fallback          │
└─────────┬─────────┘
          ▼
┌───────────────────┐
│ 2 · TRIAGE        │  app/triage.py
│ volatility priors │  risk 0..1 per claim
│ + temporal cues   │  risk < 0.25 → SKIPPED (no credits)
│ + answer_date age │  timeless maths/history → SKIPPED
└─────────┬─────────┘
          ▼
┌───────────────────┐
│ 3 · PLAN          │  app/queries.py
│ query + params    │  per claim type: official domains for
│ per claim type    │  pricing/versions, topic=news for leadership
└─────────┬─────────┘
          ▼
┌───────────────────┐
│ 4 · SEARCH        │  app/tavily_client.py
│ Tavily /search    │  cache → semaphore → retry → count credit
│ + /extract        │  /extract reads the vendor's own page
└─────────┬─────────┘
          ▼
┌───────────────────┐
│ 5 · VERIFY        │  app/verifier.py
│ LLM judge (fenced)│  evidence inside <untrusted_web_evidence>
│ or heuristics     │  heuristic never guesses → UNVERIFIABLE
└─────────┬─────────┘
          ▼
┌───────────────────┐
│ 6 · SCORE         │  app/scoring.py
│ 0-100 + breakdown │  deterministic formula, every term returned
└─────────┬─────────┘
          ▼
┌───────────────────┐
│ 7 · CORRECT       │  app/corrector.py
│ rewrite + cites   │  LLM rewrite, or deterministic replacement
└─────────┬─────────┘
          ▼
   CheckReport ──► SQLite (report history)
```

## Module map

| File | Responsibility |
| :--- | :--- |
| `app/main.py` | Routes, SSE streaming, pipeline orchestration |
| `app/extractor.py` | Claim extraction — LLM structured output, regex fallback |
| `app/triage.py` | Staleness-risk priors, temporal cues, skip decisions |
| `app/queries.py` | Per-claim-type search strategy, official-domain resolution |
| `app/tavily_client.py` | Tavily `/search` + `/extract`, retries, concurrency limit, cache |
| `app/verifier.py` | LLM judge with injection fences, deterministic heuristics |
| `app/scoring.py` | Freshness formula, per-claim breakdown, weighted aggregate |
| `app/corrector.py` | Corrected answer + citation list + change records |
| `app/llm.py` | OpenAI-compatible client (retries, JSON-mode, tolerant parsing) |
| `app/cache.py` | SQLite cache with per-claim-type TTL |
| `app/db.py` | Report history persistence |
| `app/usage.py` | Per-request credit accounting (`ContextVar`, concurrency-safe) |
| `app/schemas.py` | Pydantic request/response contracts |

## The scoring formula

```
freshness = verdict_multiplier × (0.35·agreement + 0.30·authority + 0.20·recency + 0.15·relevance)
```

| Term | Source | Meaning |
| :--- | :--- | :--- |
| `verdict_multiplier` | `CURRENT 1.00 · PARTIALLY_STALE 0.45 · UNVERIFIABLE 0.50 · STALE 0.05` | The truth factor |
| `agreement` | distinct evidence domains (≥3 → 100) | do independent sources agree |
| `authority` | official/trusted domains → 100 | is this the primary source |
| `recency` | newest evidence date | how fresh is the evidence itself |
| `relevance` | Tavily relevance score | did the search actually hit the fact |

Per-claim scores are aggregated into the answer score weighted by `staleness_risk`
(high-risk claims count more), and every component is returned in `score_breakdown`.

## Credit model

| Mechanism | Effect |
| :--- | :--- |
| **Triage** | Claims below risk 0.25 never reach Tavily — timeless maths and history cost nothing |
| **TTL cache** (SQLite) | Repeated queries are served from cache; TTL by claim type: leadership/event 30 min, pricing 6 h, version 12 h, regulation 24 h |
| **`/extract` caching** | The expensive page-read is cached by URL |
| **`force_fresh`** | `options.force_fresh` bypasses the cache for a deliberate re-check |
| **Per-request counters** | `usage.tavily_calls`, `tavily_calls_saved`, `llm_calls` — `ContextVar`-based so concurrent requests never mix |

## Failure behaviour (by design)

| Situation | Behaviour |
| :--- | :--- |
| No Tavily key | Every checked claim → `UNVERIFIABLE`, report still produced |
| No LLM key | Deterministic heuristics only; abstains rather than guessing |
| Search returns nothing | `UNVERIFIABLE` (50 multiplier), never a fabricated verdict |
| Tavily 429/5xx | Up to 3 attempts with exponential backoff, then the claim abstains |
| Client disconnects mid-stream | The pipeline task is cancelled and work stops |
| Report persistence fails | The user still gets the report; the error is logged |

## Security posture

- **Prompt injection:** scraped page content is wrapped in `<untrusted_web_evidence>` and the judge is told it is data, never instructions.
- **Secrets:** `.env` is gitignored; a CI job fails the build if a `.env`/`.db` file is ever tracked or a `tvly-…` key appears in history.
- **Optional API key:** when `API_KEY` is set, every `/v1` route requires `X-API-Key`.
- **CORS:** permissive for the demo frontend, credentials disabled (no `*` + credentials conflict).

## Scaling notes

- Claims are verified **concurrently**; Tavily concurrency is bounded by a semaphore (`TAVILY_CONCURRENCY`, default 5).
- The API is stateless apart from SQLite; swap `DATABASE_PATH` for a shared volume or a hosted DB.
- The cache is per-instance today — moving it to Redis would make it shared across replicas.
