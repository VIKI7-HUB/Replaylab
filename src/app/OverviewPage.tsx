import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { PageHeader } from './PageHeader';
import { Panel } from '../components/Panel';
import { Button } from '../components/Button';
import { StatusLabel } from '../components/StatusLabel';
import { ServiceHealth, productHealth } from '../api/health';
import { CONSOLE_NAME, PRODUCT_HOME, PRODUCT_NAMES, Product } from './consoles';

/**
 * The console's front page: what each product does, whether its service is
 * up, and where to start. It needs nothing but the two health checks, so it
 * opens at once even while a service is still waking.
 */

const PRODUCTS: {
  product: Product;
  summary: string;
  steps: string[];
  action: string;
}[] = [
  {
    product: 'replaylab',
    summary:
      'Records the chat traffic that passes through a Bifrost gateway, replays it against cheaper models under a budget cap, and says whether switching is safe.',
    steps: [
      'Captures: the requests the gateway recorded. Send sample traffic if it is empty.',
      'Experiments: pick a baseline and cheaper candidates, estimate the cost, then run.',
      'Read the verdict per model: quality, errors, latency, cost and projected savings.',
    ],
    action: 'Open Captures',
  },
  {
    product: 'stale',
    summary:
      'Splits an LLM answer into claims, checks each one against the live web, scores how fresh the answer is, and writes a corrected version with sources.',
    steps: [
      'Check: paste an answer, or load the example, and start the check.',
      'Watch the claims being extracted and verified as it runs.',
      'Report: a freshness score, the evidence per claim, and the corrected answer.',
    ],
    action: 'Check an answer',
  },
];

function healthLabel(health: ServiceHealth | null) {
  if (!health) return <StatusLabel value="running" customText="Checking" />;
  if (health.status === 'healthy') return <StatusLabel value="completed" customText="Service up" />;
  // A hosted service that has been idle answers its first request slowly.
  return <StatusLabel value="running" customText="Waking up" />;
}

export const OverviewPage: React.FC = () => {
  const navigate = useNavigate();
  const [health, setHealth] = useState<Record<Product, ServiceHealth | null>>({
    replaylab: null,
    stale: null,
  });

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    // Asking also wakes a sleeping service, so by the time a product is opened it is ready.
    const poll = async () => {
      const [replaylab, stale] = await Promise.all([productHealth.replaylab(), productHealth.stale()]);
      if (cancelled) return;
      setHealth({ replaylab, stale });
      if (replaylab.status !== 'healthy' || stale.status !== 'healthy') timer = setTimeout(poll, 4000);
    };
    poll();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, []);

  return (
    <div className="max-w-[1040px]">
      <PageHeader
        title={CONSOLE_NAME}
        description="Two tools for running LLMs in production: one tells you whether a cheaper model is safe to switch to, the other whether an answer is still true."
      />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {PRODUCTS.map(({ product, summary, steps, action }) => (
          <Panel key={product} title={PRODUCT_NAMES[product]} actions={healthLabel(health[product])}>
            <p className="m-0 text-[13px] leading-5 text-text-2">{summary}</p>
            <ol className="m-0 mt-4 p-0 list-none border-t border-border divide-y divide-border">
              {steps.map((step, i) => (
                <li key={step} className="py-2.5 flex gap-3 text-[13px] leading-5 text-text">
                  <span className="mono-ui text-text-3 tabular-nums shrink-0">{i + 1}</span>
                  <span>{step}</span>
                </li>
              ))}
            </ol>
            <div className="mt-4">
              <Button size="md" variant="primary" onClick={() => navigate(PRODUCT_HOME[product])}>
                {action}
              </Button>
            </div>
          </Panel>
        ))}
      </div>
    </div>
  );
};
