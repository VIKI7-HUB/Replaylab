import { describe, it, expect } from 'vitest';
import { ApiError, normalizeError } from '../api/error';

describe('Error normalization (§15)', () => {
  it('preserves existing ApiError', () => {
    const err = new ApiError(404, 'Not found');
    const norm = normalizeError(err, 'Service', 'http://localhost');
    expect(norm.status).toBe(404);
    expect(norm.message).toBe('Not found');
  });

  it('normalizes fetch network failures into status 0 with formatted message', () => {
    const fetchErr = new TypeError('Failed to fetch');
    const norm = normalizeError(fetchErr, 'Bifrost Replay Lab', 'http://localhost:8086');
    expect(norm.status).toBe(0);
    expect(norm.message).toBe("Can't reach Bifrost Replay Lab at http://localhost:8086");
  });

  it('handles abort errors', () => {
    const abortErr = new DOMException('The user aborted a request.', 'AbortError');
    const norm = normalizeError(abortErr, 'Stale', 'http://localhost:8000');
    expect(norm.status).toBe(0);
    expect(norm.message).toBe('Request aborted');
  });
});

