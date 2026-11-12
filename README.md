# Bifrost Replay Lab console

The operator console for Bifrost Replay Lab: Captures, Experiments (new, detail with live progress) and Settings.

It is one of two consoles that share the same shell (sidebar, top bar, command palette and components). The sidebar lists both products; the Stale-Answer Detector pages are links to that product's own console. Which product this app serves is set in `src/app/product.ts`.

## Commands

| Command | What it does |
| :--- | :--- |
| `npm run dev` | Dev server on http://localhost:3000 |
| `npm run build` | Type-check, then build to `dist/` |
| `npm run lint` | ESLint |
| `npm test` | Unit tests (Vitest) |
| `npm run e2e` | Playwright, against the running console and backend |

## Configuration

Copy `.env.example` to `.env.local` and change what differs on your machine. Every setting has a default for a local setup.

The top bar shows the health of both backends and of the Insights read API. Insights is optional: without it, search is unavailable and everything else works.
