# Operator Console

A production-grade, engineering-focused operator console for **Bifrost Replay Lab**, **Stale-Answer Detector**, and the **Insights Analytics API**.

Built with React 18, TypeScript, Tailwind CSS v3, and Radix primitives in strict adherence to the engineering guidelines. Follows a clean, monochromatic, high-density design inspired by Vercel, Braintrust, and Langfuse.

---

## 1. Features & Architecture

- **Overview (`/`)**: Real-time traffic, experiment verdicts, and answer freshness metrics across both backend services. Automatically switches to graceful "Analytics offline" mode when the Insights API is unreachable while keeping live features functional.
- **Replay Lab**:
  - **Captures (`/replaylab/captures`)**: Search, filter, and inspect gateway-recorded LLM requests. Detailed slide-in drawer (`/replaylab/captures/:id`) with keyboard navigation (`↑`/`↓`/`Esc`), PII redaction token highlights, raw parameter & response viewer, and syntax-highlighted code blocks.
  - **Experiments List (`/replaylab/experiments`)**: Overview of active and completed candidate replay runs with live auto-refreshing polling.
  - **New Experiment (`/replaylab/experiments/new`)**: Configure baseline models, candidate selections, budget caps, sampling limits, and optional LLM-as-a-judge rubrics. Features a live dry-run cost estimation panel.
  - **Experiment Detail (`/replaylab/experiments/:id`)**: Real-time SSE progress streaming (`EventSource`), Braintrust-style metric comparison table with delta indicators, candidate verdict panels, latency & cost distribution charts, and LLM judge sample inspections.
  - **Settings (`/replaylab/settings`)**: Runtime configuration management for sampling rate, opt-in/opt-out virtual keys, PII redaction, retention, and budget caps with an unsaved changes bar.
- **Stale-Answer Detector**:
  - **Check an Answer (`/stale/check`)**: Interactive claim freshness auditor. Reads streaming SSE over POST (`fetch` stream reader) to track pipeline progress across extraction, risk triage, claim verification, scoring, and correction.
  - **Report (`/stale/reports/:id`)**: In-place sentence highlights with verdict color underlines, claim breakdown accordion with evidence quotes and authority scores, and tabbed view of the corrected answer with superscript citation links and diff changes.
  - **History (`/stale/history`)**: Paginated audit log of previously verified answers.
- **Global Shell & Navigation**: Collapsible sidebar, health status polling dots (15s interval) with tooltips, breadcrumb tracking, light/dark theme switching, and global command palette (`⌘K` / `Ctrl+K`).

---

## 2. Environment Variables

Configure backend connection endpoints via `.env`:

| Variable | Default Value | Description |
| :--- | :--- | :--- |
| `VITE_REPLAYLAB_URL` | `http://localhost:8086` | Bifrost Replay Lab gateway endpoint (`:8085` for sidecar mode) |
| `VITE_STALE_URL` | `http://localhost:8000` | Stale-Answer Detector FastAPI backend |
| `VITE_STALE_API_KEY` | *(optional)* | Optional `X-API-Key` header for Stale Detector authentication |
| `VITE_INSIGHTS_URL` | `http://localhost:8090` | Insights Analytics read API service |

---

## 3. How to Run Against the Backends

### Prerequisites
- Node.js (v18+)
- npm (v9+)
- Python 3.10+ (for Stale Detector)
- Docker & Docker Compose (optional, for Bifrost Replay Lab)

### Step 1: Install Frontend Dependencies
```bash
cd frontend
npm install
```

### Step 2: Start the Stale-Answer Detector Backend
In the workspace root:
```bash
# Windows
run.bat

# Or directly with uvicorn
python -m uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload
```

### Step 3: Start Bifrost Replay Lab Demo
If running Docker:
```bash
docker compose -f bifrost-backend/loadable/compose.yml up --build
```
Ensure `VITE_REPLAYLAB_URL` points to `http://localhost:8086`.

### Step 4: Start Frontend Dev Server
```bash
cd frontend
npm run dev
```
Open [http://localhost:3000](http://localhost:3000) in your browser.

---

## 4. Scripts

- `npm run dev`: Starts Vite dev server on port 3000
- `npm run build`: Typechecks with `tsc --noEmit` and bundles via Vite
- `npm run typecheck`: Runs strict TypeScript typecheck
- `npm run test`: Runs Vitest test suite (formatting, SSE parsers, error normalization, design compliance)
- `npm run preview`: Serves production build locally

---

## 5. Design System Compliance & Tokens

- **Strict Monochrome**: All colors are governed by CSS variables defined in `src/styles/tokens.css` (neutral base, semantic colors reserved exclusively for meaning).
- **Zero Banned Elements**: No gradients, shadows on panels, frosted overlays, Lucide/Heroicon icons, emojis, animated counters, or unauthorized fonts.
- **Typography**: Powered exclusively by `@fontsource-variable/geist` and `@fontsource-variable/geist-mono` with tabular figures on numbers.
- **Motion**: Durations capped at 120–160ms; respects `prefers-reduced-motion`.

---

## 6. Limitations & Notes

- **Offline Analytics**: When `VITE_INSIGHTS_URL` is unreachable, the Overview screen displays the neutral "Analytics offline" state with direct action buttons to live features (Captures and Check).
- **Backend File Immutability**: All files in `app/`, `tests/`, `docs/`, `Makefile`, and `run.bat` have remained strictly untouched.

