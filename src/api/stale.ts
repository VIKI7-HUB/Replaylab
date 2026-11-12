import { z } from 'zod';
import { ApiError, normalizeError } from './error';

const BASE_URL = import.meta.env.VITE_STALE_URL || 'http://localhost:8000';
const API_KEY = import.meta.env.VITE_STALE_API_KEY || '';

function getHeaders(customHeaders: Record<string, string> = {}): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...customHeaders,
  };
  if (API_KEY) {
    headers['X-API-Key'] = API_KEY;
  }
  return headers;
}

export const StaleHealthSchema = z.object({
  status: z.string().default('healthy'),
  service: z.string().default('stale-answer-detector'),
  version: z.string().default('1.1.0'),
  tavily_configured: z.boolean().default(false),
  llm_configured: z.boolean().default(false),
  verification_mode: z.enum(['llm', 'heuristic']).default('heuristic'),
  database: z.string().default('sqlite'),
  tavily_calls_total: z.number().default(0),
  tavily_calls_saved: z.number().default(0),
  llm_calls_total: z.number().default(0),
});
export type StaleHealth = z.infer<typeof StaleHealthSchema>;

export const QuotedSpanSchema = z.object({
  start: z.number(),
  end: z.number(),
});
export type QuotedSpan = z.infer<typeof QuotedSpanSchema>;

export const ScoreBreakdownSchema = z.object({
  verdict_score: z.number().default(0),
  evidence_agreement: z.number().default(0),
  source_authority: z.number().default(0),
  recency_factor: z.number().default(0),
  tavily_relevance: z.number().default(0),
  formula: z.string().default(''),
});
export type ScoreBreakdown = z.infer<typeof ScoreBreakdownSchema>;

export const EvidenceItemSchema = z.object({
  url: z.string().default(''),
  title: z.string().default(''),
  published_date: z.string().optional().nullable(),
  quote: z.string().default(''),
  domain: z.string().default(''),
  is_official: z.boolean().default(false),
  relevance_score: z.number().default(0),
});
export type EvidenceItem = z.infer<typeof EvidenceItemSchema>;

export const ClaimSchema = z.object({
  id: z.string(),
  text: z.string(),
  entity: z.string().optional().nullable(),
  claim_type: z.string().default('general'),
  quoted_span: QuotedSpanSchema,
  staleness_risk: z.number().default(0),
  risk_reasons: z.array(z.string()).default([]),
  skipped: z.boolean().default(false),
  skip_reason: z.string().optional().nullable(),
  verdict: z.enum(['CURRENT', 'STALE', 'PARTIALLY_STALE', 'UNVERIFIABLE']).optional().nullable(),
  freshness_score: z.number().default(0),
  score_breakdown: ScoreBreakdownSchema.optional().nullable(),
  current_value: z.string().optional().nullable(),
  explanation: z.string().optional().nullable(),
  confidence: z.number().default(0),
  evidence: z.array(EvidenceItemSchema).default([]),
});
export type Claim = z.infer<typeof ClaimSchema>;

export const ChangeItemSchema = z.object({
  claim_id: z.string(),
  original_claim: z.string(),
  updated_claim: z.string(),
  reason: z.string(),
  citation_index: z.number(),
});
export type ChangeItem = z.infer<typeof ChangeItemSchema>;

export const SummarySchema = z.object({
  current: z.number().default(0),
  stale: z.number().default(0),
  partially_stale: z.number().default(0),
  unverifiable: z.number().default(0),
  skipped: z.number().default(0),
  total_claims: z.number().default(0),
});
export type Summary = z.infer<typeof SummarySchema>;

export const UsageSchema = z.object({
  tavily_calls: z.number().default(0),
  tavily_calls_saved: z.number().default(0),
  llm_calls: z.number().default(0),
  latency_ms: z.number().default(0),
});
export type Usage = z.infer<typeof UsageSchema>;

