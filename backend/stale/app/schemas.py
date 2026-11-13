"""Pydantic v2 schemas for Stale-Answer Detector."""

from enum import Enum
from typing import Any, Dict, List, Literal, Optional
from pydantic import BaseModel, Field


class ClaimType(str, Enum):
    """Category of factual claim with distinct volatility profile."""

    PRICING = "pricing"
    VERSION = "version"
    LAW_REGULATION = "law_regulation"
    LEADERSHIP_PEOPLE = "leadership_people"
    STATISTIC = "statistic"
    PRODUCT_AVAILABILITY = "product_availability"
    EVENT = "event"
    OTHER = "other"
    TIMELESS = "timeless"


class Verdict(str, Enum):
    """Verification verdict for a claim against live ground truth."""

    CURRENT = "CURRENT"
    STALE = "STALE"
    PARTIALLY_STALE = "PARTIALLY_STALE"
    UNVERIFIABLE = "UNVERIFIABLE"


class QuotedSpan(BaseModel):
    """Character offsets within the original text."""

    start: int = Field(..., description="Starting character offset (inclusive)")
    end: int = Field(..., description="Ending character offset (exclusive)")


class EvidenceItem(BaseModel):
    """Source evidence item retrieved via Tavily."""

    url: str
    title: str
    published_date: Optional[str] = None
    quote: str = Field(..., description="Short supporting quote under 25 words")
    domain: str = ""
    is_official: bool = False
    relevance_score: float = 0.0


class ScoreBreakdown(BaseModel):
    """Transparent, explainable breakdown of the 0-100 freshness score."""

    verdict_score: float = Field(..., description="Score contribution from verdict match (0-100)")
    evidence_agreement: float = Field(..., description="Consensus ratio across independent domains (0-100)")
    source_authority: float = Field(..., description="Authority boost for official or trusted domains (0-100)")
    recency_factor: float = Field(..., description="Evidence freshness decay factor (0-100)")
    tavily_relevance: float = Field(..., description="Average search relevance score (0-100)")
    formula: str = Field(
        default="verdict_multiplier * (0.35*agreement + 0.30*authority + 0.20*recency + 0.15*relevance)",
        description="Deterministic formula used to compute final freshness score",
    )


class Claim(BaseModel):
    """Atomic verifiable factual claim extracted from LLM text."""

    id: str = Field(..., description="Unique claim identifier, e.g. claim-1")
    text: str = Field(..., description="Original claim statement as extracted")
    entity: str = Field(..., description="Primary subject entity (e.g. OpenAI, Python)")
    claim_type: ClaimType = Field(default=ClaimType.OTHER)
    quoted_span: Optional[QuotedSpan] = None
    staleness_risk: float = Field(default=0.5, ge=0.0, le=1.0, description="Likelihood of change (0.0 to 1.0)")
    risk_reasons: List[str] = Field(default_factory=list)
    skipped: bool = Field(default=False, description="True if verification was skipped to conserve credits")
    skip_reason: Optional[str] = None
    verdict: Optional[Verdict] = Field(default=None, description="Null when the claim was skipped by triage")
    freshness_score: Optional[float] = Field(default=None, ge=0.0, le=100.0)
    score_breakdown: Optional[ScoreBreakdown] = None
    current_value: Optional[str] = Field(default=None, description="Verified factual value as of today")
    explanation: Optional[str] = Field(default=None, description="Clear summary explaining verdict")
    evidence: List[EvidenceItem] = Field(default_factory=list)
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)


class CheckOptions(BaseModel):
    """Optional tuning parameters for check requests."""

    max_claims: Optional[int] = Field(default=10, ge=1, le=30)
    depth: Literal["basic", "advanced"] = Field(default="advanced", description="Tavily search depth")
    domains_allow: Optional[List[str]] = None
    domains_block: Optional[List[str]] = None
    force_fresh: bool = Field(default=False, description="Bypass cache and force live verification")


class CheckRequest(BaseModel):
    """Input payload to verify an LLM answer."""

    answer: str = Field(..., min_length=5, max_length=20000, description="The LLM answer to audit for stale facts")
    question: Optional[str] = Field(default=None, description="Original user prompt or question")
    answer_date: Optional[str] = Field(default=None, description="Estimated generation date (YYYY-MM-DD)")
    options: Optional[CheckOptions] = Field(default_factory=CheckOptions)


class WrapRequest(BaseModel):
    """Convenience payload wrapper."""

    model_config = {"protected_namespaces": ()}

    question: str = Field(..., max_length=2000)
    model_answer: str = Field(..., min_length=5, max_length=20000)
    options: Optional[CheckOptions] = None


class AskRequest(BaseModel):
    """Request to generate answer with LLM then verify it."""

    question: str = Field(..., min_length=3, max_length=2000)
    options: Optional[CheckOptions] = None


class Summary(BaseModel):
    """Aggregate summary statistics across all extracted claims."""

    current: int = 0
    stale: int = 0
    partially_stale: int = 0
    unverifiable: int = 0
    skipped: int = 0
    total_claims: int = 0


class AnswerChange(BaseModel):
    """Recorded correction applied to the original answer."""

    claim_id: str
    original_claim: str
    updated_claim: str
    reason: str
    citation_index: int


class UsageStats(BaseModel):
    """Operational telemetry and credit optimization stats."""

    tavily_calls: int = 0
    tavily_calls_saved: int = 0
    llm_calls: int = 0
    latency_ms: float = 0.0


class CheckReport(BaseModel):
    """Full staleness audit report."""

    report_id: str
    checked_at: str
    overall_freshness_score: float = Field(..., ge=0.0, le=100.0)
    summary: Summary
    claims: List[Claim]
    corrected_answer: str
    changes: List[AnswerChange]
    usage: UsageStats
    verification_mode: str = Field(default="heuristic", description="'llm' or 'heuristic' (no LLM key configured)")
    question: Optional[str] = None
    original_answer: str


class StreamEvent(BaseModel):
    """Server-Sent Event message schema."""

    event: str
    data: Dict[str, Any]
