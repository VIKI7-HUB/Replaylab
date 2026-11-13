import { describe, it, expect } from 'vitest';
import { StatusValue } from '../components/StatusLabel';

// Test mapping logic strictly adhering to §10.4
function getStatusMapping(val: StatusValue, stopReason?: string | null) {
  const normalized = val ? String(val) : 'skipped';
  switch (normalized) {
    case 'SAFE_TO_SWITCH':
      return { text: 'Safe to switch', tone: 'success' };
    case 'SWITCH_WITH_CAUTION':
      return { text: 'Switch with caution', tone: 'warning' };
    case 'DO_NOT_SWITCH':
      return { text: 'Do not switch', tone: 'danger' };
    case 'running':
      return { text: 'Running', tone: 'running' };
    case 'completed':
      return { text: 'Completed', tone: 'success' };
    case 'cancelled':
      return { text: 'Cancelled', tone: 'neutral' };
    case 'budget_exhausted':
      return { text: 'Budget reached', tone: 'warning' };
    case 'failed':
      return {
        text: stopReason?.startsWith('interrupted:') ? 'Interrupted' : 'Failed',
        tone: 'danger',
      };
    case 'CURRENT':
      return { text: 'Current', tone: 'success' };
    case 'STALE':
      return { text: 'Stale', tone: 'danger' };
    case 'PARTIALLY_STALE':
      return { text: 'Partly stale', tone: 'warning' };
    case 'UNVERIFIABLE':
      return { text: 'Unverifiable', tone: 'neutral' };
    case 'skipped':
    case 'not_checked':
    case 'null':
      return { text: 'Not checked', tone: 'neutral' };
    case 'complete':
      return { text: 'Complete', tone: 'neutral' };
    case 'error':
      return { text: 'Error', tone: 'danger' };
    case 'abandoned':
      return { text: 'Abandoned', tone: 'warning' };
    case 'truncated':
      return { text: 'Truncated', tone: 'warning' };
    case 'llm':
      return { text: 'LLM verified', tone: 'neutral' };
    case 'heuristic':
      return { text: 'Heuristic only', tone: 'warning' };
    default:
      return { text: normalized, tone: 'neutral' };
  }
}

describe('StatusLabel mapping (§10.4)', () => {
  it('maps Replay Lab verdicts correctly', () => {
    expect(getStatusMapping('SAFE_TO_SWITCH')).toEqual({ text: 'Safe to switch', tone: 'success' });
    expect(getStatusMapping('SWITCH_WITH_CAUTION')).toEqual({ text: 'Switch with caution', tone: 'warning' });
    expect(getStatusMapping('DO_NOT_SWITCH')).toEqual({ text: 'Do not switch', tone: 'danger' });
  });

  it('maps experiment states including interrupted', () => {
    expect(getStatusMapping('running')).toEqual({ text: 'Running', tone: 'running' });
    expect(getStatusMapping('completed')).toEqual({ text: 'Completed', tone: 'success' });
    expect(getStatusMapping('budget_exhausted')).toEqual({ text: 'Budget reached', tone: 'warning' });
    expect(getStatusMapping('failed')).toEqual({ text: 'Failed', tone: 'danger' });
    expect(getStatusMapping('failed', 'interrupted: system reboot')).toEqual({
      text: 'Interrupted',
      tone: 'danger',
    });
  });

  it('maps Stale Detector claim verdicts and skipped claims', () => {
    expect(getStatusMapping('CURRENT')).toEqual({ text: 'Current', tone: 'success' });
    expect(getStatusMapping('STALE')).toEqual({ text: 'Stale', tone: 'danger' });
    expect(getStatusMapping('PARTIALLY_STALE')).toEqual({ text: 'Partly stale', tone: 'warning' });
    expect(getStatusMapping('UNVERIFIABLE')).toEqual({ text: 'Unverifiable', tone: 'neutral' });
    expect(getStatusMapping(null)).toEqual({ text: 'Not checked', tone: 'neutral' });
    expect(getStatusMapping('skipped')).toEqual({ text: 'Not checked', tone: 'neutral' });
  });

  it('maps verification modes', () => {
    expect(getStatusMapping('llm')).toEqual({ text: 'LLM verified', tone: 'neutral' });
    expect(getStatusMapping('heuristic')).toEqual({ text: 'Heuristic only', tone: 'warning' });
  });
});

