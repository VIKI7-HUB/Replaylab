# LLM Reliability Console

One operator console for two products:

- **Bifrost Replay Lab**: Captures, Experiments (new, detail with live progress) and Settings.
- **Stale-Answer Detector**: Check an answer, Report and History.

Both products are pages of this one app, under `/replaylab/*` and `/stale/*`, and share its shell (sidebar, top bar, command palette and components). Each product talks to its own service, and a product's pages wait for that service to wake before they open (`src/app/EngineGate.tsx`).

The same `frontend/` folder is kept in both the Replay Lab and the Stale-Answer Detector repositories, so each repository runs on its own. Change it in one and copy it to the other.

## Commands

| Command | What it does |
| :--- | :--- |
| `npm run dev` | Dev server on http://localhost:3000 |
| `npm run build` | Type-check, then build to `dist/` |
| `npm run lint` | ESLint |
| `npm test` | Unit tests (Vitest) |
| `npm run e2e` | Playwright, against the running console and both backends |

## Configuration

Copy `.env.example` to `.env.local` and change what differs on your machine. `.env.example` documents every setting.

The top bar shows the health of both services. The Insights read API is optional: without it, search is unavailable and everything else works.
