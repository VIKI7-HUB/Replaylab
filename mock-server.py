"""
Complete standalone server for Bifrost Replay Lab (:8086) and Insights API (:8090)
Allows testing all interactive features of Replay Lab & Overview without Docker.
"""

import asyncio
import json
import uuid
from datetime import datetime, timezone
from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import StreamingResponse

# ==========================================
# BIFROST REPLAY LAB APP (Port 8086)
# ==========================================
replaylab_app = FastAPI(title="Bifrost Replay Lab")
replaylab_app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

MODELS = [
    {"model": "gpt-4o", "input_per_1m_tokens": 2.50, "output_per_1m_tokens": 10.00},
    {"model": "gpt-4o-mini", "input_per_1m_tokens": 0.15, "output_per_1m_tokens": 0.60},
    {"model": "claude-3-5-sonnet", "input_per_1m_tokens": 3.00, "output_per_1m_tokens": 15.00},
    {"model": "meta-llama-3.1-8b", "input_per_1m_tokens": 0.10, "output_per_1m_tokens": 0.20},
    {"model": "meta-llama-3.1-70b", "input_per_1m_tokens": 0.50, "output_per_1m_tokens": 0.90},
]

CONFIG = {
    "enabled": True,
    "sampling_rate": 1.0,
    "opt_in_keys": ["prod-key-1", "chat-service"],
    "opt_out_keys": ["internal-test"],
    "redact_pii": True,
    "store_hashes_only": False,
    "retention_days": 30,
    "default_budget_cap": 10.0,
    "max_stream_capture_bytes": 65536,
    "db_path": "/var/lib/replaylab/captures.db",
    "gateway_url": "http://localhost:8080",
    "max_queue_size": 1000,
    "worker_count": 4,
    "api_listen_addr": "0.0.0.0:8086",
}

CAPTURES = [
    {
        "id": "cap-91a8bf21",
        "created_at": datetime.now(timezone.utc).isoformat(),
        "provider": "openai",
        "model": "gpt-4o",
        "virtual_key": "prod-key-1",
        "tenant": "enterprise-acme",
        "tag": "customer-support",
        "status_code": 200,
        "completion_state": "complete",
        "prompt_tokens": 420,
        "completion_tokens": 145,
        "total_tokens": 565,
        "latency_ms": 780,
        "time_to_first_token_ms": 220,
        "cost_dollars": 0.0025,
        "streamed": True,
        "usage_estimated": False,
        "hash_only": False,
        "error_message": None,
        "messages_json": json.dumps([
            {"role": "system", "content": "You are a customer assistant for Acme Corp."},
            {"role": "user", "content": "How do I update my email address [REDACTED_EMAIL] and credit card [REDACTED_CARD]?"},
            {"role": "assistant", "content": "You can update your account details in Settings -> Billing."},
        ]),
        "params_json": json.dumps({"temperature": 0.7, "max_tokens": 256}),
        "response_json": json.dumps({"reply": "You can update your account details in Settings -> Billing."}),
    },
    {
        "id": "cap-73cb118a",
        "created_at": datetime.now(timezone.utc).isoformat(),
        "provider": "openai",
        "model": "gpt-4o",
        "virtual_key": "prod-key-1",
        "tenant": "enterprise-acme",
        "tag": "data-extractor",
        "status_code": 200,
        "completion_state": "complete",
        "prompt_tokens": 850,
        "completion_tokens": 210,
        "total_tokens": 1060,
        "latency_ms": 1150,
        "time_to_first_token_ms": 310,
        "cost_dollars": 0.0042,
        "streamed": True,
        "usage_estimated": False,
        "hash_only": False,
        "error_message": None,
        "messages_json": json.dumps([
            {"role": "user", "content": "Extract the vendor name and total: Invoice from Stark Industries total $4,500."}
        ]),
        "params_json": json.dumps({"temperature": 0.2}),
        "response_json": json.dumps({"vendor": "Stark Industries", "total": 4500.0}),
    },
]

