import { ProgressEvent, ProgressEventSchema } from './replaylab';
import { CheckPayload, CheckReport, Claim } from './stale';
import { ApiError } from './error';

const REPLAYLAB_BASE = import.meta.env.VITE_REPLAYLAB_URL || 'http://localhost:8086';

/**
 * SSE subscription for Bifrost Replay Lab experiments
 */
export function subscribeExperiment(
  id: string,
  onEvent: (event: ProgressEvent) => void,
  onError?: (err: Error) => void
): () => void {
  const url = `${REPLAYLAB_BASE}/replaylab/experiments/${encodeURIComponent(id)}/events`;
  const es = new EventSource(url);

  const handleMessage = (e: MessageEvent) => {
    try {
      const data = JSON.parse(e.data);
      const parsed = ProgressEventSchema.parse(data);
      onEvent(parsed);
      if (parsed.final) {
        es.close();
      }
    } catch (err) {
      console.warn('Failed to parse SSE experiment event:', err);
    }
  };

  es.addEventListener('progress', handleMessage);
  // Also listen to default onmessage in case server sends untyped events
  es.onmessage = handleMessage;

  es.onerror = () => {
    es.close();
    if (onError) {
      onError(new ApiError(0, `Experiment SSE connection closed`));
    }
  };

  return () => {
    es.close();
  };
}


// Same base as api/stale.ts: the dev proxy, or VITE_STALE_API_URL when deployed.
const STALE_BASE = import.meta.env.VITE_STALE_API_URL || '';

export type StaleStreamEvent =
  | { type: 'pipeline_started'; data: { report_id: string; answer_len: number; question?: string } }
  | { type: 'claims_extracted'; data: { count: number; claims: Claim[] } }
  | { type: 'triage_completed'; data: { total: number; active: number; skipped: number } }
  | { type: 'claim_verifying'; data: { claim_id: string; text: string; entity?: string | null } }
  | { type: 'claim_verified'; data: { claim_id: string; verdict?: string | null; freshness_score: number; current_value?: string | null } }
  | { type: 'complete'; data: CheckReport }
  | { type: 'error'; data: { detail: string } };

/**
 * SSE over POST for Stale-Answer Detector
 */
export async function streamCheck(
  body: CheckPayload,
  onEvent: (ev: StaleStreamEvent) => void,
  signal?: AbortSignal
): Promise<void> {
  const url = `${STALE_BASE}/v1/check/stream`;
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  const res = await fetch(url, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
    signal,
  });

  if (!res.ok) {
    let msg = `HTTP error ${res.status}`;
    try {
      const errJson = await res.json();
      if (errJson && typeof errJson.detail === 'string') msg = errJson.detail;
      else if (errJson && typeof errJson.error === 'string') msg = errJson.error;
    } catch {
      // ignore
    }
    throw new ApiError(res.status, msg);
  }

  if (!res.body) {
    throw new ApiError(0, 'Readable stream not supported');
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  try {
    while (true) {
      if (signal?.aborted) {
        await reader.cancel();
        return;
      }

      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      let boundary: number;

      while ((boundary = buffer.indexOf('\n\n')) >= 0) {
        const block = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);

        // Lines starting with : are keep-alives - ignore them
        const lines = block.split('\n').filter((l) => !l.startsWith(':') && l.trim().length > 0);
        let eventName = '';
        let dataStr = '';

        for (const line of lines) {
          if (line.startsWith('event:')) {
            eventName = line.slice(6).trim();
          } else if (line.startsWith('data:')) {
            dataStr = line.slice(5).trim();
          }
        }

        if (eventName && dataStr) {
          try {
            const data = JSON.parse(dataStr);
            onEvent({ type: eventName, data } as StaleStreamEvent);
            if (eventName === 'complete' || eventName === 'error') {
              await reader.cancel();
              return;
            }
          } catch (e) {
            console.warn('Failed parsing stream chunk:', e);
          }
        }
      }
    }
  } catch (err) {
    if (signal?.aborted) return;
    throw new ApiError(0, err instanceof Error && err.message ? err.message : 'Stream read error');
  } finally {
    try {
      reader.releaseLock();
    } catch {
      // ignore
    }
  }
}

