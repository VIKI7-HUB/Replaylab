"""Staleness-risk triage engine prioritizing claims by volatility and temporal cues."""

import re
from datetime import datetime, timezone
from typing import List, Optional, Tuple
from app.schemas import Claim, ClaimType

# Volatility priors based on domain rate of change
VOLATILITY_PRIORS = {
    ClaimType.PRICING: 0.88,
    ClaimType.VERSION: 0.82,
    ClaimType.LEADERSHIP_PEOPLE: 0.78,
    ClaimType.PRODUCT_AVAILABILITY: 0.72,
    ClaimType.LAW_REGULATION: 0.68,
    ClaimType.STATISTIC: 0.60,
    ClaimType.EVENT: 0.50,
    ClaimType.OTHER: 0.40,
    ClaimType.TIMELESS: 0.05,
}

TEMPORAL_KEYWORDS = re.compile(
    r'\b(current(ly)?|latest|now|as of|recent(ly)?|today|presently|modern)\b',
    re.IGNORECASE,
)
YEAR_PATTERN = re.compile(r'\b(19\d\d|20\d\d)\b')
MATH_SCIENCE_PATTERN = re.compile(
    r'\b(formula|equals|multiplied|divided|constant|speed of light|boiling point|pythagorean|atomic number|newton)\b',
    re.IGNORECASE,
)


def compute_staleness_risk(claim: Claim, answer_date: Optional[str] = None) -> Tuple[float, List[str], bool, str]:
    """
    Compute staleness risk (0.0 to 1.0) for a claim and determine whether to triage/skip.
    Returns: (risk_score, reasons, skipped, skip_reason)
    """
    reasons: List[str] = []
    base_prior = VOLATILITY_PRIORS.get(claim.claim_type, 0.40)
    reasons.append(f"Domain volatility prior ({claim.claim_type.value}): {base_prior:.2f}")

    risk = base_prior
    text_lower = claim.text.lower()

    # Check for timeless math, scientific laws, or historical facts
    if MATH_SCIENCE_PATTERN.search(text_lower):
        risk = min(risk, 0.05)
        reasons.append("Identified timeless mathematical or scientific invariant")
        return (round(risk, 3), reasons, True, "not_checked: timeless invariant")

    # Historical year check (e.g. before 2000)
    years = [int(y) for y in YEAR_PATTERN.findall(claim.text)]
    current_year = datetime.now(timezone.utc).year
    if years:
        oldest_year = min(years)
        newest_year = max(years)
        if newest_year < 2000:
            risk = 0.08
            reasons.append(f"Historical event citation ({oldest_year})")
            return (round(risk, 3), reasons, True, "not_checked: timeless historical fact")
        elif newest_year <= current_year - 3:
            # Older reference year in an evolving tech/policy field increases risk
            risk += 0.15
            reasons.append(f"Explicit older reference year ({newest_year}) indicates likely model cutoff staleness")
        elif newest_year >= current_year - 2:
            risk += 0.05
            reasons.append(f"Contemporary reference year ({newest_year}) requires recent live verification")

    # An answer generated long ago is more likely to have drifted.
    if answer_date:
        try:
            generated = datetime.fromisoformat(answer_date[:10]).date()
            age_days = (datetime.now(timezone.utc).date() - generated).days
            if age_days > 365:
                risk += 0.10
                reasons.append(f"Answer generated {age_days // 365}+ year(s) ago")
        except ValueError:
            reasons.append("Ignored unparseable answer_date")

    # Temporal cues check
    if TEMPORAL_KEYWORDS.search(claim.text):
        risk += 0.15
        reasons.append("Presence of temporal markers ('currently', 'latest', 'now')")

    # Clamp risk to [0.0, 1.0]
    risk = max(0.0, min(1.0, risk))

    # Triage decision: skip claims with risk < 0.25 to conserve credits
    skipped = False
    skip_reason = None
    if risk < 0.25 or claim.claim_type == ClaimType.TIMELESS:
        skipped = True
        skip_reason = "not_checked: timeless or low volatility prior"

    return (round(risk, 3), reasons, skipped, skip_reason)


def triage_claims(claims: List[Claim], answer_date: Optional[str] = None) -> List[Claim]:
    """Triage all claims, updating risk scores and triage flags."""
    triaged: List[Claim] = []
    for c in claims:
        risk, reasons, skipped, skip_reason = compute_staleness_risk(c, answer_date)
        c.staleness_risk = risk
        c.risk_reasons = reasons
        c.skipped = skipped
        c.skip_reason = skip_reason
        triaged.append(c)
    return triaged
