# LLM Reliability Console

One operator console for two products:

- **Bifrost Replay Lab**: Captures, Experiments (new, detail with live progress) and Settings.
- **Stale-Answer Detector**: Check an answer, Report and History.

Both products are pages of this app, under `/replaylab/*` and `/stale/*`, and share its shell (sidebar, top bar, command palette and components). The services live in `../backend/` and are reached only over HTTP.

## Changing the design (safe) vs changing the wiring (careful)

The look of the console can change as often as you like. These are **design**, and nothing in the backend depends on them:

| Safe to change freely | |
| :--- | :--- |
| `src/styles/tokens.css`, `tailwind.config.ts`, `index.html` | Colors, fonts, spacing, theme |
| `src/components/*` | Buttons, panels, tables, meters, dialogs |
| `src/app/Sidebar.tsx`, `TopBar.tsx`, `AppShell.tsx`, `OverviewPage.tsx`, `CommandPalette.tsx` | Layout and navigation |
| The JSX and styling in `src/features/*` | How each page looks |

These are **wiring**. They decide what the console sends to and expects from the backends. Change them only on purpose, and run `npm test` and `npm run e2e` afterwards:

| Wiring | |
| :--- | :--- |
| `src/api/*.ts` | Every request, and the zod schema each response must match. Schemas mirror the backends' real responses. |
| `src/app/routes.tsx`, `src/app/EngineGate.tsx` | Which URL shows which page, and the wait for a sleeping service |
| `.env.example`, `vite.config.ts` | Where each backend lives |

Pages get their data only through `src/api/*` and react-query hooks. Keep it that way: a page that calls `fetch` itself skips the schema check.

## Commands

| Command | What it does |
| :--- | :--- |
| `npm run dev` | Dev server on http://localhost:3000 |
| `npm run build` | Type-check, then build to `dist/` |
| `npm run lint` | ESLint |
| `npm test` | Unit and API contract tests (Vitest) |
| `npm run e2e` | Playwright, against the running console and both backends |

CI runs lint, tests and build on every push.

## Configuration

Copy `.env.example` to `.env.local` and change what differs on your machine. `.env.example` documents every setting. Every `VITE_` value ends up in the browser, so none may hold a secret.

The top bar shows the health of each service. The Insights read API (the database team's, `VITE_INSIGHTS_URL`) is optional: without it, search is unavailable and everything else works.
