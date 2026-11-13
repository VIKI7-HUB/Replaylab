"""Claim extraction engine supporting both LLM structured mode and heuristic rule-based extraction."""

import logging
import re
from typing import List
from app.schemas import Claim, ClaimType, QuotedSpan
from app.llm import llm_client

logger = logging.getLogger(__name__)

EXTRACTOR_SYSTEM_PROMPT = """You are a precision fact-checker and claim extraction specialist.
Analyze the provided text and extract atomic, verifiable factual claims.
Focus specifically on claims that are subject to staleness or changes over time:
- Pricing and plan rates
- Software / API versions and model releases
- Laws, regulations, and tax brackets
- Leadership, CEOs, executives, founders
- Statistics, numbers, market caps, user counts
- Product availability, deprecations, beta statuses, feature launches
- Specific dated events

For each claim return a JSON object with:
- id: string e.g. "claim-1"
- text: the exact factual statement
- entity: the subject entity (e.g., "OpenAI", "Twitter / X", "Python")
- claim_type: one of ["pricing", "version", "law_regulation", "leadership_people", "statistic", "product_availability", "event", "other", "timeless"]
- start_char: integer character offset in the original text
- end_char: integer character offset in the original text

Output JSON format:
{
  "claims": [
    {
      "id": "claim-1",
      "text": "OpenAI GPT-4 costs $0.03 per 1K input tokens",
      "entity": "OpenAI",
      "claim_type": "pricing",
      "start_char": 0,
      "end_char": 45
    }
  ]
}
"""

# Regex patterns for heuristic extraction
PRICE_REGEX = re.compile(
    r'(\$\d+[\d,\.]*(\s*(per|\/)\s*(month|mo|year|yr|1k|1m|million|token|user|seat|hr))?|\b\d+\s*cents\b|free tier)',
    re.IGNORECASE,
)
VERSION_REGEX = re.compile(
    r'\b(v\d+(\.\d+)+|version\s+\d+(\.\d+)*|(python|node(?:\.js)?|react|vue|next(?:\.js)?|angular|gpt-\d+(\.\d+)?|claude\s*\d+(\.\d+)?|gemini\s*\d+(\.\d+)?)\s*(\d+(\.\d+)*)?)',
    re.IGNORECASE,
)
LEADERSHIP_REGEX = re.compile(
    r'\b(CEO|Chief Executive Officer|CTO|Chief Technology Officer|President|founded by|founder|appointed as|leads|stepped down)\b',
    re.IGNORECASE,
)
LAW_REGEX = re.compile(
    r'\b(GDPR|AI Act|Section 230|regulation|tax rate|federal rate|statute|compliance|ban|tariff)\b',
    re.IGNORECASE,
)
STAT_REGEX = re.compile(
    r'\b(\d+(\.\d+)?%|\d+(\.\d+)?\s*(billion|million|trillion))\b',
    re.IGNORECASE,
)
PRODUCT_REGEX = re.compile(
    r'\b(deprecated|in preview|in beta|general availability|discontinued|sunset|shut down|waitlist)\b',
    re.IGNORECASE,
)
KNOWN_ENTITIES = [
    "OpenAI", "Anthropic", "Google", "Microsoft", "Meta", "Twitter", "X", "Stripe",
    "Apple", "Amazon", "AWS", "Python", "React", "Next.js", "Node.js", "Docker",
    "PostgreSQL", "GitHub", "Vercel", "Tavily", "Tesla", "Nvidia"
]


def _detect_entity(sentence: str) -> str:
    """Identify the primary entity mentioned in a sentence."""
    for entity in KNOWN_ENTITIES:
        if re.search(rf"\b{re.escape(entity)}\b", sentence, re.IGNORECASE):
            return entity
    # Fallback: look for capitalized words or phrases
    match = re.search(r'\b[A-Z][a-zA-Z0-9_\-\.]{2,}\b', sentence)
    if match:
        return match.group(0)
    return "General"


