"""The LLM client: retries, response_format fallback and tolerant JSON parsing."""

import asyncio
import json

import httpx

from app import llm
from app.llm import LLMClient, parse_json_object

OK_BODY = {"choices": [{"message": {"content": '{"verdict": "STALE"}'}}]}


def _client(handler) -> LLMClient:
    client = LLMClient()
    client.api_key = "test-key"
    client._client = httpx.AsyncClient(transport=httpx.MockTransport(handler), base_url="https://llm.test")
    return client


def test_parse_json_object_reads_fenced_and_wrapped_output():
    assert parse_json_object('{"a": 1}') == {"a": 1}
    assert parse_json_object('```json\n{"a": 1}\n```') == {"a": 1}
    assert parse_json_object('Here it is: {"a": 1} Hope that helps.') == {"a": 1}
    assert parse_json_object("no json here") is None
    assert parse_json_object("[1, 2]") is None


def test_rate_limited_call_is_retried(monkeypatch):
    monkeypatch.setattr(llm, "MAX_RETRY_WAIT_SECONDS", 0.01)
    calls = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        if len(calls) == 1:
            return httpx.Response(429, json={"error": {"message": "Rate limit reached. Please try again in 5ms."}})
        return httpx.Response(200, json=OK_BODY)

    result = asyncio.run(_client(handler).call_json("system", "user"))
    assert result == {"verdict": "STALE"}
    assert len(calls) == 2


def test_provider_without_response_format_is_asked_again_without_it():
    bodies = []

    def handler(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        bodies.append(body)
        if "response_format" in body:
            return httpx.Response(400, json={"error": {"message": "response_format is not supported"}})
        return httpx.Response(200, json=OK_BODY)

    result = asyncio.run(_client(handler).call_json("system", "user"))
    assert result == {"verdict": "STALE"}
    assert "response_format" in bodies[0] and "response_format" not in bodies[1]


def test_auth_failure_is_not_retried():
    calls = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        return httpx.Response(401, json={"error": {"message": "bad key"}})

    assert asyncio.run(_client(handler).call_json("system", "user")) is None
    assert len(calls) == 1
