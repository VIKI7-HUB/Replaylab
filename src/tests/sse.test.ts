import { describe, it, expect } from 'vitest';
import { ProgressEventSchema } from '../api/replaylab';

describe('SSE Event Parsing (§15)', () => {
  it('parses Replay Lab progress events', () => {
    const raw = {
      status: 'running',
      candidate_model: 'gpt-4o-mini',
      completed_count: 12,
      total_count: 50,
      progress_percent: 24,
      current_quality: 91.5,
      current_judge_score: 85,
      spent_dollars: 0.0034,
      message: 'Replaying sample 12/50 against gpt-4o-mini',
      final: false,
    };

    const parsed = ProgressEventSchema.parse(raw);
    expect(parsed.progress_percent).toBe(24);
    expect(parsed.candidate_model).toBe('gpt-4o-mini');
    expect(parsed.final).toBe(false);
  });

  it('handles keep-alive lines and stream chunk extraction', () => {
    const chunk = ': keep-alive\n\nevent: triage_completed\ndata: {"total": 4, "active": 3, "skipped": 1}\n\n';
    const blocks = chunk.split('\n\n').filter((b) => b.trim().length > 0);

    const parsedEvents: { event: string; data: Record<string, unknown> }[] = [];
    for (const block of blocks) {
      const lines = block.split('\n').filter((l) => !l.startsWith(':') && l.trim().length > 0);
      let eventName = '';
      let dataStr = '';
      for (const line of lines) {
        if (line.startsWith('event:')) eventName = line.slice(6).trim();
        else if (line.startsWith('data:')) dataStr = line.slice(5).trim();
      }
      if (eventName && dataStr) {
        parsedEvents.push({ event: eventName, data: JSON.parse(dataStr) });
      }
    }

    expect(parsedEvents).toHaveLength(1);
    expect(parsedEvents[0].event).toBe('triage_completed');
    expect(parsedEvents[0].data.active).toBe(3);
    expect(parsedEvents[0].data.skipped).toBe(1);
  });
});
