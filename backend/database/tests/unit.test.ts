import { describe, expect, it } from 'vitest';
import { decodeCursor, encodeCursor, BadRequest } from '../api/paging.js';
import { likePattern, tsQuery } from '../api/text.js';
import { captureRow, claimRows, experimentResultRows, responseText } from '../sync/map.js';
import { CaptureSchema, ExperimentSchema, ReportSchema } from '../sync/sources.js';
import { backoffMs, SyncWorker } from '../sync/worker.js';
import { loadConfig } from '../lib/config.js';

describe('tsQuery', () => {
  it('prefix-matches words of 3+ characters and keeps versions whole', () => {
    expect(tsQuery('Python 3.11')).toBe('python:* & 3.11:*');
    expect(tsQuery('gpt-4o mini')).toBe('gpt-4o:* & mini:*');
    expect(tsQuery('a b')).toBe('a & b');
  });
  it('drops tsquery operators, so any input is valid syntax', () => {
    expect(tsQuery("x' | !(y) & z:*")).toBe('x & y & z');
    expect(tsQuery('!!! ()')).toBeNull();
  });
});

describe('likePattern', () => {
  it('escapes LIKE wildcards', () => {
    expect(likePattern('50%_off')).toBe('%50\\%\\_off%');
  });
});

describe('cursors', () => {
  it('round-trip', () => {
    const c = encodeCursor(['2026-11-14 13:41:22.305123+00', 'cap-1']);
    expect(decodeCursor(c, 2)).toEqual(['2026-11-14 13:41:22.305123+00', 'cap-1']);
  });
  it('reject garbage and wrong arity', () => {
    expect(() => decodeCursor('zzz', 2)).toThrow(BadRequest);
    expect(() => decodeCursor(encodeCursor(['a']), 2)).toThrow(BadRequest);
  });
});

describe('backoff', () => {
  it('doubles from 1 s and caps at 60 s, with jitter in the upper half', () => {
    expect(backoffMs(1, () => 0)).toBe(500);
    expect(backoffMs(1, () => 1)).toBe(1000);
    expect(backoffMs(3, () => 1)).toBe(4000);
    expect(backoffMs(20, () => 1)).toBe(60000);
  });
});

describe('retry scheduling', () => {
  it('retries a failed stream on its backoff instead of waiting a whole interval', async () => {
    const cfg = loadConfig({ SYNC_INTERVAL_MS: '1200000', REPLAYLAB_URL: 'http://127.0.0.1:9', SYNC_STALE: 'false' });
    const worker = new SyncWorker({} as never, cfg);
    expect(worker.nextDelayMs()).toBe(1_200_000);
    // Both Replay Lab streams fail to connect (port 9 is closed); errors are recorded best-effort.
    const fakeSql = Object.assign(async () => [], { json: (v: unknown) => v }) as never;
    (worker as unknown as { sql: unknown }).sql = fakeSql;
    await worker.cycle();
    const delay = worker.nextDelayMs();
    expect(delay).toBeGreaterThanOrEqual(0);
    expect(delay).toBeLessThanOrEqual(1000);
  });
});

describe('mapping', () => {
  it('maps a streamed capture and extracts the answer text', () => {
    const c = CaptureSchema.parse({
      id: 'cap-1', timestamp: '2026-11-14T10:00:00.123456789Z', model: 'gpt-4o', provider: 'openai',
      messages_json: '[{"role":"user","content":"hi"}]', params_json: '', response_json:
        '{"choices":[{"message":{"content":"hello"}}]}', prompt_tokens: 3, completion_tokens: 2, total_tokens: 5,
      latency_ms: 120, cost_estimate: 0.0001, status_code: 200, is_streaming: true, hash_only: false,
      completion_state: 'truncated', time_to_first_token_ms: 40, usage_estimated: true,
    });
    const row = captureRow(c);
    expect(row.response_text).toBe('hello');
    expect(row.messages).toEqual([{ role: 'user', content: 'hi' }]);
    expect(row.params).toBeNull();
    expect(row.ttft_ms).toBe(40);
    expect(row.completion_state).toBe('truncated');
    expect(row.tag).toBe('');
  });
  it('keeps a hash-only payload as a JSON string', () => {
    expect(responseText('sha256:abc')).toBe('sha256:abc');
  });
  it('takes the verdict from results, falling back to recommendations', () => {
    const e = ExperimentSchema.parse({
      id: 'exp-1', status: 'completed', created_at: '2026-11-14T10:00:00Z',
      results: [{ candidate_model: 'a', recommendation: 'SAFE_TO_SWITCH' }, { candidate_model: 'b' }],
      recommendations: [{ candidate_model: 'b', verdict: 'DO_NOT_SWITCH', confidence: 0.9 }],
    });
    const rows = experimentResultRows(e);
    expect(rows.map((r) => r.verdict)).toEqual(['SAFE_TO_SWITCH', 'DO_NOT_SWITCH']);
    expect(rows[1]!.confidence).toBe(0.9);
  });
  it('counts official evidence and nulls a skipped verdict', () => {
    const r = ReportSchema.parse({
      report_id: 'rpt-1', checked_at: '2026-11-14T10:00:00Z', original_answer: 'x', overall_freshness_score: 50,
      claims: [
        { id: 'c1', text: 't', entity: 'e', evidence: [{ is_official: true }, { is_official: false }], verdict: 'STALE' },
        { id: 'c2', text: 't', entity: 'e', skipped: true, verdict: null },
      ],
    });
    const rows = claimRows(r);
    expect(rows[0]).toMatchObject({ evidence_count: 2, official_evidence_count: 1, verdict: 'STALE' });
    expect(rows[1]).toMatchObject({ skipped: true, verdict: null });
  });
});
