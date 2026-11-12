"""FastAPI entry point for the Stale-Answer Detector Backend."""

import asyncio
import json
import logging
import time
import uuid
from contextlib import asynccontextmanager
from datetime import datetime, timezone
from typing import Optional, List, Dict, Any, AsyncGenerator

from fastapi import FastAPI, HTTPException, Header, Depends, Query, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import StreamingResponse

from app.config import settings
from app import usage
from app.schemas import (
    CheckOptions,
    CheckRequest,
    CheckReport,
    WrapRequest,
    AskRequest,
    UsageStats,
    Claim,
)
from app.db import init_db, save_report, get_report_by_id, get_reports
from app.extractor import extract_claims
from app.triage import triage_claims
from app.queries import generate_search_spec
from app.tavily_client import tavily_client
from app.verifier import verify_claim
from app.scoring import score_claim, compute_aggregate_summary
from app.corrector import generate_corrected_answer
from app.llm import llm_client

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(name)s: %(message)s")
logger = logging.getLogger("stale_detector")


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifecycle hook for database initialization."""
    init_db()
    logger.info(
        "Stale-Answer Detector ready (tavily=%s, verification=%s)",
        "configured" if tavily_client.is_configured else "MISSING KEY",
        "llm" if llm_client.is_available else "heuristic",
    )
    yield
    await tavily_client.aclose()
    await llm_client.aclose()


app = FastAPI(
    title="Stale-Answer Detector API",
    description=(
        "Live Tavily-powered knowledge freshness verification engine. "
        "Flags outdated LLM assertions (pricing, APIs, leadership, regulations), "
        "re-verifies against live web sources, produces deterministic freshness scores, "
        "and synthesizes corrected answers with inline citations."
    ),
    version="1.1.0",
    lifespan=lifespan,
)

# Enable permissive CORS for frontend integration
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=False,  # "*" origins with credentials is invalid CORS
    allow_methods=["*"],
    allow_headers=["*"],
)


def verify_api_key(x_api_key: Optional[str] = Header(None)) -> None:
    """Optional API key check if API_KEY is set in environment."""
    if settings.api_key and settings.api_key.strip():
        if x_api_key != settings.api_key:
            raise HTTPException(status_code=401, detail="Invalid or missing X-API-Key header")


async def _emit(queue: Optional[asyncio.Queue], event: str, data: Dict[str, Any]) -> None:
    if queue is not None:
        await queue.put({"event": event, "data": data})


async def run_pipeline(
    answer: str,
    question: Optional[str] = None,
    answer_date: Optional[str] = None,
    options: Optional[CheckOptions] = None,
    event_queue: Optional[asyncio.Queue] = None,
) -> CheckReport:
    """Extract -> triage -> search -> verify -> score -> correct -> persist."""
    options = options or CheckOptions()
    request_usage = usage.start_request()
    start_time = time.perf_counter()
    report_id = f"rpt-{uuid.uuid4().hex[:10]}"
    checked_at = datetime.now(timezone.utc).isoformat()

    await _emit(event_queue, "pipeline_started", {"report_id": report_id, "answer_len": len(answer), "question": question})

    # 1. Claim extraction
    claims = await extract_claims(answer, max_claims=options.max_claims or 10)
    await _emit(event_queue, "claims_extracted", {"count": len(claims), "claims": [c.model_dump() for c in claims]})

    # 2. Staleness-risk triage (skips low-risk claims to save credits)
    claims = triage_claims(claims, answer_date=answer_date)
    skipped = sum(1 for c in claims if c.skipped)
    await _emit(event_queue, "triage_completed", {"total": len(claims), "active": len(claims) - skipped, "skipped": skipped})

    # 3-5. Search, verify and score each claim concurrently (Tavily concurrency is bounded in the client)
    async def check_claim(claim: Claim) -> Claim:
        if claim.skipped:
            return score_claim(await verify_claim(claim, []))
        await _emit(event_queue, "claim_verifying", {"claim_id": claim.id, "text": claim.text, "entity": claim.entity})
        spec = generate_search_spec(
            claim,
            depth_override=options.depth,
            domains_allow=options.domains_allow,
            domains_block=options.domains_block,
        )
        results = await tavily_client.execute_claim_search(spec=spec, claim_type=claim.claim_type.value, force_fresh=options.force_fresh)
        scored = score_claim(await verify_claim(claim, results))
        await _emit(event_queue, "claim_verified", {
            "claim_id": scored.id,
            "verdict": scored.verdict.value if scored.verdict else None,
            "freshness_score": scored.freshness_score,
            "current_value": scored.current_value,
        })
        return scored

    claims = list(await asyncio.gather(*(check_claim(c) for c in claims)))

    # 6. Aggregate score and corrected answer
    overall_score, summary = compute_aggregate_summary(claims)
    corrected_answer, changes = await generate_corrected_answer(answer, claims)

    report = CheckReport(
        report_id=report_id,
        checked_at=checked_at,
        overall_freshness_score=overall_score,
        summary=summary,
        claims=claims,
        corrected_answer=corrected_answer,
        changes=changes,
        usage=UsageStats(
            tavily_calls=request_usage.tavily_calls,
            tavily_calls_saved=request_usage.tavily_calls_saved,
            llm_calls=request_usage.llm_calls,
            latency_ms=round((time.perf_counter() - start_time) * 1000, 2),
        ),
        verification_mode="llm" if llm_client.is_available else "heuristic",
        question=question,
        original_answer=answer,
    )

    # 7. Persist (a storage failure must not lose the user's result)
    try:
        save_report(report.model_dump(mode="json"))
    except Exception:
        logger.exception("Failed to persist report %s", report_id)

    await _emit(event_queue, "complete", report.model_dump(mode="json"))
    return report


@app.post(
    "/v1/check",
    response_model=CheckReport,
    summary="Audit an LLM answer for stale facts",
    tags=["Auditing"],
)
async def check_answer(
    req: CheckRequest,
    _auth: None = Depends(verify_api_key),
) -> CheckReport:
    """Takes any LLM answer text, flags claims likely to have gone stale, and re-verifies against live web sources."""
    return await run_pipeline(
        answer=req.answer,
        question=req.question,
        answer_date=req.answer_date,
        options=req.options,
    )


@app.post(
    "/v1/check/stream",
    summary="Stream audit progress via Server-Sent Events",
    tags=["Auditing"],
)
async def check_answer_stream(
    req: CheckRequest,
    request: Request,
    _auth: None = Depends(verify_api_key),
) -> StreamingResponse:
    """Streams SSE events as each claim is extracted, triaged, queried, and verified."""
    queue: asyncio.Queue = asyncio.Queue()

    async def sse_generator() -> AsyncGenerator[str, None]:
        task = asyncio.create_task(
            run_pipeline(
                answer=req.answer,
                question=req.question,
                answer_date=req.answer_date,
                options=req.options,
                event_queue=queue,
            )
        )
        try:
            while True:
                if task.done() and queue.empty():
                    break
                if await request.is_disconnected():
                    return
                try:
                    item = await asyncio.wait_for(queue.get(), timeout=0.5)
                except asyncio.TimeoutError:
                    yield ": keep-alive\n\n"
                    continue
                yield f"event: {item['event']}\ndata: {json.dumps(item['data'])}\n\n"
            if task.exception() is not None:
                logger.error("Streaming pipeline failed", exc_info=task.exception())
                yield f"event: error\ndata: {json.dumps({'detail': 'Verification pipeline failed'})}\n\n"
        finally:
            if not task.done():
                task.cancel()

    return StreamingResponse(
        sse_generator(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


@app.post(
    "/v1/wrap",
    response_model=CheckReport,
    summary="Convenience wrapper for question and model answer",
    tags=["Auditing"],
)
async def wrap_answer(
    req: WrapRequest,
    _auth: None = Depends(verify_api_key),
) -> CheckReport:
    """Convenience alias taking question and model_answer."""
    return await run_pipeline(
        answer=req.model_answer,
        question=req.question,
        options=req.options,
    )


@app.post(
    "/v1/ask",
    response_model=CheckReport,
    summary="Ask a question and auto-verify the generated response",
    tags=["Auditing"],
)
async def ask_and_verify(
    req: AskRequest,
    _auth: None = Depends(verify_api_key),
) -> CheckReport:
    """Generates an answer using the configured LLM (or mock) and immediately audits it for freshness."""
    prompt = f"Answer this question concisely and factually:\n{req.question}"
    generated_text = None

    if llm_client.is_available:
        generated_text = await llm_client.generate_text(
            "You are a helpful knowledge assistant.", prompt
        )

    if not generated_text:
        raise HTTPException(
            status_code=503,
            detail="No LLM is configured (set LLM_API_KEY) or the LLM call failed. Use /v1/check to audit an existing answer.",
        )

    return await run_pipeline(
        answer=generated_text,
        question=req.question,
        options=req.options,
    )


@app.get(
    "/v1/history",
    response_model=List[Dict[str, Any]],
    summary="List audit history",
    tags=["History"],
)
async def list_history(
    limit: int = Query(20, ge=1, le=100),
    offset: int = Query(0, ge=0),
    _auth: None = Depends(verify_api_key),
) -> List[Dict[str, Any]]:
    """Retrieve paginated historical reports from SQLite."""
    return get_reports(limit=limit, offset=offset)


@app.get(
    "/v1/history/{report_id}",
    response_model=CheckReport,
    summary="Get detailed audit report by ID",
    tags=["History"],
)
async def get_history_report(
    report_id: str,
    _auth: None = Depends(verify_api_key),
) -> CheckReport:
    """Retrieve full details of a previously generated check report."""
    report = get_report_by_id(report_id)
    if not report:
        raise HTTPException(status_code=404, detail="Report not found")
    return CheckReport(**report)


@app.get(
    "/v1/health",
    summary="Service health and readiness check",
    tags=["System"],
)
async def health_check() -> Dict[str, Any]:
    """Service status and lifetime usage counters."""
    return {
        "status": "healthy",
        "service": "stale-answer-detector",
        "version": app.version,
        "tavily_configured": tavily_client.is_configured,
        "llm_configured": llm_client.is_available,
        "verification_mode": "llm" if llm_client.is_available else "heuristic",
        "database": "sqlite",
        "tavily_calls_total": usage.lifetime.tavily_calls,
        "tavily_calls_saved": usage.lifetime.tavily_calls_saved,
        "llm_calls_total": usage.lifetime.llm_calls,
    }
