/**
 * Formatting helpers strictly implementing §10.13:
 * - Money
 * - Percentages (signed deltas start with + or \u2212)
 * - Durations
 * - Counts and Tokens
 * - Timestamps (relative vs ISO UTC)
 */

export function formatMoney(val: number | null | undefined): string {
  if (val === null || val === undefined || isNaN(val)) return '—';
  const abs = Math.abs(val);
  const sign = val < 0 ? '−' : '';
  if (abs === 0) return '$0.00';
  if (abs < 0.01) {
    return `${sign}$${abs.toFixed(4)}`;
  }
  if (abs < 1000) {
    return `${sign}$${abs.toFixed(2)}`;
  }
  if (abs < 1_000_000) {
    const k = abs / 1000;
    return `${sign}$${k >= 100 ? Math.round(k) : k.toFixed(1)}k`;
  }
  const m = abs / 1_000_000;
  return `${sign}$${m.toFixed(1)}M`;
}

export function formatPercent(val: number | null | undefined): string {
  if (val === null || val === undefined || isNaN(val)) return '—';
  return `${val.toFixed(1)}%`;
}

export function formatPercentDelta(val: number | null | undefined): string {
  if (val === null || val === undefined || isNaN(val)) return '—';
  if (val === 0) return '0.0%';
  const sign = val > 0 ? '+' : '−';
  return `${sign}${Math.abs(val).toFixed(1)}%`;
}

export function formatDuration(ms: number | null | undefined): string {
  if (ms === null || ms === undefined || isNaN(ms)) return '—';
  if (ms < 1000) {
    return `${Math.round(ms)} ms`;
  }
  const sec = ms / 1000;
  if (sec < 60) {
    return `${sec.toFixed(1)} s`;
  }
  const minutes = Math.floor(sec / 60);
  const remainingSec = Math.round(sec % 60);
  const paddedSec = remainingSec < 10 ? `0${remainingSec}` : `${remainingSec}`;
  return `${minutes}m ${paddedSec}s`;
}

export function formatCount(val: number | null | undefined): string {
  if (val === null || val === undefined || isNaN(val)) return '—';
  return val.toLocaleString('en-US');
}

export function formatTokens(val: number | null | undefined): string {
  if (val === null || val === undefined || isNaN(val)) return '—';
  if (val < 10_000) {
    return val.toLocaleString('en-US');
  }
  const k = val / 1000;
  return `${k >= 100 ? Math.round(k) : k.toFixed(1)}k`;
}

export function formatRelativeTime(dateStrOrTs: string | number | Date | null | undefined): string {
  if (!dateStrOrTs) return '—';
  const d = new Date(dateStrOrTs);
  if (isNaN(d.getTime())) return '—';
  const now = new Date();
  const diffSec = Math.floor((now.getTime() - d.getTime()) / 1000);

  if (diffSec < 0) return 'just now';
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 7) return `${diffDays}d ago`;

  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}

export function formatIsoUtc(dateStrOrTs: string | number | Date | null | undefined): string {
  if (!dateStrOrTs) return '—';
  const d = new Date(dateStrOrTs);
  if (isNaN(d.getTime())) return '—';
  const pad = (n: number) => String(n).padStart(2, '0');
  const year = d.getUTCFullYear();
  const month = pad(d.getUTCMonth() + 1);
  const day = pad(d.getUTCDate());
  const hours = pad(d.getUTCHours());
  const minutes = pad(d.getUTCMinutes());
  const seconds = pad(d.getUTCSeconds());
  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds} UTC`;
}

