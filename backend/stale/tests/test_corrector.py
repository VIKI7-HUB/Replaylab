"""Corrected-answer synthesis: citation selection and change records."""

import asyncio

from app.corrector import generate_corrected_answer
from app.schemas import Claim, ClaimType, EvidenceItem, Verdict


def _claim(evidence):
    return Claim(
        id="c-1",
        text="The CEO of Example Co is Jane Doe",
        entity="Example Co",
        claim_type=ClaimType.LEADERSHIP_PEOPLE,
        verdict=Verdict.STALE,
        current_value="John Smith is the current CEO",
        explanation="Leadership changed.",
        evidence=evidence,
    )


def _source(url, domain, official=False, relevance=0.5):
    return EvidenceItem(
        url=url,
        title=f"Title for {domain}",
        quote="a short quote",
        domain=domain,
        is_official=official,
        relevance_score=relevance,
    )


def test_citation_prefers_the_official_source_over_the_first_result():
    claim = _claim([
        _source("https://consultancy.example/who-runs-it", "consultancy.example", relevance=0.95),
        _source("https://example.com/about/leadership", "example.com", official=True, relevance=0.6),
    ])
    corrected, changes = asyncio.run(
        generate_corrected_answer("The CEO of Example Co is Jane Doe.", [claim])
    )
    assert changes and changes[0].citation_index == 1
    assert "example.com/about/leadership" in corrected
    assert "consultancy.example" not in corrected


def test_citation_falls_back_to_relevance_when_no_source_is_official():
    claim = _claim([
        _source("https://blog.example/a", "blog.example", relevance=0.4),
        _source("https://news.example/b", "news.example", relevance=0.9),
    ])
    corrected, _ = asyncio.run(
        generate_corrected_answer("The CEO of Example Co is Jane Doe.", [claim])
    )
    assert "news.example/b" in corrected


def test_no_evidence_still_produces_a_change_record():
    claim = _claim([])
    corrected, changes = asyncio.run(
        generate_corrected_answer("The CEO of Example Co is Jane Doe.", [claim])
    )
    assert len(changes) == 1
    assert changes[0].updated_claim == "John Smith is the current CEO"
    assert "tavily.com" in corrected
