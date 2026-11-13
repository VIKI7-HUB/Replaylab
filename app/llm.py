"""OpenAI-compatible LLM client. Every failure degrades to ``None`` so callers
fall back to the heuristic pipeline instead of failing the request."""

import asyncio
import json
import logging
import re
from typing import Any, Dict, Optional

import httpx

from app import usage
from app.config import settings

logger = logging.getLogger(__name__)

MAX_ATTEMPTS = 4  # one call plus three retries on rate limits and server errors
RETRYABLE_STATUS = {429, 500, 502, 503, 504}
MAX_RETRY_WAIT_SECONDS = 15.0
_TRY_AGAIN_RE = re.compile(r"try again in\s+([0-9.]+)\s*(ms|s)", re.IGNORECASE)
_FENCE_RE = re.compile(r"^```(?:json)?\s*|\s*```$", re.IGNORECASE)


def _retry_wait(resp: httpx.Response, attempt: int) -> float:
    """Seconds to wait before retrying: the provider's hint, else a backoff."""
    header = resp.headers.get("retry-after", "").strip()
    try:
        wait = float(header)
    except ValueError:
        match = _TRY_AGAIN_RE.search(resp.text[:500])
        if match:
            wait = float(match.group(1)) / (1000.0 if match.group(2).lower() == "ms" else 1.0)
        else:
            wait = 1.5 * (2 ** attempt)
    return min(max(wait, 0.0) + 0.25, MAX_RETRY_WAIT_SECONDS)


def parse_json_object(content: str) -> Optional[Dict[str, Any]]:
    """Read a JSON object from model output, with or without a code fence
    or surrounding prose. Returns None when there is no object to read."""
    text = _FENCE_RE.sub("", content.strip()).strip()
    try:
        parsed = json.loads(text)
    except json.JSONDecodeError:
        start, end = text.find("{"), text.rfind("}")
        if start == -1 or end <= start:
            return None
        try:
            parsed = json.loads(text[start:end + 1])
        except json.JSONDecodeError:
            return None
    return parsed if isinstance(parsed, dict) else None


class LLMClient:
    def __init__(self) -> None:
        self.base_url = (settings.llm_base_url or "https://api.openai.com/v1").rstrip("/")
        self.api_key = settings.llm_api_key
        self.model = settings.llm_model
        self.timeout = settings.llm_timeout_seconds
        self._client: Optional[httpx.AsyncClient] = None

    @property
    def is_available(self) -> bool:
        return bool(self.api_key and self.api_key.strip())

    def _http(self) -> httpx.AsyncClient:
        if self._client is None or self._client.is_closed:
            self._client = httpx.AsyncClient(
                base_url=self.base_url,
                timeout=self.timeout,
                headers={"Authorization": f"Bearer {self.api_key}"},
            )
        return self._client

    async def aclose(self) -> None:
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    async def _complete(self, payload: Dict[str, Any]) -> Optional[str]:
        """Send one chat completion and return the message content, or None.

        A rate-limited or failing provider is retried after the wait it asks
        for, so a per-minute limit slows a check down instead of dropping it to
        the heuristic path. A provider that rejects ``response_format`` is
        asked again without it.
        """
        payload = dict(payload)
        for attempt in range(MAX_ATTEMPTS):
            try:
                resp = await self._http().post("/chat/completions", json=payload)
            except httpx.RequestError as err:
                logger.warning("LLM request error (attempt %d): %s", attempt + 1, err)
                if attempt < MAX_ATTEMPTS - 1:
                    await asyncio.sleep(1.5 * (2 ** attempt))
                continue
            usage.record_llm_call()
            if resp.status_code == 200:
                try:
                    return resp.json()["choices"][0]["message"]["content"]
                except (ValueError, KeyError, IndexError, TypeError) as err:
                    logger.warning("Unexpected LLM response shape: %s", err)
                    return None
            if resp.status_code == 400 and "response_format" in payload:
                logger.warning("LLM rejected the request (HTTP 400); retrying without response_format: %s", resp.text[:200])
                payload.pop("response_format")
                continue
            logger.warning("LLM call failed with HTTP %d (attempt %d): %s", resp.status_code, attempt + 1, resp.text[:300])
            if resp.status_code not in RETRYABLE_STATUS:
                return None
            if attempt < MAX_ATTEMPTS - 1:
                await asyncio.sleep(_retry_wait(resp, attempt))
        return None

    async def call_json(self, system_prompt: str, user_prompt: str) -> Optional[Dict[str, Any]]:
        """Request a JSON object; retries once if the model returns invalid JSON."""
        if not self.is_available:
            return None
        payload = {
            "model": self.model,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            "response_format": {"type": "json_object"},
            "temperature": 0.0,
        }
        for _ in range(2):
            content = await self._complete(payload)
            if content is None:
                return None
            parsed = parse_json_object(content)
            if parsed is not None:
                return parsed
            logger.warning("LLM returned invalid JSON; retrying once")
        return None

    async def generate_text(self, system_prompt: str, user_prompt: str) -> Optional[str]:
        """Free-text completion."""
        if not self.is_available:
            return None
        content = await self._complete({
            "model": self.model,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            "temperature": 0.3,
        })
        return content.strip() if content else None


llm_client = LLMClient()