EXPERIMENTS = [
    {
        "id": "exp-29ae0198-6",
        "name": "gpt-4o vs gpt-4o-mini & llama-3.1",
        "status": "completed",
        "stop_reason": None,
        "original_model": "gpt-4o",
        "candidate_models": ["gpt-4o-mini", "meta-llama-3.1-8b"],
        "total_replays": 100,
        "completed_replays": 100,
        "spent_dollars": 0.045,
        "budget_cap_dollars": 10.0,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "results": [
            {
                "candidate_model": "gpt-4o-mini",
                "total_requests": 100,
                "successful_requests": 100,
                "failed_requests": 0,
                "error_rate": 0.0,
                "avg_quality_score": 96.4,
                "json_validity_rate": 100.0,
                "json_samples": 40,
                "p50_latency_ms": 280,
                "p95_latency_ms": 520,
                "original_avg_latency_ms": 820,
                "candidate_avg_latency_ms": 310,
                "latency_delta_percent": -62.2,
                "original_cost_dollars": 0.4500,
                "candidate_cost_dollars": 0.0265,
                "cost_savings_dollars": 0.4235,
                "cost_savings_percent": 94.1,
                "recommendation": "SAFE_TO_SWITCH",
                "reasoning": "Maintains 96.4% quality with 100% JSON validity and delivers 94.1% cost savings and 62% latency reduction.",
                "projected_monthly_savings": 423.50,
                "judge_score": 92.0,
                "judge_wins": 78,
                "judge_ties": 18,
                "judge_losses": 4,
                "judge_coverage": 100.0,
                "judge_disagreement_rate": 4.0,
                "judge_samples": [
                    {
                        "prompt_excerpt": "Extract vendor name and invoice total amount...",
                        "baseline_excerpt": "Vendor: Stark Industries, Amount: $4,500.00",
                        "candidate_excerpt": "Vendor: Stark Industries, Total: $4,500",
                        "judge_score": 100,
                        "orders": [
                            {"baseline_position": "A", "winner": "tie", "candidate_outcome": "tie", "reason": "Both correctly extract the structured fields."}
                        ]
                    }
                ],
            },
            {
                "candidate_model": "meta-llama-3.1-8b",
                "total_requests": 100,
                "successful_requests": 88,
                "failed_requests": 12,
                "error_rate": 0.12,
                "avg_quality_score": 71.2,
                "json_validity_rate": 75.0,
                "json_samples": 40,
                "p50_latency_ms": 210,
                "p95_latency_ms": 440,
                "original_avg_latency_ms": 820,
                "candidate_avg_latency_ms": 230,
                "latency_delta_percent": -72.0,
                "original_cost_dollars": 0.4500,
                "candidate_cost_dollars": 0.0120,
                "cost_savings_dollars": 0.4380,
                "cost_savings_percent": 97.3,
                "recommendation": "DO_NOT_SWITCH",
                "reasoning": "High error rate (12%) and lower JSON validity (75%) fails baseline quality criteria.",
                "projected_monthly_savings": 0.0,
                "judge_score": 45.0,
                "judge_wins": 30,
                "judge_ties": 15,
                "judge_losses": 55,
                "judge_coverage": 100.0,
                "judge_disagreement_rate": 55.0,
                "judge_samples": [
                    {
                        "prompt_excerpt": "Extract vendor name and invoice total amount...",
                        "baseline_excerpt": "Vendor: Stark Industries, Amount: $4,500.00",
                        "candidate_excerpt": "Here is your invoice for Stark.",
                        "judge_score": 0,
                        "orders": [
                            {"baseline_position": "A", "winner": "A", "candidate_outcome": "loss", "reason": "Candidate failed to output valid JSON schema."}
                        ]
                    }
                ],
            },
        ],
    }
]

@replaylab_app.get("/replaylab/health")
def get_health():
    return {
        "stored_captures": len(CAPTURES),
        "dropped_captures": 0,
        "active_experiments": sum(1 for e in EXPERIMENTS if e["status"] == "running"),
        "capture_enabled": CONFIG["enabled"],
        "version": "1.0.0",
        "target_gateway_version": "v1.4.2",
    }

@replaylab_app.get("/replaylab/config")
def get_config():
    return CONFIG

@replaylab_app.put("/replaylab/config")
async def update_config(req: Request):
    updates = await req.json()
    for k, v in updates.items():
        if k in CONFIG:
            CONFIG[k] = v
    return CONFIG

@replaylab_app.get("/replaylab/models")
def get_models():
    return MODELS

@replaylab_app.get("/replaylab/captures")
def get_captures(limit: int = 50, offset: int = 0):
    return CAPTURES[offset:offset + limit]

@replaylab_app.get("/replaylab/captures/{id}")
def get_capture(id: str):
    for c in CAPTURES:
        if c["id"] == id:
            return c
    raise HTTPException(status_code=404, detail="Capture not found")

