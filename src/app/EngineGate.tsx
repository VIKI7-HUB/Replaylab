import React, { useEffect, useRef, useState } from 'react';
import { CheckCircledIcon } from '@radix-ui/react-icons';
import { PageHeader } from './PageHeader';
import { Panel } from '../components/Panel';
import { StatusLabel } from '../components/StatusLabel';
import { ErrorState } from '../components/States';
import { PRODUCT_SERVICE_URL, productHealth } from '../api/health';
import { PRODUCT_NAMES } from './consoles';
import { THIS_PRODUCT } from './product';

/**
 * Holds the pages back until the product's service answers its health check.
 *
 * Hosted services sleep when idle, and the first request after that waits while
 * the service boots. Instead of every page showing a hung skeleton or a failed
 * request, the console shows that the engine is starting, then opens the page
 * once the service is up. A service that is already awake answers in well under
 * a second, so the gate is never seen.
 */

const SHOW_AFTER_MS = 1000; // a warm service answers before this, so nothing flashes
const RETRY_EVERY_MS = 3000;
const ATTEMPT_TIMEOUT_MS = 20000;
const GIVE_UP_AFTER_MS = 120000;
const TYPICAL_START_MS = 60000; // the progress bar fills over this, then holds

type Phase = 'checking' | 'starting' | 'ready' | 'failed';

// Once the service has answered, later mounts (route changes) skip the check.
let serviceAwake = false;

function healthyWithin(ms: number): Promise<boolean> {
  return Promise.race([
    productHealth[THIS_PRODUCT]().then((h) => h.status === 'healthy'),
    new Promise<boolean>((resolve) => setTimeout(() => resolve(false), ms)),
  ]);
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

export const EngineGate: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [phase, setPhase] = useState<Phase>(serviceAwake ? 'ready' : 'checking');
  const [elapsedMs, setElapsedMs] = useState(0);
  const [attempt, setAttempt] = useState(0); // bumping it restarts the wait after a failure
  const startedAt = useRef(Date.now());

  useEffect(() => {
    if (serviceAwake) return;
    let cancelled = false;
    startedAt.current = Date.now();
    setElapsedMs(0);
    setPhase('checking');

    const showTimer = setTimeout(() => {
      if (!cancelled) setPhase((p) => (p === 'checking' ? 'starting' : p));
    }, SHOW_AFTER_MS);
    const tick = setInterval(() => {
      if (!cancelled) setElapsedMs(Date.now() - startedAt.current);
    }, 1000);

    (async () => {
      while (!cancelled) {
        if (await healthyWithin(ATTEMPT_TIMEOUT_MS)) {
          serviceAwake = true;
          if (!cancelled) setPhase('ready');
          return;
        }
        if (Date.now() - startedAt.current >= GIVE_UP_AFTER_MS) {
          if (!cancelled) setPhase('failed');
          return;
        }
        await sleep(RETRY_EVERY_MS);
      }
    })();

    return () => {
      cancelled = true;
      clearTimeout(showTimer);
      clearInterval(tick);
    };
  }, [attempt]);

  if (phase === 'ready') return <>{children}</>;
  if (phase === 'checking') return null;

  const productName = PRODUCT_NAMES[THIS_PRODUCT];
  const serviceUrl = PRODUCT_SERVICE_URL[THIS_PRODUCT];

  if (phase === 'failed') {
    return (
      <div className="max-w-[560px]">
        <PageHeader title="The engine didn't start" description={`${productName} is still not answering.`} />
        <Panel noPadding>
          <ErrorState
            title="No response from the service"
            message={`${serviceUrl} didn't answer within ${GIVE_UP_AFTER_MS / 60000} minutes.`}
            onRetry={() => setAttempt((n) => n + 1)}
          />
        </Panel>
      </div>
    );
  }

  const seconds = Math.floor(elapsedMs / 1000);
  const fill = Math.min(elapsedMs / TYPICAL_START_MS, 0.95) * 100;
  const slow = elapsedMs > TYPICAL_START_MS;

  const steps: { label: string; state: 'done' | 'active' | 'pending' }[] = [
    { label: 'Request sent', state: 'done' },
    { label: 'Service starting', state: 'active' },
    { label: `Open ${productName}`, state: 'pending' },
  ];

  return (
    <div className="max-w-[560px]" role="status" aria-live="polite">
      <PageHeader
        title="Starting the engine"
        description={`${productName} sleeps when it's idle. Your first request wakes it up.`}
      />

      <Panel>
        <div className="flex items-center justify-between">
          <StatusLabel value="running" customText="Starting" />
          <span className="mono-ui text-[13px] text-text-2 tabular-nums">{seconds} s</span>
        </div>

        <div className="mt-3 w-full h-1.5 bg-bg-muted overflow-hidden">
          <div className="h-full bg-text transition-all duration-150" style={{ width: `${fill}%` }} />
        </div>
        <div className="mt-2 text-[12px] text-text-3">
          {slow ? 'Taking longer than usual. Still waiting.' : 'Usually under a minute.'}
        </div>

        <ol className="mt-4 border-t border-border divide-y divide-border">
          {steps.map((step) => (
            <li key={step.label} className="h-9 flex items-center gap-2 text-[13px]">
              <span className="w-[15px] h-[15px] inline-flex items-center justify-center shrink-0">
                {step.state === 'done' ? (
                  <CheckCircledIcon className="w-[15px] h-[15px] text-success" />
                ) : step.state === 'active' ? (
                  <span className="w-1.5 h-1.5 rounded-full bg-accent animate-running-dot" />
                ) : (
                  <span className="w-1.5 h-1.5 rounded-full bg-border-strong" />
                )}
              </span>
              <span className={step.state === 'pending' ? 'text-text-3' : 'text-text'}>{step.label}</span>
            </li>
          ))}
        </ol>

        <div className="mt-3 pt-3 border-t border-border text-[12px] text-text-3">
          This page opens on its own when the service is ready. Waiting on{' '}
          <span className="mono-ui text-text-2">{serviceUrl}</span>
        </div>
      </Panel>
    </div>
  );
};
