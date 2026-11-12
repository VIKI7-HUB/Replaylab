import { ProgressEvent, ProgressEventSchema } from './replaylab';
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
