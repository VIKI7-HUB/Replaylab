"""Rich terminal demo that audits the sample answers through a *running service*.

Same six realistic stale answers as ``run_demo.py``, but instead of running the
pipeline in-process it talks to a deployed Stale-Answer Detector — the exact
thing judges see on stage. Useful as a fallback when a local server is not an
option, and as a way to demo the LLM judge with real Gemini/Groq output.

Usage:
    python demo/run_demo_remote.py
    python demo/run_demo_remote.py --api http://localhost:8000
"""

import argparse
import asyncio
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent))

if sys.platform == "win32":
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except Exception:
        pass

import httpx
from rich.console import Console
from rich.panel import Panel
from rich.table import Table
from rich import box

DEFAULT_API = "https://stale-detector-api.onrender.com"
console = Console(safe_box=True)

VERDICT_STYLE = {
    "STALE": ("bold red", "STALE"),
    "PARTIALLY_STALE": ("bold yellow", "PARTIAL"),
    "CURRENT": ("bold green", "CURRENT"),
    "UNVERIFIABLE": ("bold blue", "UNVERIFIED"),
}


async def audit(client: httpx.AsyncClient, api: str, sample: dict) -> dict:
    resp = await client.post(
        f"{api}/v1/check",
        json={"question": sample.get("question"), "answer": sample["answer"]},
    )
    resp.raise_for_status()
    return resp.json()


async def run(api: str, only: int | None) -> int:
    samples = json.loads(
        (Path(__file__).parent / "sample_answers.json").read_text(encoding="utf-8")
    )
    if only:
        samples = samples[:only]

    try:
        async with httpx.AsyncClient(timeout=90) as probe:
            health = (await probe.get(f"{api}/v1/health")).json()
    except Exception as err:
        console.print(f"[bold red]Service unreachable at {api}[/bold red]: {err}")
        return 1

    console.print(
        Panel.fit(
            "[bold white on blue] STALE-ANSWER DETECTOR — LIVE DEMO [/bold white on blue]\n"
            f"[cyan]{len(samples)} realistic stale answers audited through {api}[/cyan]\n"
            f"[cyan]verification mode: {health.get('verification_mode')} | "
            f"llm: {health.get('llm_configured')} | tavily: {health.get('tavily_configured')}[/cyan]",
            border_style="cyan",
            box=box.DOUBLE,
        )
    )

    async with httpx.AsyncClient(timeout=120) as client:
        for i, sample in enumerate(samples, start=1):
            console.print(f"\n[bold yellow]=== Case #{i}: {sample['topic']} ===[/bold yellow]")
            console.print(f"[bold cyan]User Query:[/bold cyan] {sample.get('question', '')}")
            console.print(
                Panel(
                    f"[italic white]{sample['answer']}[/italic white]",
                    title="[bold red]Original LLM Answer (Potentially Stale)[/bold red]",
                    border_style="red",
                    box=box.ROUNDED,
                )
            )

            try:
                report = await audit(client, api, sample)
            except Exception as err:
                console.print(f"[bold red]Audit failed:[/bold red] {err}")
                continue

            score = report["overall_freshness_score"]
            color = "green" if score >= 80 else "yellow" if score >= 50 else "red"
            summary = report["summary"]
            console.print(
                f"[bold]Overall Freshness Score:[/bold] [{color}]{score:.1f} / 100[/{color}] | "
                f"[bold]Claims Audited:[/bold] {summary['total_claims']} "
                f"([red]{summary['stale']} Stale[/red], [green]{summary['current']} Current[/green], "
                f"[yellow]{summary['partially_stale']} Partial[/yellow], "
                f"[blue]{summary['unverifiable']} Unverifiable[/blue], "
                f"[dim]{summary['skipped']} Skipped[/dim])"
            )

            table = Table(
                title=f"Claims & Freshness Audit ({sample['topic']})",
                box=box.SIMPLE_HEAVY,
                header_style="bold magenta",
            )
            table.add_column("Claim", style="dim", width=6)
            table.add_column("Type", width=15)
            table.add_column("Claim statement", width=36)
            table.add_column("Risk", width=6)
            table.add_column("Verdict", width=11)
            table.add_column("Fresh", width=6)
            table.add_column("Live ground truth", width=32)

            for c in report["claims"]:
                if c["skipped"]:
                    v = "[dim]SKIPPED[/dim]"
                elif c["verdict"] in VERDICT_STYLE:
                    style, label = VERDICT_STYLE[c["verdict"]]
                    v = f"[{style}]{label}[/{style}]"
                else:
                    v = "[dim]UNVERIFIED[/dim]"
                truth = "[dim]not checked[/dim]" if c["skipped"] else (c["current_value"] or "-")
                table.add_row(
                    c["id"],
                    c["claim_type"],
                    c["text"][:58] + ("..." if len(c["text"]) > 58 else ""),
                    f"{c['staleness_risk'] * 100:.0f}%",
                    v,
                    f"{c['freshness_score']:.1f}" if c["freshness_score"] is not None else "-",
                    truth[:50] + ("..." if len(truth) > 50 else ""),
                )
            console.print(table)

            if report.get("changes"):
                console.print(
                    Panel(
                        report["corrected_answer"],
                        title="[bold green]Corrected Answer (with live citations)[/bold green]",
                        border_style="green",
                        box=box.ROUNDED,
                    )
                )

            usage = report["usage"]
            console.print(
                f"[dim]Tavily calls: {usage['tavily_calls']} | cached: {usage['tavily_calls_saved']} | "
                f"LLM calls: {usage['llm_calls']} | latency: {usage['latency_ms'] / 1000:.1f}s | "
                f"mode: {report['verification_mode']}[/dim]\n"
            )

    console.print(
        Panel.fit(
            "[bold green]LIVE DEMO COMPLETE[/bold green]\n"
            "[white]Every verdict above came from a live Tavily search and the deployed LLM judge.[/white]",
            border_style="green",
            box=box.DOUBLE,
        )
    )
    return 0


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Demo the sample answers through a running service")
    parser.add_argument("--api", default=DEFAULT_API, help="Base URL of the running service")
    parser.add_argument("--only", type=int, default=None, help="Only the first N samples")
    args = parser.parse_args()
    sys.exit(asyncio.run(run(args.api.rstrip("/"), args.only)))
