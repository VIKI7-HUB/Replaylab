"""End-to-end integration tests using mocked Tavily search responses."""

import pytest
from unittest.mock import patch, AsyncMock
from fastapi.testclient import TestClient
from app.main import app

client = TestClient(app)


def test_health_endpoint():
    resp = client.get("/v1/health")
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "healthy"
    assert data["service"] == "stale-answer-detector"


@pytest.mark.asyncio
async def test_full_check_pipeline_mocked():
    sample_text = "OpenAI GPT-4 costs $0.03 per 1K input tokens. The CEO of Twitter is Elon Musk."
    
    mock_tavily_results = [
        {
            "url": "https://openai.com/api/pricing/",
            "title": "OpenAI Official Pricing",
            "content": "GPT-4o starts at $2.50 per 1M input tokens.",
            "score": 0.95,
            "published_date": "2026-01-10",
        },
        {
            "url": "https://reuters.com/technology/yaccarino-twitter-ceo",
            "title": "Linda Yaccarino Twitter CEO",
            "content": "Linda Yaccarino is the Chief Executive Officer of X, formerly Twitter.",
            "score": 0.90,
            "published_date": "2025-06-01",
        }
    ]

    with patch("app.main.tavily_client.execute_claim_search", new_callable=AsyncMock) as mock_search:
        mock_search.return_value = mock_tavily_results
        
        response = client.post(
            "/v1/check",
            json={
                "answer": sample_text,
                "question": "What are the specs and leadership?",
            },
        )
        assert response.status_code == 200
        report = response.json()
        assert "report_id" in report
        assert "overall_freshness_score" in report
        assert report["summary"]["total_claims"] >= 2
        assert len(report["claims"]) >= 2
        assert report["corrected_answer"] != ""


def test_history_endpoints():
    resp = client.get("/v1/history?limit=5")
    assert resp.status_code == 200
    assert isinstance(resp.json(), list)
