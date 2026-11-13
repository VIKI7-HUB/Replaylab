import { z } from 'zod';
import { ApiError, normalizeError } from './error';

const BASE_URL = import.meta.env.VITE_REPLAYLAB_URL || 'http://localhost:8086';

export const HealthSchema = z.object({
  stored_captures: z.number().default(0),
  dropped_captures: z.number().default(0),
  active_experiments: z.number().default(0),
  capture_enabled: z.boolean().default(true),
  version: z.string().default('1.0.0'),
  target_gateway_version: z.string().optional().nullable(),
});
export type ReplayLabHealth = z.infer<typeof HealthSchema>;

export const ConfigSchema = z.object({
  enabled: z.boolean().default(true),
  sampling_rate: z.number().default(1),
  // A gateway with no keys configured may send null for these lists.
  opt_in_keys: z.array(z.string()).nullish().transform((keys) => keys ?? []),
  opt_out_keys: z.array(z.string()).nullish().transform((keys) => keys ?? []),
  redact_pii: z.boolean().default(false),
  store_hashes_only: z.boolean().default(false),
  retention_days: z.number().default(30),
  default_budget_cap: z.number().default(10),
  max_stream_capture_bytes: z.number().default(65536),
  db_path: z.string().default(''),
  gateway_url: z.string().default(''),
  max_queue_size: z.number().default(1000),
  worker_count: z.number().default(4),
  api_listen_addr: z.string().default(''),
});
export type ReplayLabConfig = z.infer<typeof ConfigSchema>;

export const ModelPriceSchema = z.object({
  model: z.string(),
  input_per_1m_tokens: z.number(),
  output_per_1m_tokens: z.number(),
});
export type ModelPrice = z.infer<typeof ModelPriceSchema>;

export const CaptureSchema = z.object({
  id: z.string(),
  request_id: z.string().default(''),
  timestamp: z.string(),
  provider: z.string().optional().nullable(),
  model: z.string(),
  virtual_key: z.string().optional().nullable(),
  tenant: z.string().optional().nullable(),
  tag: z.string().optional().nullable(),
  status_code: z.number(),
  completion_state: z.enum(['complete', 'error', 'abandoned', 'truncated']).default('complete'),
  prompt_tokens: z.number().default(0),
  completion_tokens: z.number().default(0),
  total_tokens: z.number().default(0),
  latency_ms: z.number().default(0),
  time_to_first_token_ms: z.number().optional().nullable(),
  cost_estimate: z.number().default(0),
  is_streaming: z.boolean().default(false),
  usage_estimated: z.boolean().default(false),
  hash_only: z.boolean().default(false),
  error_message: z.string().optional().nullable(),
  messages_json: z.string().default('[]'),
  params_json: z.string().default('{}'),
  response_json: z.string().default('{}'),
});
export type Capture = z.infer<typeof CaptureSchema>;

export const JudgeOrderSchema = z.object({
  baseline_position: z.string().optional().nullable(),
  winner: z.string().optional().nullable(),
  candidate_outcome: z.string().optional().nullable(),
  reason: z.string().optional().nullable(),
});

export const JudgeSampleSchema = z.object({
  prompt_excerpt: z.string().default(''),
  baseline_excerpt: z.string().default(''),
  candidate_excerpt: z.string().default(''),
  judge_score: z.number().default(0),
  orders: z.array(JudgeOrderSchema).default([]),
});

export const CandidateResultSchema = z.object({
  candidate_model: z.string(),
  total_requests: z.number().default(0),
  successful_requests: z.number().default(0),
  failed_requests: z.number().default(0),
  error_rate: z.number().default(0),
  avg_quality_score: z.number().default(0),
  json_validity_rate: z.number().default(0),
  json_samples: z.number().default(0),
  p50_latency_ms: z.number().default(0),
  p95_latency_ms: z.number().default(0),
  original_avg_latency_ms: z.number().default(0),
  candidate_avg_latency_ms: z.number().default(0),
  latency_delta_percent: z.number().default(0),
  original_cost_dollars: z.number().default(0),
  candidate_cost_dollars: z.number().default(0),
  cost_savings_dollars: z.number().default(0),
  cost_savings_percent: z.number().default(0),
  recommendation: z.enum(['SAFE_TO_SWITCH', 'SWITCH_WITH_CAUTION', 'DO_NOT_SWITCH']),
  reasoning: z.string().default(''),
  projected_monthly_savings: z.number().default(0),
  judge_score: z.number().optional().nullable(),
  judge_wins: z.number().optional().nullable(),
  judge_ties: z.number().optional().nullable(),
  judge_losses: z.number().optional().nullable(),
  judge_coverage: z.number().optional().nullable(),
  judge_disagreement_rate: z.number().optional().nullable(),
  judge_samples: z.array(JudgeSampleSchema).optional().nullable(),
});
export type CandidateResult = z.infer<typeof CandidateResultSchema>;

