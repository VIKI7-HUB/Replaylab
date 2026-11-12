/**
 * Health of every service the console shell reports on, including the product
 * served by the other console. Only the fields the top bar shows are read.
 */

export interface ServiceHealth {
  status: 'healthy' | 'degraded' | 'offline';
  text: string;
  version?: string;
}

const REPLAYLAB_URL = import.meta.env.VITE_REPLAYLAB_URL || 'http://localhost:8086';
const STALE_URL = import.meta.env.VITE_STALE_URL || 'http://localhost:8000';
const INSIGHTS_URL = import.meta.env.VITE_INSIGHTS_URL || 'http://localhost:8090';

async function getJson(url: string): Promise<{ ok: boolean; body: Record<string, unknown> }> {
  const res = await fetch(url);
  let body: Record<string, unknown> = {};
  try {
    body = await res.json();
  } catch {
    // a non-JSON body leaves the defaults
  }
  return { ok: res.ok, body };
}

export async function replayLabHealth(): Promise<ServiceHealth> {
  try {
    const { ok, body } = await getJson(`${REPLAYLAB_URL}/replaylab/health`);
    if (!ok) return { status: 'degraded', text: 'Replay Lab: degraded' };
    return {
      status: 'healthy',
      text: `Replay Lab: healthy · ${Number(body.stored_captures ?? 0)} captures`,
      version: typeof body.version === 'string' ? body.version : undefined,
    };
  } catch {
    return { status: 'offline', text: 'Replay Lab: offline' };
  }
}

export async function staleHealth(): Promise<ServiceHealth> {
  try {
    const { ok, body } = await getJson(`${STALE_URL}/v1/health`);
    if (!ok || body.status !== 'healthy') {
      return { status: 'degraded', text: 'Stale-Answer Detector: degraded' };
    }
    const mode = body.verification_mode === 'llm' ? 'llm mode' : 'heuristic mode';
    return {
      status: 'healthy',
      text: `Stale-Answer Detector: healthy · ${mode}`,
      version: typeof body.version === 'string' ? body.version : undefined,
    };
  } catch {
    return { status: 'offline', text: 'Stale-Answer Detector: offline' };
  }
}

export async function insightsHealth(): Promise<ServiceHealth> {
  try {
    const res = await fetch(`${INSIGHTS_URL}/api/health`);
    return res.ok
      ? { status: 'healthy', text: 'Insights: healthy' }
      : { status: 'offline', text: 'Insights: offline' };
  } catch {
    return { status: 'offline', text: 'Insights: offline' };
  }
}

/** Health of the product this console serves. */
export const productHealth = { replaylab: replayLabHealth, stale: staleHealth };

/** The service each product's console depends on, as the browser reaches it. */
export const PRODUCT_SERVICE_URL = { replaylab: REPLAYLAB_URL, stale: STALE_URL };
