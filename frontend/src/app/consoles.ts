export type Product = 'replaylab' | 'stale';

/** The console as a whole: one app that serves both products. */
export const CONSOLE_NAME = 'LLM Reliability Console';

export const PRODUCT_NAMES: Record<Product, string> = {
  replaylab: 'Bifrost Replay Lab',
  stale: 'Stale-Answer Detector',
};

/** The route every product opens on. */
export const PRODUCT_HOME: Record<Product, string> = {
  replaylab: '/replaylab/captures',
  stale: '/stale/check',
};

/** The product a route belongs to, or null for a route outside both. */
export function productOfPath(path: string): Product | null {
  if (path.startsWith('/replaylab')) return 'replaylab';
  if (path.startsWith('/stale')) return 'stale';
  return null;
}
