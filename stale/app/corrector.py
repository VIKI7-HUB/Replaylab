"""Corrected answer synthesis replacing stale claims with live facts and citations."""

from typing import List, Tuple, Dict
from app.schemas import Claim, AnswerChange, Verdict
from app.llm import llm_client

CORRECTOR_SYSTEM_PROMPT = """You are an expert technical editor.
Rewrite the provided original text so that all outdated or stale factual claims are corrected using the verified modern facts provided.

Guidelines:
1. Preserve the original voice, style, formatting, and markdown layout as much as possible.
2. Replace stale assertions with their verified current values.
3. Insert inline bracketed source citations like [1], [2] immediately following the corrected assertions.
4. At the very bottom, output a '### Verified Sources' section listing each citation with title and URL.
"""


def _best_source(claim: Claim):
    """Pick the most trustworthy evidence item for the citation.

    Official and widely-trusted domains are preferred over whatever happened to
    come back first, because search ranking is not the same thing as authority.
    """
    if not claim.evidence:
        return None
    return sorted(
        claim.evidence,
        key=lambda ev: (1 if ev.is_official else 0, ev.relevance_score or 0.0),
        reverse=True,
    )[0]


async def generate_corrected_answer(
    original_text: str,
    claims: List[Claim],
) -> Tuple[str, List[AnswerChange]]:
    """Synthesize a corrected version of the answer with inline citations and diffs."""
    stale_claims = [c for c in claims if c.verdict in (Verdict.STALE, Verdict.PARTIALLY_STALE) and c.current_value]

    if not stale_claims:
        # If no stale claims were detected, return original text unchanged
        return original_text, []

    changes: List[AnswerChange] = []
    citation_map: Dict[int, str] = {}
    citation_counter = 1

    for c in stale_claims:
        # Cite the strongest source, not just the first one: official/trusted domains win,
        # then Tavily relevance breaks the tie.
        best = _best_source(c)
        top_url = best.url if best else "https://tavily.com"
        top_title = best.title if best else "Live Web Ground Truth"
        citation_idx = citation_counter
        citation_counter += 1
        citation_map[citation_idx] = f"[{citation_idx}] [{top_title}]({top_url})"

        changes.append(
            AnswerChange(
                claim_id=c.id,
                original_claim=c.text,
                updated_claim=c.current_value or "",
                reason=c.explanation or "Updated with live verified data",
                citation_index=citation_idx,
            )
        )

    # Attempt LLM rewrite if available
    if llm_client.is_available:
        corrections_brief = []
        for ch in changes:
            corrections_brief.append(
                f"- Original: \"{ch.original_claim}\"\n  Correction: \"{ch.updated_claim}\" (cite as [{ch.citation_index}])"
            )
        sources_brief = "\n".join(citation_map.values())
        user_prompt = (
            f"Original Text:\n{original_text}\n\n"
            f"Mandatory Corrections to Apply:\n" + "\n".join(corrections_brief) + "\n\n"
            f"Sources Bibliography:\n{sources_brief}"
        )
        rewritten = await llm_client.generate_text(CORRECTOR_SYSTEM_PROMPT, user_prompt)
        if rewritten and len(rewritten) > len(original_text) * 0.5:
            return rewritten, changes

    # High-precision deterministic fallback replacement
    corrected = original_text
    for ch in changes:
        # Replace original claim text with updated value + inline citation
        if ch.original_claim in corrected:
            corrected = corrected.replace(
                ch.original_claim,
                f"{ch.updated_claim} [{ch.citation_index}]",
            )
        else:
            # Try fuzzy replacement if exact sentence boundaries differed slightly
            first_words = " ".join(ch.original_claim.split()[:5])
            if first_words in corrected:
                pos = corrected.find(first_words)
                # Find sentence boundary
                end_pos = corrected.find(".", pos)
                if end_pos != -1:
                    target_span = corrected[pos : end_pos + 1]
                    corrected = corrected.replace(
                        target_span,
                        f"{ch.updated_claim} [{ch.citation_index}].",
                    )

    # Append Verified Sources section at the end
    sources_section = "\n\n### Verified Sources\n" + "\n".join(
        [f"- {val}" for val in citation_map.values()]
    )
    corrected += sources_section

    return corrected, changes
