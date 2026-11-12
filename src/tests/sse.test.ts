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

});

