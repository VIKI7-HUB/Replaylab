"""Deterministic, explainable freshness scoring engine with component breakdowns."""

from datetime import datetime, timezone
import re
from typing import List, Optional, Tuple
from app.schemas import Claim, ScoreBreakdown, Verdict, Summary


def parse_date_recency_score(date_str: Optional[str]) -> float:
    """Recency factor (0-100) from a publication date, relative to today."""
    if not date_str:
        return 60.0  # neutral when the source has no date
    year_match = re.search(r"\b(20\d\d)\b", date_str)
    if not year_match:
        return 60.0
    age = datetime.now(timezone.utc).year - int(year_match.group(1))
    if age <= 0:
        return 100.0
    if age == 1:
        return 80.0
    if age == 2:
        return 60.0
    return 40.0


def score_claim(claim: Claim) -> Claim:
    """Compute deterministic freshness score (0-100) and full breakdown for a single claim."""
    if claim.skipped:
        breakdown = ScoreBreakdown(
            verdict_score=100.0,
            evidence_agreement=100.0,
            source_authority=100.0,
            recency_factor=100.0,
            tavily_relevance=100.0,
            formula="Not checked: low staleness risk, assumed fresh (100.0)",
        )
        claim.freshness_score = 100.0
        claim.score_breakdown = breakdown
        return claim

    # 1. Verdict Score (0-100)
    verdict_map = {
        Verdict.CURRENT: 100.0,
        Verdict.PARTIALLY_STALE: 45.0,
        Verdict.UNVERIFIABLE: 50.0,
        Verdict.STALE: 0.0,
    }
    verdict_score = verdict_map.get(claim.verdict, 50.0)

    # 2. Evidence Agreement across distinct domains (0-100)
    distinct_domains = {ev.domain for ev in claim.evidence if ev.domain}
    num_domains = len(distinct_domains)
    if num_domains >= 3:
        evidence_agreement = 100.0
    elif num_domains == 2:
        evidence_agreement = 85.0
    elif num_domains == 1:
        evidence_agreement = 60.0
    else:
        evidence_agreement = 30.0

    # 3. Source Authority (0-100)
    has_official = any(ev.is_official for ev in claim.evidence)
    if has_official:
        source_authority = 100.0
    elif any(d.endswith((".gov", ".edu", "wikipedia.org", "reuters.com")) for d in distinct_domains):
        source_authority = 85.0
    elif num_domains > 0:
        source_authority = 60.0
    else:
        source_authority = 30.0

    # 4. Evidence Recency Factor (0-100)
    recency_scores = [parse_date_recency_score(ev.published_date) for ev in claim.evidence if ev.published_date]
    recency_factor = max(recency_scores) if recency_scores else 65.0

    # 5. Tavily Search Relevance Score (0-100)
    scores = [ev.relevance_score for ev in claim.evidence if ev.relevance_score > 0]
    avg_relevance = (sum(scores) / len(scores)) * 100.0 if scores else 50.0
    tavily_relevance = min(100.0, max(0.0, avg_relevance))

    # Deterministic explainable formula:
    # Verdict establishes the truth factor, while evidence signals calibrate confidence & authority
    verdict_multipliers = {
        Verdict.CURRENT: 1.00,
        Verdict.PARTIALLY_STALE: 0.45,
        Verdict.UNVERIFIABLE: 0.50,
        Verdict.STALE: 0.05,  # Decisive stale claims have near-zero freshness
    }
    multiplier = verdict_multipliers.get(claim.verdict, 0.50)

    evidence_quality = (
        (0.35 * evidence_agreement)
        + (0.30 * source_authority)
        + (0.20 * recency_factor)
        + (0.15 * tavily_relevance)
    )

    raw_score = multiplier * evidence_quality

    final_score = round(max(0.0, min(100.0, raw_score)), 1)
    breakdown = ScoreBreakdown(
        verdict_score=round(verdict_score, 1),
        evidence_agreement=round(evidence_agreement, 1),
        source_authority=round(source_authority, 1),
        recency_factor=round(recency_factor, 1),
        tavily_relevance=round(tavily_relevance, 1),
        formula="verdict_multiplier * (0.35*agreement + 0.30*authority + 0.20*recency + 0.15*relevance)",
    )

    claim.freshness_score = final_score
    claim.score_breakdown = breakdown
    return claim


def compute_aggregate_summary(claims: List[Claim]) -> Tuple[float, Summary]:
    """Compute overall answer freshness score and verdict counts."""
    summary = Summary(total_claims=len(claims))
    if not claims:
        return 100.0, summary

    total_weight = 0.0
    weighted_score_sum = 0.0

    for c in claims:
        # Update verdict counts
        if c.skipped:
            summary.skipped += 1
        elif c.verdict == Verdict.CURRENT:
            summary.current += 1
        elif c.verdict == Verdict.STALE:
            summary.stale += 1
        elif c.verdict == Verdict.PARTIALLY_STALE:
            summary.partially_stale += 1
        elif c.verdict == Verdict.UNVERIFIABLE:
            summary.unverifiable += 1

        # Weighting: claims with higher staleness risk carry greater weight in the score
        weight = max(0.20, c.staleness_risk)
        c_score = c.freshness_score if c.freshness_score is not None else 50.0
        weighted_score_sum += c_score * weight
        total_weight += weight

    overall_score = round(weighted_score_sum / max(0.01, total_weight), 1)
    return overall_score, summary