@replaylab_app.post("/replaylab/experiments")
async def create_experiment(req: Request):
    body = await req.json()
    if body.get("dry_run"):
        sample_size = body.get("sample_size", 50)
        num_candidates = len(body.get("candidate_models", []))
        total_replays = sample_size * num_candidates
        estimated_cost = round(total_replays * 0.00035, 4)
        return {
            "spent_dollars": estimated_cost,
            "total_replays": total_replays,
            "stop_reason": None,
            "replay_cost_dollars": round(estimated_cost * 0.7, 4),
            "judge_cost_dollars": round(estimated_cost * 0.3, 4) if body.get("judge") else 0,
        }

    exp_id = f"exp-{uuid.uuid4().hex[:8]}"
    candidates = body.get("candidate_models", ["gpt-4o-mini"])
    sample_size = body.get("sample_size", 50)
    total_replays = sample_size * len(candidates)

    new_exp = {
        "id": exp_id,
        "name": body.get("name", "New Experiment"),
        "status": "running",
        "stop_reason": None,
        "original_model": body.get("original_model", "gpt-4o"),
        "candidate_models": candidates,
        "total_replays": total_replays,
        "completed_replays": 0,
        "spent_dollars": 0.0,
        "budget_cap_dollars": body.get("budget_cap_dollars", 10.0),
        "created_at": datetime.now(timezone.utc).isoformat(),
        "results": [],
    }
    EXPERIMENTS.insert(0, new_exp)

    # Launch background simulation
    asyncio.create_task(simulate_experiment(exp_id, total_replays, candidates))
    return Response(content=json.dumps({"id": exp_id}), status_code=202, media_type="application/json")

