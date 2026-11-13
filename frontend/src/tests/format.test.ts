import { describe, it, expect } from 'vitest';
import {
  formatMoney,
  formatPercent,
  formatPercentDelta,
  formatDuration,
  formatCount,
  formatTokens,
  formatIsoUtc,
} from '../lib/format';

describe('formatMoney (§10.13)', () => {
  it('formats under $0.01 with 4 decimals', () => {
    expect(formatMoney(0.0028)).toBe('$0.0028');
    expect(formatMoney(0.0001)).toBe('$0.0001');
  });

  it('formats under $1,000 with 2 decimals', () => {
    expect(formatMoney(3.19)).toBe('$3.19');
    expect(formatMoney(0.05)).toBe('$0.05');
    expect(formatMoney(999.99)).toBe('$999.99');
  });

  it('formats >= $1,000 compactly', () => {
    expect(formatMoney(12400)).toBe('$12.4k');
    expect(formatMoney(100000)).toBe('$100k');
  });

  it('handles zero and nulls', () => {
    expect(formatMoney(0)).toBe('$0.00');
    expect(formatMoney(null)).toBe('—');
  });
});

describe('formatPercent and formatPercentDelta (§10.13)', () => {
  it('formats percentages with 1 decimal', () => {
    expect(formatPercent(94.123)).toBe('94.1%');
  });

  it('formats deltas with signed prefix using U+2212 minus sign', () => {
    expect(formatPercentDelta(12.3)).toBe('+12.3%');
    expect(formatPercentDelta(-42.7)).toBe('−42.7%');
    expect(formatPercentDelta(0)).toBe('0.0%');
  });
});

describe('formatDuration (§10.13)', () => {
  it('formats under 1,000 ms', () => {
    expect(formatDuration(23)).toBe('23 ms');
  });

  it('formats under 60 s with 1 decimal', () => {
    expect(formatDuration(1400)).toBe('1.4 s');
  });

  it('formats >= 60 s in minutes and seconds', () => {
    expect(formatDuration(125000)).toBe('2m 05s');
  });
});

describe('formatCount and formatTokens (§10.13)', () => {
  it('formats counts with thousands separators', () => {
    expect(formatCount(1240)).toBe('1,240');
  });

  it('formats tokens compactly above 10k', () => {
    expect(formatTokens(5000)).toBe('5,000');
    expect(formatTokens(12400)).toBe('12.4k');
  });
});

describe('formatIsoUtc (§10.13)', () => {
  it('formats UTC timestamps with YYYY-MM-DD HH:mm:ss UTC in mono-ui', () => {
    const d = new Date('2026-11-08T09:00:12Z');
    expect(formatIsoUtc(d)).toBe('2026-11-08 09:00:12 UTC');
  });
});

