"""Unit tests for claim extraction and character span indexing."""

from app.extractor import extract_claims_heuristic
from app.schemas import ClaimType


def test_heuristic_pricing_extraction():
    text = "The enterprise tier costs $49.00 per user per month. It also includes 24/7 dedicated support."
    claims = extract_claims_heuristic(text)
    assert len(claims) >= 1
    pricing_claim = claims[0]
    assert pricing_claim.claim_type == ClaimType.PRICING
    assert "$49.00" in pricing_claim.text
    assert pricing_claim.quoted_span is not None
    # Validate span offsets
    extracted_slice = text[pricing_claim.quoted_span.start : pricing_claim.quoted_span.end]
    assert "$49.00" in extracted_slice


def test_heuristic_leadership_extraction():
    text = "Linda Yaccarino serves as the Chief Executive Officer of social network X."
    claims = extract_claims_heuristic(text)
    assert len(claims) >= 1
    assert claims[0].claim_type == ClaimType.LEADERSHIP_PEOPLE
    assert claims[0].entity in ("X", "Twitter", "Linda Yaccarino")


def test_heuristic_version_extraction():
    text = "Python 3.9 was released with multiple dictionary union operator enhancements."
    claims = extract_claims_heuristic(text)
    assert len(claims) >= 1
    assert claims[0].claim_type == ClaimType.VERSION
    assert "Python" in claims[0].entity
