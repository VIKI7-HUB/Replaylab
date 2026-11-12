import { z } from 'zod';
import { ApiError, normalizeError } from './error';

const BASE_URL = import.meta.env.VITE_INSIGHTS_URL || 'http://localhost:8090';

export const InsightsHealthSchema = z.object({
  status: z.string().default('healthy'),
});

export const InsightsOverviewSchema = z.object({
  replaylab: z.object({
    captures_24h: z.number().default(0),
    cost_24h_usd: z.number().default(0),
    error_rate_24h: z.number().default(0),
    safe_switch_monthly_savings_usd: z.number().default(0),
  }),
  stale: z.object({
    reports_7d: z.number().default(0),
    avg_freshness_7d: z.number().default(0),
    stale_claims_7d: z.number().default(0),
    llm_mode_percent: z.number().default(0),
    heuristic_mode_percent: z.number().default(0),
  }),
  latest_experiments: z.array(z.object({
    id: z.string(),
    name: z.string(),
    status: z.string(),
    best_verdict: z.string().optional().nullable(),
    best_verdict_model: z.string().optional().nullable(),
    created_at: z.string(),
  })).default([]),
  latest_reports: z.array(z.object({
    id: z.string(),
    text_preview: z.string(),
    freshness_score: z.number().default(0),
    stale_count: z.number().default(0),
    checked_at: z.string(),
  })).default([]),
});
export type InsightsOverview = z.infer<typeof InsightsOverviewSchema>;

export const DailyReplayLabPointSchema = z.object({
  date: z.string(),
  model: z.string(),
  captures: z.number().default(0),
  cost_usd: z.number().default(0),
});
export type DailyReplayLabPoint = z.infer<typeof DailyReplayLabPointSchema>;

export const VerdictPointSchema = z.object({
  date: z.string(),
  safe: z.number().default(0),
  caution: z.number().default(0),
  do_not_switch: z.number().default(0),
});
export type VerdictPoint = z.infer<typeof VerdictPointSchema>;

export const StaleDailyPointSchema = z.object({
  date: z.string(),
  claim_type: z.string(),
  avg_freshness: z.number().default(0),
});
export type StaleDailyPoint = z.infer<typeof StaleDailyPointSchema>;

export const SearchResultItemSchema = z.object({
  kind: z.enum(['Capture', 'Experiment', 'Report', 'Claim']),
  id: z.string(),
  title: z.string(),
  created_at: z.string(),
  url: z.string(),
});
export type SearchResultItem = z.infer<typeof SearchResultItemSchema>;

export async function getInsightsHealth(): Promise<boolean> {
  const url = `${BASE_URL}/api/health`;
  try {
    const res = await fetch(url);
    return res.ok;
  } catch {
    return false;
  }
}

export async function getOverview(days: number = 1): Promise<InsightsOverview> {
  const url = `${BASE_URL}/api/insights/overview?days=${days}`;
  try {
    const res = await fetch(url);
    if (!res.ok) throw new ApiError(res.status, `Insights error ${res.status}`);
    const data = await res.json();
    return InsightsOverviewSchema.parse(data);
  } catch (err) {
    throw normalizeError(err, 'Insights API', BASE_URL);
  }
}

export async function getReplayLabDaily(days: number = 30, model?: string): Promise<DailyReplayLabPoint[]> {
  const params = new URLSearchParams({ days: String(days) });
  if (model) params.set('model', model);
  const url = `${BASE_URL}/api/insights/replaylab/daily?${params.toString()}`;
  try {
    const res = await fetch(url);
    if (!res.ok) throw new ApiError(res.status, `Insights error ${res.status}`);
    return await res.json();
  } catch (err) {
    throw normalizeError(err, 'Insights API', BASE_URL);
  }
}

export async function getReplayLabVerdicts(days: number = 90): Promise<VerdictPoint[]> {
  const url = `${BASE_URL}/api/insights/replaylab/verdicts?days=${days}`;
  try {
    const res = await fetch(url);
    if (!res.ok) throw new ApiError(res.status, `Insights error ${res.status}`);
    return await res.json();
  } catch (err) {
    throw normalizeError(err, 'Insights API', BASE_URL);
  }
}

export async function getStaleDaily(days: number = 30): Promise<StaleDailyPoint[]> {
  const url = `${BASE_URL}/api/insights/stale/daily?days=${days}`;
  try {
    const res = await fetch(url);
    if (!res.ok) throw new ApiError(res.status, `Insights error ${res.status}`);
    return await res.json();
  } catch (err) {
    throw normalizeError(err, 'Insights API', BASE_URL);
  }
}

export async function searchGlobal(q: string, limit: number = 20): Promise<SearchResultItem[]> {
  if (!q.trim()) return [];
  const url = `${BASE_URL}/api/search?q=${encodeURIComponent(q)}&limit=${limit}`;
  try {
    const res = await fetch(url);
    if (!res.ok) throw new ApiError(res.status, `Search error ${res.status}`);
    const data = await res.json();
    return Array.isArray(data) ? data : data.items || [];
  } catch (err) {
    throw normalizeError(err, 'Insights API', BASE_URL);
  }
}

