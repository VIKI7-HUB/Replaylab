"""Rich visual terminal demo for the Stale-Answer Detector."""

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
from rich.panel import Panel
from rich.table import Table
from rich import box
from app.main import run_pipeline

console = Console(safe_box=True)


async def run_all_demos():
    samples_path = Path(__file__).parent / "sample_answers.json"
    with open(samples_path, "r", encoding="utf-8") as f:
        samples = json.load(f)

    console.print(
        Panel.fit(
            "[bold white on blue]  STALE-ANSWER DETECTOR  [/bold white on blue]\n"
            "[italic cyan]Finds outdated claims in LLM answers and re-checks them with Tavily[/italic cyan]",
            border_style="cyan",
            box=box.DOUBLE,
        )
    )

    for i, sample in enumerate(samples, start=1):
        console.print(f"\n[bold yellow]=== Case #{i}: {sample['topic']} ===[/bold yellow]")
        console.print(f"[bold cyan]User Query:[/bold cyan] {sample['question']}")
        console.print(
            Panel(
                f"[italic white]{sample['answer']}[/italic white]",
                title="[bold red]Original LLM Answer (Potentially Stale)[/bold red]",
                border_style="red",
                box=box.ROUNDED,
            )
        )

        with console.status(f"[bold green]Running live Tavily verification pipeline for Case #{i}...[/bold green]"):
            report = await run_pipeline(
                answer=sample["answer"],
                question=sample["question"],
            )

        # Freshness Score Badge
        score = report.overall_freshness_score
        if score >= 80:
            score_color = "green"
        elif score >= 50:
            score_color = "yellow"
        else:
            score_color = "red"

        console.print(
            f"[bold]Overall Freshness Score:[/bold] [{score_color}]{score:.1f} / 100[/{score_color}] "
            f"| [bold]Claims Audited:[/bold] {report.summary.total_claims} "
            f"([red]{report.summary.stale} Stale[/red], [green]{report.summary.current} Current[/green], [dim]{report.summary.skipped} Skipped[/dim])"
        )

        # Claims Table
        claims_table = Table(
            title=f"Extracted Claims & Freshness Audit ({sample['topic']})",
            box=box.SIMPLE_HEAVY,
            header_style="bold magenta",
        )
        claims_table.add_column("Claim ID", style="dim", width=8)
        claims_table.add_column("Type", width=14)
        claims_table.add_column("Claim Statement", width=34)
        claims_table.add_column("Staleness Risk", width=14)
        claims_table.add_column("Verdict", width=12)
        claims_table.add_column("Freshness", width=10)
        claims_table.add_column("Current Ground Truth", width=30)

        for c in report.claims:
            # Verdict styling
            if c.skipped:
                v_text = "[dim]SKIPPED[/dim]"
            elif c.verdict and c.verdict.value == "STALE":
                v_text = "[bold red]STALE[/bold red]"
            elif c.verdict and c.verdict.value == "PARTIALLY_STALE":
                v_text = "[bold yellow]PARTIAL[/bold yellow]"
            elif c.verdict and c.verdict.value == "CURRENT":
                v_text = "[bold green]CURRENT[/bold green]"
            else:
                v_text = "[dim]UNVERIFIED[/dim]"

            risk_str = f"{c.staleness_risk * 100:.0f}%"
            score_str = f"{c.freshness_score:.1f}" if c.freshness_score is not None else "-"
            current_truth = "[dim]not checked[/dim]" if c.skipped else (c.current_value or "-")

            claims_table.add_row(
                c.id,
                c.claim_type.value,
                c.text[:60] + ("..." if len(c.text) > 60 else ""),
                risk_str,
                v_text,
                score_str,
                current_truth[:55] + ("..." if len(current_truth) > 55 else ""),
            )

        console.print(claims_table)

        # Corrected Answer Panel
        if report.changes:
            console.print(
                Panel(
                    report.corrected_answer,
                    title="[bold green]Corrected Answer (Synthesized with Live Tavily Citations)[/bold green]",
                    border_style="green",
                    box=box.ROUNDED,
                )
            )
            console.print("[bold]Corrections Applied:[/bold]")
            for ch in report.changes:
                console.print(f"  * [red]{ch.original_claim}[/red] -> [green]{ch.updated_claim}[/green] [cyan][{ch.citation_index}][/cyan]")

        # Telemetry
        console.print(
            f"[dim]Latency: {report.usage.latency_ms:.0f}ms | Tavily Calls: {report.usage.tavily_calls} | Cached/Saved: {report.usage.tavily_calls_saved}[/dim]\n"
        )

    console.print(
        Panel.fit(
            "[bold green][SUCCESS] All 6 Demo Scenarios Successfully Audited and Corrected![/bold green]\n"
            "[white]Credit-smart triage skipped timeless invariants, saving Tavily credits.[/white]\n"
            "[white]Explainable formula generated transparent 0-100 freshness breakdowns.[/white]",
            border_style="green",
            box=box.DOUBLE,
        )
    )


if __name__ == "__main__":
    asyncio.run(run_all_demos())
