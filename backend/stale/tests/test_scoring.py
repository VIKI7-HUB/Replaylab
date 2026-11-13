"""Unit tests for deterministic freshness scoring and component breakdown."""

from app.schemas import Claim, ClaimType, Verdict, EvidenceItem
from app.scoring import score_claim, compute_aggregate_summary


def test_stale_claim_scoring():
    claim = Claim(
        id="c-stale",
        text="OpenAI GPT-4 costs $0.03 per 1K input tokens",
        entity="OpenAI",
        claim_type=ClaimType.PRICING,
        verdict=Verdict.STALE,
        evidence=[
            EvidenceItem(
                url="https://openai.com/api/pricing/",
                title="OpenAI Pricing",
                published_date="2026-01-15",
                quote="GPT-4o costs $2.50 per 1M input tokens",
                domain="openai.com",
                is_official=True,
                relevance_score=0.92,
            ),
            EvidenceItem(
                url="https://techcrunch.com/openai-pricing-update",
                title="TechCrunch OpenAI Updates",
                published_date="2026-01-16",
                quote="OpenAI lowered model fees substantially",
                domain="techcrunch.com",
                is_official=False,
                relevance_score=0.85,
            ),
        ],
    )
    scored = score_claim(claim)
    assert scored.freshness_score is not None
    # Since verdict is STALE (0 verdict score), the overall freshness must be low
    assert scored.freshness_score < 45.0
    assert scored.score_breakdown.verdict_score == 0.0
    assert scored.score_breakdown.source_authority == 100.0  # Official openai.com present


def test_current_claim_scoring():
    claim = Claim(
        id="c-current",
        text="Linda Yaccarino was appointed Chief Executive Officer of X",
        entity="X",
        claim_type=ClaimType.LEADERSHIP_PEOPLE,
        verdict=Verdict.CURRENT,
        evidence=[
            EvidenceItem(
                url="https://reuters.com/world/us/yaccarino-ceo",
                title="Reuters Yaccarino",
                published_date="2025-05-12",
                quote="Linda Yaccarino serves as CEO of X",
                domain="reuters.com",
                is_official=False,
                relevance_score=0.88,
            ),
            EvidenceItem(
                url="https://x.com/about",
                title="About X",
                quote="Leadership team",
                domain="x.com",
                is_official=True,
                relevance_score=0.95,
            ),
        ],
    )
    scored = score_claim(claim)
    assert scored.freshness_score > 85.0
    assert scored.score_breakdown.verdict_score == 100.0


def test_skipped_claim_scoring():
    claim = Claim(
        id="c-skipped",
        text="Speed of light is invariant",
        entity="Physics",
        claim_type=ClaimType.TIMELESS,
        skipped=True,
        skip_reason="Timeless invariant",
    )
    scored = score_claim(claim)
    assert scored.freshness_score == 100.0


def test_aggregate_summary_calculation():
    claims = [
        Claim(id="1", text="Stale fact", entity="A", staleness_risk=0.8, freshness_score=20.0, verdict=Verdict.STALE),
        Claim(id="2", text="Fresh fact", entity="B", staleness_risk=0.5, freshness_score=95.0, verdict=Verdict.CURRENT),
    ]
    overall, summary = compute_aggregate_summary(claims)
    assert summary.total_claims == 2
    assert summary.stale == 1
    assert summary.current == 1
    assert 20.0 < overall < 95.0
