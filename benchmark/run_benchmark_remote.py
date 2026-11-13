"""Benchmark a deployed Stale-Answer Detector API on the labeled claim set.

The local benchmark (``run_benchmark.py``) exercises the pipeline in-process.
This script points the same 30 labeled claims at a running service instead,
which is how you measure the *deployed* configuration — the one judges see —
including its LLM judge and its Tavily usage.

Usage:
    python benchmark/run_benchmark_remote.py
    python benchmark/run_benchmark_remote.py --api https://stale-detector-api.onrender.com
    python benchmark/run_benchmark_remote.py --api http://localhost:8000 --save benchmark/remote_results.json

Metrics match the local benchmark exactly: a skipped claim counts as "not
stale", and UNVERIFIABLE is reported separately as an abstention.
"""

import argparse
import asyncio
import json
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent))

if sys.platform == "win32":
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except Exception:
        pass

import httpx

DEFAULT_API = "https://stale-detector-api.onrender.com"


async def check_claim(client: httpx.AsyncClient, api: str, text: str) -> dict:
    """Send one claim to the API and return a flat result row."""
    started = time.perf_counter()
    resp = await client.post(f"{api}/v1/check", json={"answer": text})
    resp.raise_for_status()
    report = resp.json()
    claim = report["claims"][0] if report.get("claims") else {}
    verdict = claim.get("verdict")
    skipped = bool(claim.get("skipped"))
    return {
        "id": None,
        "expected": None,
        "predicted": "SKIPPED" if skipped else (verdict or "UNVERIFIABLE"),
        "skipped": skipped,
        "abstained": (not skipped) and verdict == "UNVERIFIABLE",
        "score": claim.get("freshness_score"),
        "current_value": (claim.get("current_value") or "")[:90],
        "explanation": (claim.get("explanation") or "")[:110],
        "latency_ms": round((time.perf_counter() - started) * 1000, 1),
        "tavily_calls": report.get("usage", {}).get("tavily_calls", 0),
        "llm_calls": report.get("usage", {}).get("llm_calls", 0),
        "mode": report.get("verification_mode"),
    }


async def run(api: str, concurrency: int, save: str | None) -> int:
    dataset_path = Path(__file__).parent / "labeled_claims.json"
    dataset = json.loads(dataset_path.read_text(encoding="utf-8"))

    health = {}
    try:
        async with httpx.AsyncClient(timeout=90) as probe:
            health = (await probe.get(f"{api}/v1/health")).json()
    except Exception:
        pass

    print(f"\n  Remote benchmark → {api}")
    print(f"  service mode: {health.get('verification_mode', 'unknown')} | "
          f"llm_configured: {health.get('llm_configured')} | "
          f"tavily_configured: {health.get('tavily_configured')}\n")

    semaphore = asyncio.Semaphore(concurrency)

    async with httpx.AsyncClient(timeout=120) as client:
        async def run_one(item):
            async with semaphore:
                try:
                    row = await check_claim(client, api, item["text"])
                except Exception as err:  # keep the run alive on a single failure
                    row = {"id": item["id"], "expected": item["expected_verdict"],
                           "predicted": "ERROR", "skipped": False, "abstained": False,
                           "score": None, "current_value": "", "explanation": str(err)[:110],
                           "latency_ms": None, "tavily_calls": 0, "llm_calls": 0, "mode": None}
                row["id"] = item["id"]
                row["expected"] = item["expected_verdict"]
                row["category"] = item.get("category", "")
                return row

        results = await asyncio.gather(*(run_one(item) for item in dataset))

    tp = fp = tn = fn = 0
    abstained = skipped = errors = 0
    tavily_total = llm_total = 0
    latencies = []

    print(f"  {'ID':<6}{'EXPECTED':<10}{'PREDICTED':<16}{'SCORE':>7}  {'CATEGORY':<20}CURRENT VALUE")
    print("  " + "-" * 108)
    for row in results:
        pred = row["predicted"]
        is_pred_stale = pred in ("STALE", "PARTIALLY_STALE")
        is_exp_stale = row["expected"] == "STALE"
        if is_exp_stale and is_pred_stale:
            tp += 1
        elif is_exp_stale:
            fn += 1
        elif is_pred_stale:
            fp += 1
        else:
            tn += 1
        if row["abstained"]:
            abstained += 1
        if row["skipped"]:
            skipped += 1
        if pred == "ERROR":
            errors += 1
        tavily_total += row["tavily_calls"]
        llm_total += row["llm_calls"]
        if row["latency_ms"]:
            latencies.append(row["latency_ms"])
        score = f"{row['score']:.1f}" if row["score"] is not None else "-"
        print(f"  {row['id']:<6}{row['expected']:<10}{pred:<16}{score:>7}  "
              f"{row.get('category',''):<20}{row['current_value'][:42]}")

    precision = (tp / (tp + fp) * 100) if (tp + fp) else 0.0
    recall = (tp / (tp + fn) * 100) if (tp + fn) else 0.0
    f1 = (2 * precision * recall / (precision + recall)) if (precision + recall) else 0.0
    accuracy = ((tp + tn) / len(results) * 100) if results else 0.0
    avg_latency = sum(latencies) / len(latencies) if latencies else 0.0

    print("\n  ── Remote benchmark metrics " + "─" * 52)
    for label, value in [
        ("Samples", len(results)),
        ("True positives (stale caught)", tp),
        ("False negatives (stale missed)", fn),
        ("False positives", fp),
        ("True negatives", tn),
        ("Precision", f"{precision:.1f}%"),
        ("Recall", f"{recall:.1f}%"),
        ("F1", f"{f1:.1f}%"),
        ("Accuracy", f"{accuracy:.1f}%"),
        ("Abstentions (UNVERIFIABLE)", abstained),
        ("Skipped (timeless / low risk)", skipped),
        ("Errors", errors),
        ("Tavily calls spent", tavily_total),
        ("LLM calls", llm_total),
        ("Average latency", f"{avg_latency / 1000:.1f}s"),
        ("Verification mode", results[0]["mode"] if results else "-"),
    ]:
        print(f"  {label:<32}{value}")

    if save:
        payload = {
            "api": api, "health": health, "metrics": {
                "precision": precision, "recall": recall, "f1": f1, "accuracy": accuracy,
                "tp": tp, "fp": fp, "tn": tn, "fn": fn, "abstentions": abstained,
                "skipped": skipped, "errors": errors, "tavily_calls": tavily_total,
                "llm_calls": llm_total, "avg_latency_ms": round(avg_latency, 1),
            },
            "results": results,
        }
        Path(save).write_text(json.dumps(payload, indent=2), encoding="utf-8")
        print(f"\n  Saved → {save}")

    return 0


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Benchmark a deployed Stale-Answer Detector API")
    parser.add_argument("--api", default=DEFAULT_API, help="Base URL of the running service")
    parser.add_argument("--concurrency", type=int, default=3, help="Parallel checks (keep it polite)")
    parser.add_argument("--save", default=None, help="Write the full JSON result here")
    args = parser.parse_args()
    sys.exit(asyncio.run(run(args.api.rstrip("/"), args.concurrency, args.save)))
