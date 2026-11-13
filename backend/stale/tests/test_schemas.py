"""Unit tests verifying Pydantic v2 schemas and validation constraints."""

from app.schemas import CheckRequest, CheckReport, ClaimType, Verdict, Summary, UsageStats


def test_check_request_validation():
    req = CheckRequest(answer="OpenAI GPT-4 costs $0.03 per 1K input tokens")
    assert req.answer.startswith("OpenAI")
    assert req.options.max_claims == 10
    assert req.options.depth == "advanced"


def test_claim_type_enum():
    assert ClaimType.PRICING.value == "pricing"
    assert ClaimType.VERSION.value == "version"
    assert ClaimType.LEADERSHIP_PEOPLE.value == "leadership_people"


def test_verdict_enum():
    assert Verdict.CURRENT.value == "CURRENT"
    assert Verdict.STALE.value == "STALE"
    assert Verdict.PARTIALLY_STALE.value == "PARTIALLY_STALE"
    assert Verdict.UNVERIFIABLE.value == "UNVERIFIABLE"


def test_check_report_serialization():
    report = CheckReport(
        report_id="rpt-12345",
        checked_at="2026-11-04T12:00:00Z",
        overall_freshness_score=88.5,
        summary=Summary(current=2, stale=1, total_claims=3),
        claims=[],
        corrected_answer="Corrected text here",
        changes=[],
        usage=UsageStats(tavily_calls=2, latency_ms=450.0),
        original_answer="Original text here",
    )
    dumped = report.model_dump()
    assert dumped["report_id"] == "rpt-12345"
    assert dumped["overall_freshness_score"] == 88.5
    assert dumped["summary"]["total_claims"] == 3
