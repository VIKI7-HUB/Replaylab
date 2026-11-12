"""OpenAI-compatible LLM client. Every failure degrades to ``None`` so callers
fall back to the heuristic pipeline instead of failing the request."""

import json
import logging
from typing import Any, Dict, Optional

import httpx

from app import usage
from app.config import settings

logger = logging.getLogger(__name__)


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
        """Send one chat completion and return the message content, or None."""
        try:
            resp = await self._http().post("/chat/completions", json=payload)
        except httpx.RequestError as err:
            logger.warning("LLM request error: %s", err)
            return None
        usage.record_llm_call()
        if resp.status_code != 200:
            logger.warning("LLM call failed with HTTP %d: %s", resp.status_code, resp.text[:300])
            return None
        try:
            return resp.json()["choices"][0]["message"]["content"]
        except (ValueError, KeyError, IndexError, TypeError) as err:
            logger.warning("Unexpected LLM response shape: %s", err)
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
            try:
                parsed = json.loads(content)
            except json.JSONDecodeError:
                logger.warning("LLM returned invalid JSON; retrying once")
                continue
            return parsed if isinstance(parsed, dict) else None
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
