"""Check that a Stale-Answer Detector report still has every field the console reads.

The console's recorded report (frontend/src/tests/fixtures/stale.report.json)
lists the fields it uses. This loads it into the backend's own CheckReport
model and dumps it again: if the backend renamed or dropped a field, the dump
no longer has it and this fails. Producing a live report needs Tavily credits,
so CI checks the model instead of a live call.

Usage: python .github/scripts/stale_contract.py
"""

import json
import sys
from pathlib import Path

root = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(root / "backend" / "stale"))

from app.schemas import CheckReport  # noqa: E402

fixture = json.loads((root / "frontend/src/tests/fixtures/stale.report.json").read_text(encoding="utf-8"))
dumped = CheckReport(**fixture).model_dump(mode="json")

missing: list[str] = []


def compare(expected, actual, path: str) -> None:
    if isinstance(expected, dict):
        if not isinstance(actual, dict):
            missing.append(f"{path} (no longer an object)")
            return
        for key, value in expected.items():
            if key not in actual:
                missing.append(f"{path}.{key}")
            else:
                compare(value, actual[key], f"{path}.{key}")
    elif isinstance(expected, list) and expected:
        if not isinstance(actual, list) or not actual:
            missing.append(f"{path} (no longer a list, or empty)")
            return
        compare(expected[0], actual[0], f"{path}[0]")


compare(fixture, dumped, "report")
if missing:
    print("::error::The Stale API's CheckReport lost fields the console reads:")
    for m in missing:
        print("  -", m)
    sys.exit(1)
print("CheckReport still has every field the console reads.")
