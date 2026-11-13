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


def test_span_ignores_miscounted_llm_offsets():
    """The live bug: the LLM gave 28-69 for a claim that starts at 0."""
    from app.extractor import locate_span

    answer = "The latest stable release of Python is 3.9, and Node.js 14 is the current LTS version."
    claim = "The latest stable release of Python is 3.9"
    span = locate_span(answer, claim, 28, 69)
    assert (span.start, span.end) == (0, len(claim))
    assert answer[span.start:span.end] == claim


def test_span_keeps_correct_llm_offsets():
    from app.extractor import locate_span

    answer = "Node.js 14 is the current LTS version."
    span = locate_span(answer, "node.js 14 is the current LTS version", 0, 37)
    assert (span.start, span.end) == (0, 37)


def test_span_none_when_claim_not_in_answer():
    from app.extractor import locate_span

    assert locate_span("Nothing relevant here.", "Python 3.9 is the latest", None, None) is None
