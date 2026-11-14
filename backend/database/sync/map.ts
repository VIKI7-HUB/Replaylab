import type { Capture, Experiment, Report } from './sources.js';

/** Parses a JSON string payload; a non-JSON string (e.g. a hash) is kept as a JSON string value. */
export function jsonOrString(s: string): unknown {
  if (!s) return null;
  try {
    return JSON.parse(s);
  } catch {
    return s;
  }
}

/** Pulls the assistant text out of an OpenAI-style response, for search and snippets. */
export function responseText(raw: string): string | null {
  if (!raw) return null;
  const parsed = jsonOrString(raw);
  if (parsed && typeof parsed === 'object') {
    const choices = (parsed as { choices?: unknown }).choices;
    if (Array.isArray(choices)) {
      const text = choices
        .map((c) => {
          const m = (c ?? {}) as { message?: { content?: unknown }; delta?: { content?: unknown }; text?: unknown };
          const content = m.message?.content ?? m.delta?.content ?? m.text;
          return typeof content === 'string' ? content : '';
        })
        .join('');
      if (text) return text;
    }
  }
  return raw.length > 4000 ? raw.slice(0, 4000) : raw;
}

const COMPLETION_STATES = new Set(['complete', 'error', 'abandoned', 'truncated']);
const int = (v: number) => Math.round(v);
const intOrNull = (v: number | null | undefined) => (v == null ? null : Math.round(v));

export function captureRow(c: Capture) {
  return {
    id: c.id,
    ts: new Date(c.timestamp),
    request_id: c.request_id,
    virtual_key: c.virtual_key,
    tenant: c.tenant,
    provider: c.provider,
    model: c.model,
    tag: c.tag,
    prompt_tokens: int(c.prompt_tokens),
    completion_tokens: int(c.completion_tokens),
    total_tokens: int(c.total_tokens),
    latency_ms: int(c.latency_ms),
    ttft_ms: intOrNull(c.time_to_first_token_ms),
    cost_usd: c.cost_estimate,
    status_code: int(c.status_code),
    error_message: c.error_message || null,
    is_streaming: c.is_streaming,
    completion_state: COMPLETION_STATES.has(c.completion_state) ? c.completion_state : 'complete',
    usage_estimated: c.usage_estimated,
    hash_only: c.hash_only,
    messages: jsonOrString(c.messages_json),
    params: jsonOrString(c.params_json),
    response_text: responseText(c.response_json),
  };
}
export type CaptureRow = ReturnType<typeof captureRow>;

const EXPERIMENT_STATUSES = new Set(['running', 'completed', 'cancelled', 'budget_exhausted', 'failed']);
const VERDICTS = new Set(['SAFE_TO_SWITCH', 'SWITCH_WITH_CAUTION', 'DO_NOT_SWITCH']);

export const isFinal = (status: string) => status !== 'running';

export function experimentRow(e: Experiment) {
  return {
    id: e.id,
    name: e.name,
    status: EXPERIMENT_STATUSES.has(e.status) ? e.status : 'failed',
    stop_reason: e.stop_reason || null,
    original_model: e.original_model,
    candidate_models: e.candidate_models,
    filter_tag: e.filter_tag || null,
    streamed: e.streamed ?? null,
    sample_size: int(e.sample_size),
    budget_cap_usd: e.budget_cap_dollars,
    monthly_request_volume: int(e.monthly_request_volume),
    spent_usd: e.spent_dollars,
    spent_tokens: int(e.spent_tokens),
    total_replays: int(e.total_replays),
    completed_replays: int(e.completed_replays),
    has_judge: !!e.judge,
    judge_model: e.judge?.model ?? null,
    judge_spent_usd: e.judge_spent_dollars ?? null,
    dry_run: e.dry_run,
    created_at: new Date(e.created_at),
    completed_at: e.completed_at ? new Date(e.completed_at) : null,
    raw: e as unknown,
  };
}