export const RecommendationSchema = z.object({
  candidate_model: z.string(),
  verdict: z.string(),
  confidence: z.number().default(0),
});

export const ExperimentSchema = z.object({
  id: z.string(),
  name: z.string(),
  status: z.enum(['running', 'completed', 'cancelled', 'budget_exhausted', 'failed']),
  stop_reason: z.string().optional().nullable(),
  original_model: z.string(),
  candidate_models: z.array(z.string()).default([]),
  total_replays: z.number().default(0),
  completed_replays: z.number().default(0),
  spent_dollars: z.number().default(0),
  budget_cap_dollars: z.number().default(0),
  created_at: z.string(),
  results: z.array(CandidateResultSchema).default([]),
  recommendations: z.array(RecommendationSchema).default([]),
});
export type Experiment = z.infer<typeof ExperimentSchema>;

export const EstimateResponseSchema = z.object({
  spent_dollars: z.number().default(0),
  total_replays: z.number().default(0),
  stop_reason: z.string().optional().nullable(),
  estimated_replay_dollars: z.number().optional().nullable(),
  estimated_judge_dollars: z.number().optional().nullable(),
});
export type EstimateResponse = z.infer<typeof EstimateResponseSchema>;

export const ProgressEventSchema = z.object({
  status: z.string(),
  candidate_model: z.string().optional().nullable(),
  completed_count: z.number().default(0),
  total_count: z.number().default(0),
  progress_percent: z.number().default(0),
  current_quality: z.number().optional().nullable(),
  current_judge_score: z.number().optional().nullable(),
  spent_dollars: z.number().default(0),
  message: z.string().default(''),
  final: z.boolean().default(false),
});
export type ProgressEvent = z.infer<typeof ProgressEventSchema>;

async function handleResponse<O, I = O>(res: Response, schema?: z.ZodType<O, z.ZodTypeDef, I>): Promise<O> {
  if (!res.ok) {
    let msg = `HTTP error ${res.status}`;
    try {
      const errJson = await res.json();
      if (errJson && typeof errJson.error === 'string') {
        msg = errJson.error;
      } else if (errJson && typeof errJson.detail === 'string') {
        msg = errJson.detail;
      }
    } catch {
      // ignore
    }
    throw new ApiError(res.status, msg);
  }
  const data = await res.json();
  if (schema) {
    const parsed = schema.safeParse(data);
    if (!parsed.success) {
      console.warn('Schema validation warnings:', parsed.error);
      return data as O;
    }
    return parsed.data;
  }
  return data as O;
}

let inFlightHealth: Promise<ReplayLabHealth> | null = null;
let cachedHealth: { data: ReplayLabHealth; timestamp: number } | null = null;

export async function getHealth(forceRefresh = false): Promise<ReplayLabHealth> {
  const now = Date.now();
  if (!forceRefresh && cachedHealth && now - cachedHealth.timestamp < 5000) {
    return cachedHealth.data;
  }
  if (!forceRefresh && inFlightHealth) {
    return inFlightHealth;
  }
  const url = `${BASE_URL}/replaylab/health`;
  inFlightHealth = (async () => {
    try {
      const res = await fetch(url);
      const data = await handleResponse(res, HealthSchema);
      cachedHealth = { data, timestamp: Date.now() };
      return data;
    } catch (err) {
      throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
    } finally {
      inFlightHealth = null;
    }
  })();
  return inFlightHealth;
}

export async function getConfig(): Promise<ReplayLabConfig> {
  const url = `${BASE_URL}/replaylab/config`;
  try {
    const res = await fetch(url);
    return await handleResponse(res, ConfigSchema);
  } catch (err) {
    throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
  }
}

export async function updateConfig(updates: Partial<ReplayLabConfig>): Promise<ReplayLabConfig> {
  const url = `${BASE_URL}/replaylab/config`;
  try {
    const res = await fetch(url, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(updates),
    });
    return await handleResponse(res, ConfigSchema);
  } catch (err) {
    throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
  }
}

