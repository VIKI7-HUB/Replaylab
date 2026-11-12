"""Async Tavily client: search + extract with caching, concurrency limits and retries."""

import asyncio
import logging
from typing import Any, Dict, List, Optional
from urllib.parse import urlparse

import httpx

from app import usage
from app.cache import generate_cache_key, get_cached_result, set_cached_result
from app.config import settings
from app.queries import SearchSpec

logger = logging.getLogger(__name__)

MAX_RETRIES = 3
RETRYABLE_STATUS = {429, 500, 502, 503, 504}
EXTRACT_CHARS = 1500


class TavilyClient:
    """Wraps Tavily /search and /extract.

    Every call goes through the SQLite cache first (TTL depends on the claim
    type), then a semaphore that bounds concurrent requests. A credit is
    counted only for responses Tavily actually served.
    """

    def __init__(self) -> None:
        self.api_key = settings.tavily_api_key
        self.base_url = settings.tavily_base_url.rstrip("/")
        self.timeout = settings.tavily_timeout_seconds
        self._semaphore: Optional[asyncio.Semaphore] = None
        self._client: Optional[httpx.AsyncClient] = None

    @property
    def is_configured(self) -> bool:
        return bool(self.api_key and self.api_key.strip())

    @property
    def call_count(self) -> int:
        """Lifetime Tavily calls served (kept for backwards compatibility)."""
        return usage.lifetime.tavily_calls

    def _http(self) -> httpx.AsyncClient:
        if self._client is None or self._client.is_closed:
            self._client = httpx.AsyncClient(
                base_url=self.base_url,
                timeout=self.timeout,
                headers={"Authorization": f"Bearer {self.api_key}"},
            )
        return self._client

    def _sem(self) -> asyncio.Semaphore:
        if self._semaphore is None:
            self._semaphore = asyncio.Semaphore(max(1, settings.tavily_concurrency))
        return self._semaphore

    async def aclose(self) -> None:
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    async def _post(self, path: str, payload: Dict[str, Any], timeout: Optional[float] = None) -> Optional[Dict[str, Any]]:
        """POST with retries on network errors and retryable statuses."""
        async with self._sem():
            for attempt in range(MAX_RETRIES):
                try:
                    resp = await self._http().post(path, json=payload, timeout=timeout or self.timeout)
                except httpx.RequestError as err:
                    logger.warning("Tavily %s network error (attempt %d): %s", path, attempt + 1, err)
                else:
                    if resp.status_code == 200:
                        usage.record_tavily_call()
                        return resp.json()
                    if resp.status_code not in RETRYABLE_STATUS:
                        logger.error("Tavily %s returned HTTP %d: %s", path, resp.status_code, resp.text[:300])
                        return None
                    logger.warning("Tavily %s returned HTTP %d (attempt %d)", path, resp.status_code, attempt + 1)
                if attempt < MAX_RETRIES - 1:
                    await asyncio.sleep(1.5 * (2 ** attempt))
        return None

    async def search(
        self,
        query: str,
        search_depth: str = "advanced",
        topic: str = "general",
        time_range: Optional[str] = None,
        include_domains: Optional[List[str]] = None,
        exclude_domains: Optional[List[str]] = None,
        claim_type: str = "other",
        force_fresh: bool = False,
        max_results: int = 5,
    ) -> Dict[str, Any]:
        """Run a Tavily search, serving from cache when an unexpired entry exists."""
        if not self.is_configured:
            logger.warning("TAVILY_API_KEY is not set; skipping live search.")
            return {"results": [], "error": "Tavily API key not configured"}

        cache_key = generate_cache_key(
            query,
            search_depth,
            topic,
            ",".join(sorted(include_domains or [])),
            time_range or "",
            ",".join(sorted(exclude_domains or [])),
        )
        if not force_fresh:
            cached = get_cached_result(cache_key)
            if cached is not None:
                return cached

        payload: Dict[str, Any] = {
            "query": query,
            "search_depth": search_depth,
            "topic": topic,
            "max_results": max_results,
        }
        if time_range:
            payload["time_range"] = time_range
        if include_domains:
            payload["include_domains"] = include_domains
        if exclude_domains:
            payload["exclude_domains"] = exclude_domains

        data = await self._post("/search", payload)
        if data is None:
            return {"results": [], "error": "Tavily search failed"}
        set_cached_result(cache_key, claim_type, data)
        return data

    async def extract(self, urls: List[str], claim_type: str = "other") -> List[Dict[str, Any]]:
        """Fetch full page content for up to three URLs via Tavily /extract (cached)."""
        urls = [u for u in urls if u][:3]
        if not self.is_configured or not urls:
            return []

        cache_key = generate_cache_key("extract", ",".join(sorted(urls)), "", "")
        cached = get_cached_result(cache_key)
        if cached is not None:
            return cached.get("results", [])

        data = await self._post("/extract", {"urls": urls}, timeout=self.timeout + 5.0)
        if data is None:
            return []
        set_cached_result(cache_key, claim_type, data)
        return data.get("results", [])

    async def execute_claim_search(
        self,
        spec: SearchSpec,
        claim_type: str,
        force_fresh: bool = False,
    ) -> List[Dict[str, Any]]:
        """Primary search, a broader fallback search if results are thin, then
        /extract on the top official-domain hit to read the actual page."""
        search_res = await self.search(
            query=spec.primary_query,
            search_depth=spec.search_depth,
            topic=spec.topic,
            time_range=spec.time_range,
            include_domains=spec.include_domains or None,
            exclude_domains=spec.exclude_domains or None,
            claim_type=claim_type,
            force_fresh=force_fresh,
        )
        results: List[Dict[str, Any]] = list(search_res.get("results", []))

        if len(results) < 2 and spec.secondary_query:
            sec_res = await self.search(
                query=spec.secondary_query,
                search_depth="basic",
                topic=spec.topic,
                exclude_domains=spec.exclude_domains or None,
                claim_type=claim_type,
                force_fresh=force_fresh,
            )
            seen = {r.get("url") for r in results}
            results.extend(r for r in sec_res.get("results", []) if r.get("url") not in seen)

        if spec.should_extract and spec.official_domains:
            official = next(
                (r.get("url", "") for r in results if _matches_domain(r.get("url", ""), spec.official_domains)),
                None,
            )
            if official:
                for ext in await self.extract([official], claim_type=claim_type):
                    raw = ext.get("raw_content")
                    if not raw:
                        continue
                    for item in results:
                        if item.get("url") == ext.get("url"):
                            item["content"] = raw[:EXTRACT_CHARS]
                            item["extracted"] = True

        return results


def _matches_domain(url: str, domains: List[str]) -> bool:
    host = urlparse(url).netloc.lower().removeprefix("www.")
    return any(host == d or host.endswith("." + d) for d in domains)


tavily_client = TavilyClient()
