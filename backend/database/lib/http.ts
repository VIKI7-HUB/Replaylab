export class HttpError extends Error {
  constructor(public status: number, public url: string, body: string) {
    super(`GET ${url} -> ${status}${body ? `: ${body.slice(0, 200)}` : ''}`);
  }
}

export async function getJson(url: string, opts: { headers?: Record<string, string>; timeoutMs: number }): Promise<unknown> {
  const res = await fetch(url, {
    headers: { accept: 'application/json', ...opts.headers },
    signal: AbortSignal.timeout(opts.timeoutMs),
  });
  if (!res.ok) throw new HttpError(res.status, url, await res.text().catch(() => ''));
  return res.json();
}
