import { z } from 'zod';
import { getJson } from '../lib/http.js';
import type { Config } from '../lib/config.js';

// Lenient parsers: they accept every field the services document, default the
// optional ones, and keep unknown fields, so an additive backend change never
// breaks the sync.

const num = z.number().nullish().transform((v) => v ?? 0);
const str = z.string().nullish().transform((v) => v ?? '');
const optNum = z.number().nullish();
const bool = z.boolean().nullish().transform((v) => v ?? false);

export const CaptureSchema = z.object({
  id: z.string(),
  request_id: str,
  timestamp: z.string(),
  virtual_key: str,
  tenant: str,
  provider: str,
  model: str,
  tag: str,
  messages_json: str,
  params_json: str,
  response_json: str,
  prompt_tokens: num,
  completion_tokens: num,
  total_tokens: num,
  latency_ms: num,
  cost_estimate: num,
  status_code: num,
  error_message: z.string().nullish(),
  is_streaming: bool,
  hash_only: bool,
  completion_state: z.string().nullish().transform((v) => v || 'complete'),
  time_to_first_token_ms: optNum,
  usage_estimated: bool,
}).passthrough();
export type Capture = z.infer<typeof CaptureSchema>;

export const ExperimentResultSchema = z.object({
  candidate_model: z.string(),
  total_requests: num,
  successful_requests: num,
  failed_requests: num,
  error_rate: num,
  avg_quality_score: num,
  json_validity_rate: num,
  json_samples: num,
  p50_latency_ms: num,
  p95_latency_ms: num,
  original_avg_latency_ms: num,
  candidate_avg_latency_ms: num,
  latency_delta_percent: num,
  original_cost_dollars: num,
  candidate_cost_dollars: num,
  cost_savings_dollars: num,
  cost_savings_percent: num,
  recommendation: z.string().nullish(),
  reasoning: z.string().nullish(),
  projected_monthly_savings: num,
  judge_score: optNum,
  judge_wins: optNum,
  judge_ties: optNum,
  judge_losses: optNum,
  judge_coverage: optNum,
  judge_disagreement_rate: optNum,
}).passthrough();

export const RecommendationSchema = z.object({
  candidate_model: z.string(),
  verdict: z.string().nullish(),
  confidence: optNum,
}).passthrough();

export const ExperimentSchema = z.object({
  id: z.string(),
  name: str,
  status: z.string(),
  stop_reason: z.string().nullish(),
  original_model: str,
  candidate_models: z.array(z.string()).nullish().transform((v) => v ?? []),
  filter_tag: z.string().nullish(),
  streamed: z.boolean().nullish(),
  sample_size: num,
  budget_cap_dollars: num,
  monthly_request_volume: num,
  total_replays: num,
  completed_replays: num,
  spent_dollars: num,
  spent_tokens: num,
  created_at: z.string(),
  completed_at: z.string().nullish(),
  judge: z.object({ model: z.string().nullish() }).passthrough().nullish(),
  judge_spent_dollars: optNum,
  dry_run: bool,
  results: z.array(ExperimentResultSchema).nullish().transform((v) => v ?? []),
  recommendations: z.array(RecommendationSchema).nullish().transform((v) => v ?? []),
}).passthrough();
export type Experiment = z.infer<typeof ExperimentSchema>;

export const HistoryItemSchema = z.object({
  report_id: z.string(),
  checked_at: z.string(),
}).passthrough();

const EvidenceSchema = z.object({ is_official: z.boolean().nullish() }).passthrough();

export const ClaimSchema = z.object({
  id: z.string(),
  text: str,
  entity: str,
  claim_type: z.string().nullish().transform((v) => v || 'other'),
  staleness_risk: num,
  skipped: bool,
  skip_reason: z.string().nullish(),
  verdict: z.string().nullish(),
  freshness_score: optNum,
  current_value: z.string().nullish(),
  explanation: z.string().nullish(),
  confidence: num,
  evidence: z.array(EvidenceSchema).nullish().transform((v) => v ?? []),
}).passthrough();

export const ReportSchema = z.object({
  report_id: z.string(),
  checked_at: z.string(),
  question: z.string().nullish(),
  original_answer: str,
  overall_freshness_score: num,
  verification_mode: z.string().nullish().transform((v) => v || 'heuristic'),
  summary: z.object({
    current: num, stale: num, partially_stale: num, unverifiable: num, skipped: num, total_claims: num,
  }).passthrough().nullish(),
  usage: z.object({
    tavily_calls: num, tavily_calls_saved: num, llm_calls: num, latency_ms: num,
  }).passthrough().nullish(),
  corrected_answer: z.string().nullish(),
  claims: z.array(ClaimSchema).nullish().transform((v) => v ?? []),
}).passthrough();
export type Report = z.infer<typeof ReportSchema>;

export class ReplayLabClient {
  constructor(private cfg: Pick<Config, 'replaylabUrl' | 'httpTimeoutMs'>) {}
  private get(p: string) {
    return getJson(`${this.cfg.replaylabUrl}${p}`, { timeoutMs: this.cfg.httpTimeoutMs });
  }
  async captures(limit: number, offset: number): Promise<Capture[]> {
    return z.array(CaptureSchema).parse(await this.get(`/replaylab/captures?limit=${limit}&offset=${offset}`));
  }
  async experiments(limit: number, offset: number): Promise<Experiment[]> {
    return z.array(ExperimentSchema).parse(await this.get(`/replaylab/experiments?limit=${limit}&offset=${offset}`));
  }
  async experiment(id: string): Promise<Experiment> {
    return ExperimentSchema.parse(await this.get(`/replaylab/experiments/${encodeURIComponent(id)}`));
  }
  async retentionDays(): Promise<number> {
    const cfg = z.object({ retention_days: z.number().nullish() }).passthrough().parse(await this.get('/replaylab/config'));
    return cfg.retention_days ?? 0;
  }
}

export class StaleClient {
  constructor(private cfg: Pick<Config, 'staleUrl' | 'staleApiKey' | 'httpTimeoutMs'>) {}
  private get(p: string) {
    const headers: Record<string, string> = this.cfg.staleApiKey ? { 'X-API-Key': this.cfg.staleApiKey } : {};
    return getJson(`${this.cfg.staleUrl}${p}`, { headers, timeoutMs: this.cfg.httpTimeoutMs });
  }
  async history(limit: number, offset: number) {
    return z.array(HistoryItemSchema).parse(await this.get(`/v1/history?limit=${limit}&offset=${offset}`));
  }
  async report(id: string): Promise<Report> {
    return ReportSchema.parse(await this.get(`/v1/history/${encodeURIComponent(id)}`));
  }
}
