export class HttpError extends Error {
  constructor(public status: number, public url: string, body: string) {
    super(`GET ${url} -> ${status}${summarize(body)}`);
  }
}

/** A short, single-line reason from an error body; HTML pages (proxies, wake-up screens) shrink to their title. */
function summarize(body: string): string {
  const text = body.trim();
  if (!text) return '';
  if (text.startsWith('<')) {
    const title = /<title>([^<]*)<\/title>/i.exec(text)?.[1]?.trim();
    return title ? `: ${title}` : ': (html page)';
  }
  return `: ${text.replace(/\s+/g, ' ').slice(0, 200)}`;
}

const USER_AGENT = 'llm-reliability-insights-sync/1.0';

export async function getJson(url: string, opts: { headers?: Record<string, string>; timeoutMs: number }): Promise<unknown> {
  const res = await fetch(url, {
    headers: { accept: 'application/json', 'user-agent': USER_AGENT, ...opts.headers },
    signal: AbortSignal.timeout(opts.timeoutMs),
  });
  if (!res.ok) throw new HttpError(res.status, url, await res.text().catch(() => ''));
  return res.json();
}
