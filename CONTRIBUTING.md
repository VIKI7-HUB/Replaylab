# Contributing

Several people change this repo at once: one redesigns the console, one builds the database, one keeps the backends and the wiring working. These rules let everyone move fast without breaking each other's work or the live site.

## The three rules

1. **Never push straight to `main`.** Make a branch, push it, open a pull request. `main` is protected and accepts a change only when CI passes.
2. **Stay in your folder.** A pull request that touches someone else's folder automatically asks them to review it; wait for them.
3. **No secrets in git.** Keys go in `.env` / `.env.local` (git-ignored) locally, and in the Render or Vercel dashboard for the live site. CI rejects committed `.env` and `.db` files and anything that looks like an API key.

## Day to day

```bash
git checkout main && git pull
git checkout -b design/new-sidebar      # any name
# ...change things...
git add -A && git commit -m "Describe the change"
git push -u origin design/new-sidebar
# open the pull request on GitHub; merge when the checks are green
```

Every pull request also gets a **Vercel preview link** (posted on the pull request), so you can see a console change live before it merges.

## Who owns what

| Folder | Change freely | Needs care |
| :--- | :--- | :--- |
| `frontend/` | Styles (`src/styles/`, `tailwind.config.ts`, `index.html`), components (`src/components/`), layout (`src/app/Sidebar.tsx`, `TopBar.tsx`, `AppShell.tsx`, `OverviewPage.tsx`, `CommandPalette.tsx`), the JSX and styling in `src/features/` | **Wiring**: `src/api/`, `src/app/routes.tsx`, `src/app/EngineGate.tsx`, `vite.config.ts`, `.env.example`. Owned by the backend owner; GitHub asks them to review any pull request touching these. |
| `backend/replaylab/`, `backend/stale/` | Backend owner | Response shapes: the console reads them. Adding a field is safe; renaming or removing one breaks the console. |
| `backend/database/` | Database team | The Insights API contract in `backend/database/README.md`, which the console already calls. |
| `.github/`, `render.yaml` | Backend owner | These decide what CI checks and how things deploy. |

### For console design changes

- Get data only through `src/api/` (the react-query hooks the pages already use). A page that calls `fetch` itself skips the checks that protect it.
- If a page needs a field the backend doesn't send, ask the backend owner; don't invent it in the schema.
- Run before pushing, from `frontend/`: `npm run lint`, `npm test`, `npm run build`. CI runs the same.

### For backend changes

- After changing a response shape, run the contract check locally: `bash .github/scripts/record-fixtures.sh /tmp/fixtures`, then from `frontend/`: `CONTRACT_FIXTURES_DIR=/tmp/fixtures npx vitest run src/tests/contract.test.ts`. If the console needs a matching change, make it in the same pull request.

### For database work

- Work only in `backend/database/`. Read the services through their HTTP APIs, never their SQLite files.
- When you're ready to deploy, add a service to `render.yaml` with `buildFilter` on `backend/database/**`, and a job to `.github/workflows/ci.yml`.

## If something breaks on the live site

- **Console:** in Vercel, open the project's Deployments and use "Instant Rollback" on the last good one. Then fix forward with a pull request.
- **A backend service:** in Render, open the service's Events and roll back to the previous deploy.
- To undo a merged change in git: `git revert <commit>` in a new pull request. Never force-push `main`.
