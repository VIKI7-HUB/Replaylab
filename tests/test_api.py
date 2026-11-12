"""API behaviour: streaming, usage accounting, options wiring and validation."""

import json
from unittest.mock import AsyncMock, patch

from fastapi.testclient import TestClient

from app import usage
from app.main import app
from app.queries import generate_search_spec
from app.schemas import Claim, ClaimType

client = TestClient(app)

RESULTS = [
    {"url": "https://www.python.org/downloads/", "title": "Python releases", "content": "Python 3.14 is the latest release.", "score": 0.9},
    {"url": "https://en.wikipedia.org/wiki/Python", "title": "Python", "content": "The newest version is Python 3.14.", "score": 0.8},
]


def _events(body: str):
    out = []
    for block in body.split("\n\n"):
        lines = dict(line.split(": ", 1) for line in block.splitlines() if ": " in line and not line.startswith(":"))
        if "event" in lines:
            out.append((lines["event"], json.loads(lines["data"])))
    return out


def test_stream_emits_stages_and_complete_report():
    with patch("app.main.tavily_client.execute_claim_search", new_callable=AsyncMock, return_value=RESULTS):
        resp = client.post("/v1/check/stream", json={"answer": "Python 3.9 is the latest stable release of Python."})
    assert resp.status_code == 200
    names = [e for e, _ in _events(resp.text)]
    assert names[0] == "pipeline_started"
    assert "claim_verified" in names
    assert names[-1] == "complete"
    report = _events(resp.text)[-1][1]
    assert report["claims"][0]["verdict"] == "STALE"
    assert report["verification_mode"] == "heuristic"


def test_stream_reports_pipeline_failure_as_error_event():
    with patch("app.main.extract_claims", new_callable=AsyncMock, side_effect=RuntimeError("boom")):
        resp = client.post("/v1/check/stream", json={"answer": "Python 3.9 is the latest stable release of Python."})
    assert _events(resp.text)[-1][0] == "error"


def test_usage_counts_only_this_request():
    async def fake_search(*args, **kwargs):
        usage.record_tavily_call()
        return RESULTS

    usage.lifetime.tavily_calls += 100  # unrelated earlier traffic must not leak in
    with patch("app.main.tavily_client.execute_claim_search", side_effect=fake_search):
        resp = client.post("/v1/check", json={"answer": "Python 3.9 is the latest stable release of Python."})
    assert resp.status_code == 200
    assert resp.json()["usage"]["tavily_calls"] == 1
    assert resp.json()["usage"]["llm_calls"] == 0


def test_domain_options_reach_search_spec():
    claim = Claim(id="c", text="Stripe charges 2.9% per card payment", entity="Stripe", claim_type=ClaimType.PRICING)
    spec = generate_search_spec(claim, domains_allow=["stripe.com"], domains_block=["reddit.com"])
    assert spec.include_domains == ["stripe.com"]
    assert spec.exclude_domains == ["reddit.com"]


def test_leadership_search_is_not_limited_to_official_domains():
    claim = Claim(id="c", text="The CEO of OpenAI is Sam Altman", entity="OpenAI", claim_type=ClaimType.LEADERSHIP_PEOPLE)
    spec = generate_search_spec(claim)
    assert spec.include_domains == []
    assert spec.official_domains == ["openai.com"]


def test_ask_without_llm_returns_503_instead_of_canned_answer():
    resp = client.post("/v1/ask", json={"question": "What is the latest Python?"})
    assert resp.status_code == 503


def test_invalid_depth_is_rejected():
    resp = client.post("/v1/check", json={"answer": "Python 3.9 is the latest.", "options": {"depth": "extreme"}})
    assert resp.status_code == 422


def test_health_reports_verification_mode():
    data = client.get("/v1/health").json()
    assert data["verification_mode"] == "heuristic"
    assert data["tavily_configured"] is False


def test_dashboard_is_served_at_root():
    resp = client.get("/")
    assert resp.status_code == 200
    assert "Stale-Answer Detector" in resp.text
    assert resp.headers["content-type"].startswith("text/html")


def test_samples_endpoint_returns_demo_answers():
    resp = client.get("/v1/samples")
    assert resp.status_code == 200
    samples = resp.json()
    assert isinstance(samples, list)
    if samples:  # bundled demo file is present in the repo
        assert {"topic", "answer"} <= set(samples[0].keys())