export const CheckReportSchema = z.object({
  report_id: z.string(),
  checked_at: z.string(),
  overall_freshness_score: z.number().default(0),
  verification_mode: z.enum(['llm', 'heuristic']).default('heuristic'),
  summary: SummarySchema,
  claims: z.array(ClaimSchema).default([]),
  corrected_answer: z.string().default(''),
  changes: z.array(ChangeItemSchema).default([]),
  usage: UsageSchema.default({
    tavily_calls: 0,
    tavily_calls_saved: 0,
    llm_calls: 0,
    latency_ms: 0,
  }),
  question: z.string().optional().nullable(),
  original_answer: z.string(),
});
export type CheckReport = z.infer<typeof CheckReportSchema>;

export const HistoryItemSchema = z.object({
  report_id: z.string(),
  checked_at: z.string(),
  question: z.string().optional().nullable(),
  overall_freshness_score: z.number().default(0),
  stale_count: z.number().default(0),
  current_count: z.number().default(0),
});
export type HistoryItem = z.infer<typeof HistoryItemSchema>;

export interface CheckOptions {
  max_claims?: number;
  depth?: 'basic' | 'advanced';
  domains_allow?: string[];
  domains_block?: string[];
  force_fresh?: boolean;
}

export interface CheckPayload {
  answer: string;
  question?: string;
  answer_date?: string;
  options?: CheckOptions;
}

async function handleResponse<O, I = O>(res: Response, schema?: z.ZodType<O, z.ZodTypeDef, I>): Promise<O> {
  if (!res.ok) {
    let msg = `HTTP error ${res.status}`;
    try {
      const errJson = await res.json();
      if (errJson && typeof errJson.detail === 'string') {
        msg = errJson.detail;
      } else if (errJson && Array.isArray(errJson.detail)) {
        msg = errJson.detail.map((d: { msg?: string }) => d.msg || '').join(', ');
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

export async function getStaleHealth(): Promise<StaleHealth> {
  const url = `${BASE_URL}/v1/health`;
  try {
    const res = await fetch(url, { headers: getHeaders() });
    return await handleResponse(res, StaleHealthSchema);
  } catch (err) {
    throw normalizeError(err, 'Stale-Answer Detector', BASE_URL);
  }
}

export async function checkAnswer(payload: CheckPayload): Promise<CheckReport> {
  const url = `${BASE_URL}/v1/check`;
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: getHeaders(),
      body: JSON.stringify(payload),
    });
    return await handleResponse(res, CheckReportSchema);
  } catch (err) {
    throw normalizeError(err, 'Stale-Answer Detector', BASE_URL);
  }
}

export async function askQuestion(question: string): Promise<CheckReport> {
  const url = `${BASE_URL}/v1/ask`;
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: getHeaders(),
      body: JSON.stringify({ question }),
    });
    if (res.status === 503) {
      throw new ApiError(503, 'Needs an LLM. Set LLM_API_KEY on the service.');
    }
    return await handleResponse(res, CheckReportSchema);
  } catch (err) {
    throw normalizeError(err, 'Stale-Answer Detector', BASE_URL);
  }
}

export async function getHistory(limit: number = 20, offset: number = 0): Promise<HistoryItem[]> {
  const url = `${BASE_URL}/v1/history?limit=${limit}&offset=${offset}`;
  try {
    const res = await fetch(url, { headers: getHeaders() });
    return await handleResponse(res, z.array(HistoryItemSchema));
  } catch (err) {
    throw normalizeError(err, 'Stale-Answer Detector', BASE_URL);
  }
}

export async function getReportById(reportId: string): Promise<CheckReport> {
  const url = `${BASE_URL}/v1/history/${encodeURIComponent(reportId)}`;
  try {
    const res = await fetch(url, { headers: getHeaders() });
    return await handleResponse(res, CheckReportSchema);
  } catch (err) {
    throw normalizeError(err, 'Stale-Answer Detector', BASE_URL);
  }
}

