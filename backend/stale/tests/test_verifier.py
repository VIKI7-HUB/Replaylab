"""Verifier heuristics must generalise (no claim-specific answers) and abstain
rather than guess when evidence is unrelated."""

import pytest

from app.schemas import Claim, ClaimType, Verdict
from app.verifier import heuristic_verify, is_official_or_trusted, verify_claim


def _claim(text, ctype, entity="General"):
    return Claim(id="c1", text=text, entity=entity, claim_type=ctype)


def _result(content, url="https://example.com/a", title="Source"):
    return {"url": url, "title": title, "content": content, "score": 0.8}


def test_newer_version_in_evidence_is_stale_for_any_product():
    claim = _claim("Node.js 18 is the latest release of Node", ClaimType.VERSION, "Node.js")
    verdict, value, *_ = heuristic_verify(claim, [_result("Node.js 22 is now the active LTS; Node.js 24 is current.")])
    assert verdict == Verdict.STALE
    assert "24" in value


def test_same_version_in_evidence_is_current():
    claim = _claim("Rust 1.80 is the newest stable Rust", ClaimType.VERSION, "Rust")
    verdict, *_ = heuristic_verify(claim, [_result("The Rust team released Rust 1.80 today.")])
    assert verdict == Verdict.CURRENT


def test_years_are_not_mistaken_for_versions():
    claim = _claim("Python 3.12 shipped in 2023", ClaimType.VERSION, "Python")
    verdict, *_ = heuristic_verify(claim, [_result("In 2025 the Python 3.12 branch entered security-fix mode.")])
    assert verdict == Verdict.CURRENT


def test_unrelated_evidence_is_unverifiable_not_stale():
    claim = _claim("Acme Widgets ships its X200 model with a titanium frame", ClaimType.OTHER, "Acme")
    verdict, value, _, confidence, _ = heuristic_verify(claim, [_result("A recipe for banana bread with walnuts.")])
    assert verdict == Verdict.UNVERIFIABLE
    assert value is None
    assert confidence <= 0.3


def test_missing_price_is_partially_stale_with_listed_prices():
    claim = _claim("The Pro plan costs $15 per month", ClaimType.PRICING, "Acme")
    verdict, value, *_ = heuristic_verify(claim, [_result("Pricing: Pro is $20/month and Team is $35/month.")])
    assert verdict == Verdict.PARTIALLY_STALE
    assert "$20" in value


def test_matching_price_is_current():
    claim = _claim("The Pro plan costs $20 per month", ClaimType.PRICING, "Acme")
    verdict, *_ = heuristic_verify(claim, [_result("Pricing: Pro is $20/month.")])
    assert verdict == Verdict.CURRENT


def test_leader_described_as_former_is_stale():
    claim = _claim("The CEO of Globex is Hank Scorpio", ClaimType.LEADERSHIP_PEOPLE, "Globex")
    results = [_result("Former chief executive Hank Scorpio stepped down in March. Jane Doe is now CEO of Globex.")]
    verdict, value, *_ = heuristic_verify(claim, results)
    assert verdict == Verdict.STALE
    assert "Jane Doe" in value


def test_no_results_is_unverifiable():
    verdict, *_ = heuristic_verify(_claim("Anything at all here", ClaimType.OTHER), [])
    assert verdict == Verdict.UNVERIFIABLE


@pytest.mark.asyncio
async def test_skipped_claim_gets_no_verdict():
    claim = _claim("Two plus two equals four", ClaimType.TIMELESS)
    claim.skipped, claim.skip_reason = True, "not_checked: timeless invariant"
    out = await verify_claim(claim, [])
    assert out.verdict is None
    assert out.confidence == 0.0


def test_official_domain_matching_is_word_based():
    assert is_official_or_trusted("python.org", "Python")
    assert is_official_or_trusted("data.census.gov", "Census")
    # "x" must not match every entity containing the letter x.
    assert not is_official_or_trusted("x.com", "Linux Foundation")
