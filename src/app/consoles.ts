import { THIS_PRODUCT } from './product';

export type Product = 'replaylab' | 'stale';

export const PRODUCT_NAMES: Record<Product, string> = {
  replaylab: 'Bifrost Replay Lab',
  stale: 'Stale-Answer Detector',
};

/** The route every product opens on. */
export const PRODUCT_HOME: Record<Product, string> = {
  replaylab: '/replaylab/captures',
  stale: '/stale/check',
};

/**
 * Where each product's console is served. The two consoles share one shell, so
 * links to the other product's pages leave this app for that console.
 */
const CONSOLE_URLS: Record<Product, string> = {
  replaylab: import.meta.env.VITE_REPLAYLAB_CONSOLE_URL || 'http://localhost:3000',
  stale: import.meta.env.VITE_STALE_CONSOLE_URL || 'http://localhost:3001',
};

export function productOfPath(path: string): Product | null {
  if (path.startsWith('/replaylab')) return 'replaylab';
  if (path.startsWith('/stale')) return 'stale';
  return null;
}

/** True when the path is served by this console rather than the other one. */
export function isLocalPath(path: string): boolean {
  const product = productOfPath(path);
  return product === null || product === THIS_PRODUCT;
}

/** An href for a console path: relative here, absolute for the other console. */
export function consoleHref(path: string): string {
  if (isLocalPath(path)) return path;
  const product = productOfPath(path) as Product;
  return `${CONSOLE_URLS[product]}${path}`;
}
