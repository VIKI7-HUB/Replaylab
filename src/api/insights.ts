import { z } from 'zod';
import { ApiError, normalizeError } from './error';

// The read API owned by the database team (ANTIGRAVITY_DATABASE_PROMPT.md §5.4).
// The console works without it; only search depends on it.
const BASE_URL = import.meta.env.VITE_INSIGHTS_URL || '';

const SEARCH_KINDS = ['capture', 'experiment', 'report', 'claim'] as const;

const SearchHitSchema = z.object({
  kind: z.enum(SEARCH_KINDS),
  id: z.string(),
  title: z.string(),
  snippet: z.string().optional().nullable(),
  at: z.string(),
});

export interface SearchResultItem {
  kind: string;
  id: string;
  title: string;
  at: string;
  /** Console path to open the hit, in whichever console serves it. */
  url: string;
}

const KIND_LABELS: Record<(typeof SEARCH_KINDS)[number], string> = {
  capture: 'Capture',
  experiment: 'Experiment',
  report: 'Report',
  claim: 'Claim',
};

function hitPath(kind: (typeof SEARCH_KINDS)[number], id: string): string {
  const encoded = encodeURIComponent(id);
  switch (kind) {
    case 'capture':
      return `/replaylab/captures/${encoded}`;
    case 'experiment':
      return `/replaylab/experiments/${encoded}`;
    case 'report':
      return `/stale/reports/${encoded}`;
    case 'claim':
      // A search hit for a claim doesn't carry its report, so open the history.
      return '/stale/history';
  }
}

async function getJson(path: string): Promise<unknown> {
  try {
    const res = await fetch(`${BASE_URL}${path}`);
    if (!res.ok) throw new ApiError(res.status, `Insights error ${res.status}`);
    return await res.json();
  } catch (err) {
    throw normalizeError(err, 'Insights API', BASE_URL);
  }
}

export async function searchGlobal(q: string, limit: number = 20): Promise<SearchResultItem[]> {
  if (!q.trim() || !BASE_URL) return [];
  const data = await getJson(`/api/search?q=${encodeURIComponent(q)}&limit=${limit}`);
  const hits = z.array(SearchHitSchema).parse(data);
  return hits.map((hit) => ({
    kind: KIND_LABELS[hit.kind],
    id: hit.id,
    title: hit.title,
    at: hit.at,
    url: hitPath(hit.kind, hit.id),
  }));
}
