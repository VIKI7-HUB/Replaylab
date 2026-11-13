"""Per-request usage accounting.

Each pipeline run installs its own ``RequestUsage`` in a context variable, so
counts stay correct when several requests run concurrently (a global counter
diffed before/after would mix them). Lifetime totals are kept separately for
the health endpoint.
"""

from contextvars import ContextVar
from dataclasses import dataclass
from typing import Optional


@dataclass
class RequestUsage:
    tavily_calls: int = 0
    tavily_calls_saved: int = 0
    llm_calls: int = 0
    llm_verdicts: int = 0  # claims whose verdict came from the LLM judge


@dataclass
class LifetimeUsage:
    tavily_calls: int = 0
    tavily_calls_saved: int = 0
    llm_calls: int = 0


lifetime = LifetimeUsage()
_current: ContextVar[Optional[RequestUsage]] = ContextVar("request_usage", default=None)


def start_request() -> RequestUsage:
    """Begin accounting for the current task (and tasks it spawns)."""
    usage = RequestUsage()
    _current.set(usage)
    return usage


def record_tavily_call() -> None:
    lifetime.tavily_calls += 1
    if (u := _current.get()) is not None:
        u.tavily_calls += 1


def record_cache_hit() -> None:
    lifetime.tavily_calls_saved += 1
    if (u := _current.get()) is not None:
        u.tavily_calls_saved += 1


def record_llm_call() -> None:
    lifetime.llm_calls += 1
    if (u := _current.get()) is not None:
        u.llm_calls += 1


def record_llm_verdict() -> None:
    if (u := _current.get()) is not None:
        u.llm_verdicts += 1
