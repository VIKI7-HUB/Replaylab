"""Benchmark evaluation measuring Stale Detection Precision, Recall, and F1."""

import asyncio
import json
import sys
from pathlib import Path

# Add project root to path
sys.path.insert(0, str(Path(__file__).parent.parent))

if sys.platform == "win32":
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except Exception:
        pass

from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from app.llm import llm_client
from app.main import run_pipeline

console = Console(safe_box=True)


async def run_benchmark():
    dataset_path = Path(__file__).parent / "labeled_claims.json"
    with open(dataset_path, "r", encoding="utf-8") as f:
        dataset = json.load(f)

    console.print(
        Panel.fit(
            "[bold white on blue] STALE-ANSWER DETECTOR BENCHMARK [/bold white on blue]\n"
            f"[cyan]Evaluating {len(dataset)} labeled claims "
            f"({sum(d['expected_verdict'] == 'STALE' for d in dataset)} stale, "
            f"{sum(d['expected_verdict'] != 'STALE' for d in dataset)} current) "
            f"in {'LLM' if llm_client.is_available else 'heuristic'} mode[/cyan]",
            border_style="cyan",
        )
    )

    tp, fp, tn, fn = 0, 0, 0, 0
    abstained = 0  # UNVERIFIABLE: the engine declined to decide
    tavily_calls = 0

    results_table = Table(title="Evaluation Results", header_style="bold magenta")
    results_table.add_column("ID", width=6)
    results_table.add_column("Claim Statement", width=42)
    results_table.add_column("Expected", width=10)
    results_table.add_column("Predicted", width=12)
    results_table.add_column("Outcome", width=10)

    for item in dataset:
        claim_id = item["id"]
        text = item["text"]
        expected = item["expected_verdict"]

        report = await run_pipeline(answer=text)
        tavily_calls += report.usage.tavily_calls
        # A claim skipped by triage is treated as "not stale".
        if not report.claims or report.claims[0].skipped:
            pred_verdict = "SKIPPED"
        else:
            pred_verdict = report.claims[0].verdict.value if report.claims[0].verdict else "UNVERIFIABLE"
        if pred_verdict == "UNVERIFIABLE":
            abstained += 1

        is_pred_stale = pred_verdict in ("STALE", "PARTIALLY_STALE")
        is_exp_stale = (expected == "STALE")

        if is_exp_stale and is_pred_stale:
            tp += 1
            outcome = "[bold green]TP (Hit)[/bold green]"
        elif not is_exp_stale and not is_pred_stale:
            tn += 1
            outcome = "[bold green]TN (OK)[/bold green]"
        elif not is_exp_stale and is_pred_stale:
            fp += 1
            outcome = "[bold red]FP (Err)[/bold red]"
        else:
            fn += 1
            outcome = "[bold yellow]FN (Miss)[/bold yellow]"

        results_table.add_row(
            claim_id,
            text[:40] + ("..." if len(text) > 40 else ""),
            "[bold red]STALE[/bold red]" if expected == "STALE" else "[bold green]CURRENT[/bold green]",
            pred_verdict,
            outcome,
        )

    console.print(results_table)

    # Compute classification metrics
    precision = (tp / (tp + fp)) * 100.0 if (tp + fp) > 0 else 0.0
    recall = (tp / (tp + fn)) * 100.0 if (tp + fn) > 0 else 0.0
    f1 = (2 * precision * recall) / (precision + recall) if (precision + recall) > 0 else 0.0
    accuracy = ((tp + tn) / len(dataset)) * 100.0

    metrics_table = Table(title="Benchmark Performance Metrics", header_style="bold green")
    metrics_table.add_column("Metric", style="bold")
    metrics_table.add_column("Value", style="cyan")

    metrics_table.add_row("Total Labeled Samples", str(len(dataset)))
    metrics_table.add_row("True Positives (Stale Caught)", str(tp))
    metrics_table.add_row("True Negatives (Current Kept)", str(tn))
    metrics_table.add_row("False Positives", str(fp))
    metrics_table.add_row("False Negatives", str(fn))
    metrics_table.add_row("Precision (Staleness)", f"{precision:.1f}%")
    metrics_table.add_row("Recall (Staleness Detection)", f"{recall:.1f}%")
    metrics_table.add_row("F1 Score", f"{f1:.1f}%")
    metrics_table.add_row("Overall Accuracy", f"{accuracy:.1f}%")
    metrics_table.add_row("Abstained (UNVERIFIABLE)", str(abstained))
    metrics_table.add_row("Tavily calls used", str(tavily_calls))

    console.print(metrics_table)
    console.print(
        f"\n[bold green]Done: {precision:.0f}% precision, {recall:.0f}% recall, "
        f"{abstained} abstentions on {len(dataset)} claims.[/bold green]"
    )


if __name__ == "__main__":
    asyncio.run(run_benchmark())
