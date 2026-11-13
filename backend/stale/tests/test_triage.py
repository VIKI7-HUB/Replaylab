"""Unit tests for staleness risk triage and credit conservation logic."""

from app.schemas import Claim, ClaimType
from app.triage import compute_staleness_risk


def test_pricing_claim_has_high_staleness_risk():
    claim = Claim(
        id="c-1",
        text="OpenAI GPT-4 costs $0.03 per 1K input tokens",
        entity="OpenAI",
        claim_type=ClaimType.PRICING,
    )
    risk, reasons, skipped, skip_reason = compute_staleness_risk(claim)
    assert risk >= 0.80
    assert not skipped
    assert any("pricing" in r.lower() for r in reasons)


def test_timeless_math_is_skipped():
    claim = Claim(
        id="c-2",
        text="Two plus two equals four according to standard arithmetic formula",
        entity="Math",
        claim_type=ClaimType.TIMELESS,
    )
    risk, reasons, skipped, skip_reason = compute_staleness_risk(claim)
    assert risk <= 0.15
    assert skipped is True
    assert "timeless" in skip_reason.lower()


def test_historical_fact_is_skipped():
    claim = Claim(
        id="c-3",
        text="In 1969 Apollo 11 landed on the Moon",
        entity="NASA",
        claim_type=ClaimType.EVENT,
    )
    risk, reasons, skipped, skip_reason = compute_staleness_risk(claim)
    assert risk <= 0.15
    assert skipped is True
    assert "historical" in skip_reason.lower()


def test_temporal_marker_boosts_risk():
    base_claim = Claim(
        id="c-4",
        text="Twitter has a Chief Executive Officer",
        entity="Twitter",
        claim_type=ClaimType.LEADERSHIP_PEOPLE,
    )
    boosted_claim = Claim(
        id="c-5",
        text="Twitter currently has a new Chief Executive Officer",
        entity="Twitter",
        claim_type=ClaimType.LEADERSHIP_PEOPLE,
    )
    risk_base, _, _, _ = compute_staleness_risk(base_claim)
    risk_boosted, _, _, _ = compute_staleness_risk(boosted_claim)
    assert risk_boosted > risk_base