export function experimentResultRows(e: Experiment) {
  const recs = new Map(e.recommendations.map((r) => [r.candidate_model, r]));
  return e.results.map((r) => {
    const rec = recs.get(r.candidate_model);
    const verdict = r.recommendation || rec?.verdict || null;
    return {
      experiment_id: e.id,
      candidate_model: r.candidate_model,
      experiment_created_at: new Date(e.created_at),
      total_requests: int(r.total_requests),
      successful_requests: int(r.successful_requests),
      failed_requests: int(r.failed_requests),
      error_rate: r.error_rate,
      avg_quality_score: r.avg_quality_score,
      json_validity_rate: r.json_validity_rate,
      json_samples: int(r.json_samples),
      p50_latency_ms: r.p50_latency_ms,
      p95_latency_ms: r.p95_latency_ms,
      original_avg_latency_ms: r.original_avg_latency_ms,
      candidate_avg_latency_ms: r.candidate_avg_latency_ms,
      latency_delta_percent: r.latency_delta_percent,
      original_cost_usd: r.original_cost_dollars,
      candidate_cost_usd: r.candidate_cost_dollars,
      cost_savings_usd: r.cost_savings_dollars,
      cost_savings_percent: r.cost_savings_percent,
      verdict: verdict && VERDICTS.has(verdict) ? verdict : null,
      confidence: rec?.confidence ?? null,
      reasoning: r.reasoning ?? null,
      projected_monthly_savings: r.projected_monthly_savings,
      judge_score: r.judge_score ?? null,
      judge_wins: intOrNull(r.judge_wins),
      judge_ties: intOrNull(r.judge_ties),
      judge_losses: intOrNull(r.judge_losses),
      judge_coverage: r.judge_coverage ?? null,
      judge_disagreement_rate: r.judge_disagreement_rate ?? null,
    };
  });
}

const CLAIM_VERDICTS = new Set(['CURRENT', 'STALE', 'PARTIALLY_STALE', 'UNVERIFIABLE']);

export function reportRow(r: Report) {
  const s = r.summary;
  const u = r.usage;
  return {
    report_id: r.report_id,
    checked_at: new Date(r.checked_at),
    question: r.question ?? null,
    original_answer: r.original_answer,
    overall_freshness_score: r.overall_freshness_score,
    verification_mode: r.verification_mode,
    current_count: int(s?.current ?? 0),
    stale_count: int(s?.stale ?? 0),
    partially_stale_count: int(s?.partially_stale ?? 0),
    unverifiable_count: int(s?.unverifiable ?? 0),
    skipped_count: int(s?.skipped ?? 0),
    total_claims: int(s?.total_claims ?? r.claims.length),
    tavily_calls: int(u?.tavily_calls ?? 0),
    tavily_calls_saved: int(u?.tavily_calls_saved ?? 0),
    llm_calls: int(u?.llm_calls ?? 0),
    latency_ms: u?.latency_ms ?? 0,
    corrected_answer: r.corrected_answer ?? null,
    raw: r as unknown,
  };
}

export function claimRows(r: Report) {
  const checkedAt = new Date(r.checked_at);
  // A report can repeat a claim id in theory; keep the last so the batch upsert stays valid.
  const byId = new Map<string, ReturnType<typeof claim>>();
  function claim(c: Report['claims'][number]) {
    return {
      report_id: r.report_id,
      claim_id: c.id,
      checked_at: checkedAt,
      text: c.text,
      entity: c.entity,
      claim_type: c.claim_type,
      staleness_risk: c.staleness_risk,
      skipped: c.skipped,
      skip_reason: c.skip_reason ?? null,
      verdict: c.verdict && CLAIM_VERDICTS.has(c.verdict) ? c.verdict : null,
      freshness_score: c.freshness_score ?? null,
      confidence: c.confidence,
      current_value: c.current_value ?? null,
      explanation: c.explanation ?? null,
      evidence_count: c.evidence.length,
      official_evidence_count: c.evidence.filter((e) => e.is_official).length,
    };
  }
  for (const c of r.claims) byId.set(c.id, claim(c));
  return [...byId.values()];
}
