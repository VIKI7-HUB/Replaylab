"""Smart query cache with claim-type-based TTL policies."""

import hashlib
import json
import time
from typing import Optional, Dict, Any
from app import usage
from app.config import settings
from app.db import get_db_connection

# TTL policies in seconds
TTL_SECONDS_MAP = {
    "pricing": 6 * 3600,            # 6 hours (pricing changes periodically)
    "leadership_people": 30 * 60,   # 30 minutes (personnel & executive moves are fast)
    "event": 30 * 60,               # 30 minutes (live breaking events)
    "version": 12 * 3600,           # 12 hours (software releases)
    "product_availability": 12 * 3600,
    "law_regulation": 24 * 3600,    # 24 hours (legislation & regulatory rulings)
    "statistic": 12 * 3600,
    "timeless": 30 * 86400,         # 30 days
    "other": 6 * 3600,
}

def get_calls_saved() -> int:
    """Lifetime number of Tavily calls served from cache."""
    return usage.lifetime.tavily_calls_saved


def generate_cache_key(*parts: str) -> str:
    """Deterministic hash key over the normalised request parameters."""
    raw = "|".join((p or "").strip().lower() for p in parts)
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()


def get_cached_result(cache_key: str) -> Optional[Dict[str, Any]]:
    """Look up unexpired search response from SQLite cache."""
    if not settings.cache_enabled:
        return None

    now = time.time()
    try:
        with get_db_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(
                "SELECT result_json, expires_at FROM query_cache WHERE cache_key = ?",
                (cache_key,),
            )
            row = cursor.fetchone()
            if row and row["expires_at"] > now:
                usage.record_cache_hit()
                return json.loads(row["result_json"])
    except Exception:
        # Cache lookup must never fail the request
        return None
    return None


def set_cached_result(cache_key: str, claim_type: str, result_data: Dict[str, Any]) -> None:
    """Store search response with claim-type-specific TTL."""
    if not settings.cache_enabled:
        return

    ttl = TTL_SECONDS_MAP.get(claim_type, 6 * 3600)
    now = time.time()
    expires_at = now + ttl

    try:
        with get_db_connection() as conn:
            cursor = conn.cursor()
            cursor.execute(
                """
                INSERT OR REPLACE INTO query_cache (cache_key, claim_type, created_at, expires_at, result_json)
                VALUES (?, ?, ?, ?, ?)
                """,
                (cache_key, claim_type, now, expires_at, json.dumps(result_data)),
            )
            conn.commit()
    except Exception:
        # Cache storage failure is non-fatal
        pass