let inFlightModels: Promise<ModelPrice[]> | null = null;
let cachedModels: { data: ModelPrice[]; timestamp: number } | null = null;

export async function getModels(forceRefresh = false): Promise<ModelPrice[]> {
  const now = Date.now();
  if (!forceRefresh && cachedModels && now - cachedModels.timestamp < 30000) {
    return cachedModels.data;
  }
  if (!forceRefresh && inFlightModels) {
    return inFlightModels;
  }
  const url = `${BASE_URL}/replaylab/models`;
  inFlightModels = (async () => {
    try {
      const res = await fetch(url);
      const data = await handleResponse(res, z.array(ModelPriceSchema));
      cachedModels = { data, timestamp: Date.now() };
      return data;
    } catch (err) {
      throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
    } finally {
      inFlightModels = null;
    }
  })();
  return inFlightModels;
}

export interface CaptureFilters {
  model?: string;
  tag?: string;
  virtual_key?: string;
  streamed?: boolean;
  limit?: number;
  offset?: number;
}

export async function getCaptures(filters: CaptureFilters = {}): Promise<Capture[]> {
  const params = new URLSearchParams();
  if (filters.model) params.set('model', filters.model);
  if (filters.tag) params.set('tag', filters.tag);
  if (filters.virtual_key) params.set('virtual_key', filters.virtual_key);
  if (filters.streamed !== undefined) params.set('streamed', String(filters.streamed));
  if (filters.limit) params.set('limit', String(filters.limit));
  if (filters.offset !== undefined) params.set('offset', String(filters.offset));

  const url = `${BASE_URL}/replaylab/captures?${params.toString()}`;
  try {
    const res = await fetch(url);
    return await handleResponse(res, z.array(CaptureSchema));
  } catch (err) {
    throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
  }
}

export async function getCaptureById(id: string): Promise<Capture> {
  const url = `${BASE_URL}/replaylab/captures/${encodeURIComponent(id)}`;
  try {
    const res = await fetch(url);
    return await handleResponse(res, CaptureSchema);
  } catch (err) {
    throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
  }
}

export interface CreateExperimentPayload {
  name: string;
  original_model: string;
  candidate_models: string[];
  filter_tag?: string;
  streamed?: boolean;
  sample_size?: number;
  budget_cap_dollars?: number;
  fixed_temperature?: number;
  monthly_request_volume?: number;
  dry_run?: boolean;
  judge?: {
    model: string;
    rubric: string;
    max_judge_samples?: number;
  };
}

export async function createExperiment(payload: CreateExperimentPayload): Promise<EstimateResponse | { id: string }> {
  const url = `${BASE_URL}/replaylab/experiments`;
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (payload.dry_run) {
      return await handleResponse(res, EstimateResponseSchema);
    }
    return await handleResponse(res);
  } catch (err) {
    throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
  }
}

export async function getExperiments(limit?: number): Promise<Experiment[]> {
  const params = limit ? `?limit=${limit}` : '';
  const url = `${BASE_URL}/replaylab/experiments${params}`;
  try {
    const res = await fetch(url);
    return await handleResponse(res, z.array(ExperimentSchema));
  } catch (err) {
    throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
  }
}

export async function getExperimentById(id: string): Promise<Experiment> {
  const url = `${BASE_URL}/replaylab/experiments/${encodeURIComponent(id)}`;
  try {
    const res = await fetch(url);
    return await handleResponse(res, ExperimentSchema);
  } catch (err) {
    throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
  }
}

export async function cancelExperiment(id: string): Promise<void> {
  const url = `${BASE_URL}/replaylab/experiments/${encodeURIComponent(id)}/cancel`;
  try {
    const res = await fetch(url, { method: 'POST' });
    if (!res.ok) {
      let msg = 'Failed to cancel experiment';
      if (res.status === 409) {
        msg = 'Experiment has already finished';
      }
      throw new ApiError(res.status, msg);
    }
  } catch (err) {
    throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
  }
}

export async function getExperimentReport(id: string): Promise<string> {
  const url = `${BASE_URL}/replaylab/experiments/${encodeURIComponent(id)}/report?format=markdown`;
  try {
    const res = await fetch(url);
    if (!res.ok) {
      throw new ApiError(res.status, `Failed to fetch report (${res.status})`);
    }
    return await res.text();
  } catch (err) {
    throw normalizeError(err, 'Bifrost Replay Lab', BASE_URL);
  }
}

