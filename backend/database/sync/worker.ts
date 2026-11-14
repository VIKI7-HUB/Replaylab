import type { Config } from '../lib/config.js';
import type { Sql } from '../lib/db.js';
import { errMessage, log } from '../lib/log.js';
import { syncCaptures } from './captures.js';
import { syncExperiments } from './experiments.js';
import { syncReports } from './reports.js';
import { reconcileRetention } from './retention.js';
import { ReplayLabClient, StaleClient } from './sources.js';
import { recordError, type Source } from './state.js';
import { refreshViews, type View } from './views.js';

interface Stream {
  source: Source;
  views: View[];
  run: () => Promise<number>;
}

/** Exponential backoff with full jitter: 1 s doubling to 60 s. */
export function backoffMs(failures: number, rand: () => number = Math.random): number {
  const ceiling = Math.min(60_000, 1000 * 2 ** Math.max(0, failures - 1));
  return Math.round(ceiling / 2 + (rand() * ceiling) / 2);
}

interface StreamState {
  failures: number;
  nextAttemptAt: number;
}

export class SyncWorker {
  private streams: Stream[] = [];
  private state = new Map<Source, StreamState>();
  private stopped = false;
  private timer: NodeJS.Timeout | undefined;
  private wake: (() => void) | undefined;

  constructor(private sql: Sql, private cfg: Config) {
    const rl = new ReplayLabClient(cfg);
    const stale = new StaleClient(cfg);
    if (cfg.syncReplaylab) {
      this.streams.push({
        source: 'replaylab.captures',
        views: ['mv_rl_daily'],
        run: async () => (await syncCaptures(sql, rl)) + (await reconcileRetention(sql, rl)),
      });
      this.streams.push({
        source: 'replaylab.experiments',
        views: ['mv_rl_verdicts'],
        run: () => syncExperiments(sql, rl),
      });
    }
    if (cfg.syncStale) {
      this.streams.push({ source: 'stale.reports', views: ['mv_sad_daily'], run: () => syncReports(sql, stale) });
    }
    for (const s of this.streams) this.state.set(s.source, { failures: 0, nextAttemptAt: 0 });
  }

  /** One pass over every stream that is due. Streams run concurrently and fail independently. */
  async cycle(): Promise<Record<string, number | string>> {
    const started = Date.now();
    const counts: Record<string, number | string> = {};
    const dirty: View[] = [];

    // The two Replay Lab streams share one backend; run them one after the other,
    // and the Stale stream alongside.
    const groups = new Map<string, Stream[]>();
    for (const s of this.streams) {
      const key = s.source.split('.')[0]!;
      groups.set(key, [...(groups.get(key) ?? []), s]);
    }

    await Promise.all(
      [...groups.values()].map(async (group) => {
        for (const s of group) {
          const st = this.state.get(s.source)!;
          if (Date.now() < st.nextAttemptAt) {
            counts[s.source] = 'backoff';
            continue;
          }
          try {
            const n = await s.run();
            counts[s.source] = n;
            st.failures = 0;
            st.nextAttemptAt = 0;
            if (n > 0) dirty.push(...s.views);
          } catch (err) {
            st.failures += 1;
            const wait = backoffMs(st.failures);
            st.nextAttemptAt = Date.now() + wait;
            const message = errMessage(err);
            counts[s.source] = 'error';
            log.warn('sync stream failed', { source: s.source, error: message, failures: st.failures, retry_in_ms: wait });
            await recordError(this.sql, s.source, message).catch(() => {});
          }
        }
      }),
    );

    let refreshed: View[] = [];
    if (dirty.length) {
      try {
        refreshed = await refreshViews(this.sql, dirty);
      } catch (err) {
        log.error('view refresh failed', { error: errMessage(err) });
      }
    }
    log.info('sync cycle', { ...counts, refreshed, ms: Date.now() - started });
    return counts;
  }

  async run(): Promise<void> {
    log.info('sync worker started', {
      streams: this.streams.map((s) => s.source),
      interval_ms: this.cfg.syncIntervalMs,
    });
    while (!this.stopped) {
      try {
        await this.cycle();
      } catch (err) {
        log.error('sync cycle crashed', { error: errMessage(err) });
      }
      if (this.stopped) break;
      await new Promise<void>((resolve) => {
        this.wake = resolve;
        this.timer = setTimeout(resolve, this.nextDelayMs());
      });
    }
  }

  /**
   * How long to sleep before the next cycle: the normal interval, or sooner
   * when a failed stream's backoff runs out first, so a backend that was
   * waking up (or briefly down) is retried in seconds, not one interval later.
   */
  nextDelayMs(now = Date.now()): number {
    let delay = this.cfg.syncIntervalMs;
    for (const st of this.state.values()) {
      if (st.failures > 0) delay = Math.min(delay, Math.max(0, st.nextAttemptAt - now));
    }
    return delay;
  }

  stop() {
    this.stopped = true;
    if (this.timer) clearTimeout(this.timer);
    this.wake?.();
  }
}
