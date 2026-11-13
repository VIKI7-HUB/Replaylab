"""Smart query generator crafting targeted Tavily search and extract specifications."""

import re
from datetime import datetime, timezone
from typing import List, Optional

from pydantic import BaseModel, Field
from app.schemas import Claim, ClaimType

OFFICIAL_DOMAIN_MAP = {
    "openai": ["openai.com"],
    "anthropic": ["anthropic.com"],
    "google": ["google.com", "ai.google.dev", "cloud.google.com"],
    "microsoft": ["microsoft.com", "azure.microsoft.com"],
    "twitter": ["x.com", "twitter.com"],
    "x": ["x.com"],
    "stripe": ["stripe.com"],
    "python": ["python.org", "docs.python.org"],
    "next.js": ["nextjs.org"],
    "react": ["react.dev"],
    "node.js": ["nodejs.org"],
    "docker": ["docker.com", "docs.docker.com"],
    "tavily": ["tavily.com", "docs.tavily.com"],
    "apple": ["apple.com"],
    "meta": ["meta.com", "about.meta.com"],
}


class SearchSpec(BaseModel):
    """Execution parameters tailored for Tavily API."""

    primary_query: str
    secondary_query: Optional[str] = None
    topic: str = "general"
    time_range: Optional[str] = None
    search_depth: str = "advanced"
    include_domains: List[str] = Field(default_factory=list)
    exclude_domains: List[str] = Field(default_factory=list)
    official_domains: List[str] = Field(default_factory=list)
    should_extract: bool = False


ROLE_REGEX = re.compile(
    r"\b(CEO|CTO|CFO|COO|Chief [A-Z][a-z]+ Officer|President|Chair(?:man|woman|person)?|founder)\b",
    re.IGNORECASE,
)

# Claim types whose ground truth lives on the vendor's own site. For these the
# primary search is restricted to official domains; news-like claims are not,
# because an official domain rarely reports its own leadership changes.
OFFICIAL_FIRST_TYPES = {ClaimType.PRICING, ClaimType.VERSION}


def official_domains_for(entity: str) -> List[str]:
    """Official domains for an entity, matched on whole words."""
    words = set(re.findall(r"[a-z0-9.]+", entity.lower()))
    for key, domains in OFFICIAL_DOMAIN_MAP.items():
        if key in words:
            return list(domains)
    return []


def generate_search_spec(
    claim: Claim,
    depth_override: Optional[str] = None,
    domains_allow: Optional[List[str]] = None,
    domains_block: Optional[List[str]] = None,
) -> SearchSpec:
    """Formulate search queries and pick Tavily parameters for one claim."""
    entity = claim.entity if claim.entity and claim.entity != "General" else ""
    c_type = claim.claim_type
    search_depth = depth_override if depth_override in ("basic", "advanced") else "advanced"
    year = datetime.now(timezone.utc).year

    official_domains = official_domains_for(entity)

    # Extract salient keywords from claim text by removing common stopwords
    cleaned_text = re.sub(r'\b(the|a|an|is|are|was|were|has|have|been|that|this|of|in|to|for|with)\b', '', claim.text, flags=re.IGNORECASE)
    cleaned_text = " ".join(cleaned_text.split()[:8])

    topic = "general"
    time_range = None
    should_extract = False

    if c_type == ClaimType.LEADERSHIP_PEOPLE:
        # A year window: leadership changes are reported once, not monthly.
        topic = "news"
        time_range = "year"
        entity_query = "Twitter X" if entity.lower() in ("twitter", "x") else entity
        role_match = ROLE_REGEX.search(claim.text)
        role = role_match.group(0) if role_match else "CEO"
        primary_query = f"who is the current {role} of {entity_query}".strip()
        secondary_query = f"{entity_query} {role} appointed {year}".strip()

    elif c_type == ClaimType.EVENT:
        topic = "news"
        time_range = "month"
        primary_query = f"{entity} {cleaned_text} latest updates".strip()
        secondary_query = f"{entity} recent announcements {year}".strip()

    elif c_type == ClaimType.PRICING:
        topic = "general"
        should_extract = True  # Pricing tables benefit from exact URL extraction
        primary_query = f"{entity} {cleaned_text} official pricing {year}".strip()
        secondary_query = f"{cleaned_text} cost pricing".strip()

    elif c_type == ClaimType.VERSION:
        topic = "general"
        should_extract = True
        # Naming the year pulls in the current downloads and release pages
        # rather than the notes of whichever old release ranks first.
        primary_query = f"{entity} latest stable release version {year}".strip()
        secondary_query = f"{cleaned_text} current version release notes".strip()

    elif c_type == ClaimType.LAW_REGULATION:
        topic = "general"
        primary_query = f"{entity} {cleaned_text} legal regulatory status current".strip()
        secondary_query = f"{cleaned_text} official law statute".strip()

    elif c_type == ClaimType.PRODUCT_AVAILABILITY:
        topic = "general"
        primary_query = f"{entity} {cleaned_text} availability status deprecated beta".strip()
        secondary_query = f"{entity} product lifecycle status".strip()

    else:
        primary_query = f"{entity} {cleaned_text} current facts".strip()
        secondary_query = f"{cleaned_text} verified".strip()

    if domains_allow:
        include = list(domains_allow)
    elif c_type in OFFICIAL_FIRST_TYPES:
        include = official_domains
    else:
        include = []

    return SearchSpec(
        primary_query=" ".join(primary_query.split())[:400],
        secondary_query=" ".join(secondary_query.split())[:400] if secondary_query else None,
        topic=topic,
        time_range=time_range,
        search_depth=search_depth,
        include_domains=include,
        exclude_domains=list(domains_block or []),
        official_domains=official_domains,
        should_extract=should_extract and bool(official_domains),
    )