async def simulate_experiment(exp_id: str, total_replays: int, candidates: list):
    exp = next((e for e in EXPERIMENTS if e["id"] == exp_id), None)
    if not exp:
        return

    step = max(1, total_replays // 10)
    for i in range(step, total_replays + 1, step):
        await asyncio.sleep(0.2)
        if exp["status"] != "running":
            break
        exp["completed_replays"] = min(i, total_replays)
        exp["spent_dollars"] = round(exp["completed_replays"] * 0.0003, 4)

    if exp["status"] == "running":
        exp["status"] = "completed"
        # populate mock results
        exp["results"] = [
            {
                "candidate_model": cand,
                "total_requests": exp["total_replays"] // len(candidates),
                "successful_requests": exp["total_replays"] // len(candidates),
                "failed_requests": 0,
                "error_rate": 0.0,
                "avg_quality_score": 95.0,
                "json_validity_rate": 100.0,
                "json_samples": 20,
                "p50_latency_ms": 250,
                "p95_latency_ms": 480,
                "original_avg_latency_ms": 800,
                "candidate_avg_latency_ms": 280,
                "latency_delta_percent": -65.0,
                "original_cost_dollars": 0.20,
                "candidate_cost_dollars": 0.012,
                "cost_savings_dollars": 0.188,
                "cost_savings_percent": 94.0,
                "recommendation": "SAFE_TO_SWITCH" if "mini" in cand else "SWITCH_WITH_CAUTION",
                "reasoning": "High fidelity replacement with substantial cost reduction.",
                "projected_monthly_savings": 320.0,
                "judge_score": 90.0,
                "judge_wins": 40,
                "judge_ties": 10,
                "judge_losses": 0,
                "judge_coverage": 100.0,
                "judge_disagreement_rate": 0.0,
                "judge_samples": [],
            }
            for cand in candidates
        ]

@replaylab_app.get("/replaylab/experiments")
def get_experiments(limit: int = 50):
    return EXPERIMENTS[:limit]

@replaylab_app.get("/replaylab/experiments/{id}")
def get_experiment(id: str):
    for e in EXPERIMENTS:
        if e["id"] == id:
            return e
    raise HTTPException(status_code=404, detail="Experiment not found")

@replaylab_app.get("/replaylab/experiments/{id}/events")
async def get_experiment_events(id: str):
    exp = next((e for e in EXPERIMENTS if e["id"] == id), None)
    if not exp:
        raise HTTPException(status_code=404, detail="Experiment not found")

    async def event_generator():
        while exp["status"] == "running":
            pct = int((exp["completed_replays"] / exp["total_replays"]) * 100) if exp["total_replays"] > 0 else 0
            data = {
                "status": "running",
                "candidate_model": exp["candidate_models"][0] if exp["candidate_models"] else "gpt-4o-mini",
                "completed_count": exp["completed_replays"],
                "total_count": exp["total_replays"],
                "progress_percent": pct,
                "current_quality": 95.0,
                "current_judge_score": 92.0,
                "spent_dollars": exp["spent_dollars"],
                "message": f"Replayed {exp['completed_replays']}/{exp['total_replays']}",
                "final": False,
            }
            yield f"event: progress\ndata: {json.dumps(data)}\n\n"
            await asyncio.sleep(0.2)

        # Final event
        final_data = {
            "status": exp["status"],
            "candidate_model": exp["candidate_models"][0] if exp["candidate_models"] else None,
            "completed_count": exp["completed_replays"],
            "total_count": exp["total_replays"],
            "progress_percent": 100,
            "current_quality": 95.0,
            "current_judge_score": 92.0,
            "spent_dollars": exp["spent_dollars"],
            "message": "Experiment completed",
            "final": True,
        }
        yield f"event: progress\ndata: {json.dumps(final_data)}\n\n"

    return StreamingResponse(event_generator(), media_type="text/event-stream")

@replaylab_app.post("/replaylab/experiments/{id}/cancel")
def cancel_experiment_endpoint(id: str):
    exp = next((e for e in EXPERIMENTS if e["id"] == id), None)
    if not exp:
        raise HTTPException(status_code=404, detail="Experiment not found")
    if exp["status"] != "running":
        raise HTTPException(status_code=409, detail="Experiment has already finished")
    exp["status"] = "cancelled"
    return {"status": "cancelled"}

@replaylab_app.get("/replaylab/experiments/{id}/report")
def get_report(id: str):
    return (
        f"# Bifrost Replay Lab Experiment Report: {id}\n\n"
        "## Summary\n"
        "- **Status**: Completed\n"
        "- **Baseline**: gpt-4o\n"
        "- **Recommended**: gpt-4o-mini (Safe to switch, 94.1% cost savings)\n"
    )

# ==========================================
# INSIGHTS API APP (Port 8090)
# ==========================================
insights_app = FastAPI(title="Insights API")
insights_app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

@insights_app.get("/api/health")
def insights_health():
    return {"status": "healthy"}

@insights_app.get("/api/insights/overview")
def insights_overview():
    return {
        "replaylab": {
            "captures_24h": 1280,
            "cost_24h_usd": 42.50,
            "error_rate_24h": 0.4,
            "safe_switch_monthly_savings_usd": 480.00,
        },
        "stale": {
            "reports_7d": 45,
            "avg_freshness_7d": 78.4,
            "stale_claims_7d": 18,
            "llm_mode_percent": 84.0,
            "heuristic_mode_percent": 16.0,
        },
        "latest_experiments": [
            {
                "id": "exp-29ae0198-6",
                "name": "gpt-4o vs gpt-4o-mini & llama-3.1",
                "status": "completed",
                "best_verdict": "SAFE_TO_SWITCH",
                "best_verdict_model": "gpt-4o-mini",
                "created_at": datetime.now(timezone.utc).isoformat(),
            }
        ],
        "latest_reports": [
            {
                "id": "rpt-demo-1",
                "text_preview": "Python 3.9 is the latest stable release of Python. OpenAI GPT-4...",
                "freshness_score": 22.4,
                "stale_count": 2,
                "checked_at": datetime.now(timezone.utc).isoformat(),
            }
        ],
    }

@insights_app.get("/api/insights/replaylab/daily")
def insights_replaylab_daily(days: int = 30):
    points = []
    models = ["gpt-4o", "gpt-4o-mini", "claude-3-5-sonnet"]
    for i in range(days, 0, -1):
        d_str = f"2026-10-{31 - i:02d}" if i <= 30 else "2026-11-01"
        for m in models:
            points.append({
                "date": d_str,
                "model": m,
                "captures": 30 + (i * 3) % 25,
                "cost_usd": round(1.2 + (i * 0.1) % 2.5, 2),
            })
    return points

@insights_app.get("/api/insights/replaylab/verdicts")
def insights_replaylab_verdicts(days: int = 90):
    return [
        {"date": "2026-09", "safe": 14, "caution": 5, "do_not_switch": 3},
        {"date": "2026-10", "safe": 22, "caution": 8, "do_not_switch": 4},
        {"date": "2026-11", "safe": 31, "caution": 6, "do_not_switch": 2},
    ]

@insights_app.get("/api/insights/stale/daily")
def insights_stale_daily(days: int = 30):
    points = []
    types = ["pricing", "software_release", "general"]
    for i in range(days, 0, -1):
        d_str = f"2026-10-{31 - i:02d}" if i <= 30 else "2026-11-01"
        for t in types:
            points.append({
                "date": d_str,
                "claim_type": t,
                "avg_freshness": round(65 + (i * 1.5) % 30, 1),
            })
    return points

@insights_app.get("/api/search")
def search(q: str = "", limit: int = 20):
    if not q:
        return []
    return [
        {
            "kind": "Capture",
            "id": "cap-91a8bf21",
            "title": "Customer support email update",
            "created_at": datetime.now(timezone.utc).isoformat(),
            "url": "/replaylab/captures/cap-91a8bf21",
        },
        {
            "kind": "Experiment",
            "id": "exp-29ae0198-6",
            "title": "gpt-4o vs gpt-4o-mini & llama-3.1",
            "created_at": datetime.now(timezone.utc).isoformat(),
            "url": "/replaylab/experiments/exp-29ae0198-6",
        },
    ]

# Runner
if __name__ == "__main__":
    import sys
    import uvicorn
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8086
    if port == 8090:
        uvicorn.run(insights_app, host="0.0.0.0", port=8090)
    else:
        uvicorn.run(replaylab_app, host="0.0.0.0", port=8086)