def extract_claims_heuristic(text: str, max_claims: int = 10) -> List[Claim]:
    """Fallback high-precision rule-based extractor using NLP patterns."""
    claims: List[Claim] = []
    # Split text into sentences without splitting on decimals/versions like 3.9 or $49.00
    sentence_pattern = re.compile(r'((?:[^\n.!?]|(?<=\d)\.(?=\d)|(?<=[A-Za-z])\.(?=[A-Za-z]))+[.!?\n]*)')
    matches = list(sentence_pattern.finditer(text))

    claim_idx = 1
    for match in matches:
        sentence = match.group(0).strip()
        start = match.start()
        end = match.end()

        if len(sentence) < 15:
            continue

        c_type = ClaimType.OTHER
        if PRICE_REGEX.search(sentence):
            c_type = ClaimType.PRICING
        elif LEADERSHIP_REGEX.search(sentence):
            c_type = ClaimType.LEADERSHIP_PEOPLE
        elif VERSION_REGEX.search(sentence):
            c_type = ClaimType.VERSION
        elif LAW_REGEX.search(sentence):
            c_type = ClaimType.LAW_REGULATION
        elif PRODUCT_REGEX.search(sentence):
            c_type = ClaimType.PRODUCT_AVAILABILITY
        elif STAT_REGEX.search(sentence):
            c_type = ClaimType.STATISTIC
        elif re.search(r'\b(19\d\d|18\d\d|17\d\d)\b', sentence) or any(m in sentence.lower() for m in ["landed on the moon", "formula", "equals", "constant"]):
            c_type = ClaimType.TIMELESS
        elif any(k in sentence.lower() for k in ["released", "announced", "hosted", "acquired"]):
            c_type = ClaimType.EVENT

        # If it matches any high-volatility pattern or has temporal cues
        has_temporal = bool(re.search(r'\b(current|latest|now|as of|in 20\d\d|today)\b', sentence, re.IGNORECASE))
        if c_type != ClaimType.OTHER or has_temporal:
            entity = _detect_entity(sentence)
            claims.append(
                Claim(
                    id=f"claim-{claim_idx}",
                    text=sentence,
                    entity=entity,
                    claim_type=c_type,
                    quoted_span=QuotedSpan(start=start, end=end),
                )
            )
            claim_idx += 1
            if len(claims) >= max_claims:
                break

    # If no specific patterns matched, extract first 1-2 informative sentences
    if not claims and matches:
        first_sent = matches[0].group(0).strip()
        if len(first_sent) >= 10:
            claims.append(
                Claim(
                    id="claim-1",
                    text=first_sent,
                    entity=_detect_entity(first_sent),
                    claim_type=ClaimType.OTHER,
                    quoted_span=QuotedSpan(start=matches[0].start(), end=matches[0].end()),
                )
            )

    return claims


async def extract_claims(text: str, max_claims: int = 10) -> List[Claim]:
    """Extract atomic verifiable claims using LLM if available, else heuristic engine."""
    if llm_client.is_available:
        try:
            user_prompt = f"Extract up to {max_claims} atomic claims from this answer:\n\n{text}"
            res = await llm_client.call_json(EXTRACTOR_SYSTEM_PROMPT, user_prompt)
            if res and "claims" in res and isinstance(res["claims"], list):
                extracted: List[Claim] = []
                for idx, c in enumerate(res["claims"][:max_claims], start=1):
                    c_id = c.get("id") or f"claim-{idx}"
                    claim_text = c.get("text", "").strip()
                    if not claim_text:
                        continue
                    
                    claim_type_raw = str(c.get("claim_type", "other")).lower().replace("-", "_").replace(" ", "_")
                    try:
                        c_type = ClaimType(claim_type_raw)
                    except ValueError:
                        c_type = ClaimType.OTHER

                    span = None
                    if "start_char" in c and "end_char" in c:
                        try:
                            s = int(c["start_char"])
                            e = int(c["end_char"])
                            if 0 <= s < e <= len(text):
                                span = QuotedSpan(start=s, end=e)
                        except (ValueError, TypeError):
                            pass

                    # If span not valid, locate text in source
                    if not span:
                        pos = text.find(claim_text[:30])
                        if pos != -1:
                            span = QuotedSpan(start=pos, end=min(len(text), pos + len(claim_text)))

                    extracted.append(
                        Claim(
                            id=c_id,
                            text=claim_text,
                            entity=c.get("entity") or _detect_entity(claim_text),
                            claim_type=c_type,
                            quoted_span=span,
                        )
                    )
                if extracted:
                    return extracted
        except Exception:
            logger.exception("LLM claim extraction failed; using heuristic extractor")

    # Heuristic fallback
    return extract_claims_heuristic(text, max_claims=max_claims)
