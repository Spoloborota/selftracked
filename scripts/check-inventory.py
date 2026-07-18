#!/usr/bin/env python3
"""Total-accounting check for the traceability inventory.

Enforces the machine-checkable half of the execution plan's §3 rule 2:
every inventory row is owned by exactly one stage the plan declares, or is
an explicit §17 deferral; every stage owns at least one row; nothing claims
to be verified without evidence.

What this check CANNOT do (stated so nobody mistakes a green run for more
than it is): it does not judge whether a stage assignment is *buildable* at
that point in the dependency order, it does not read the spec to confirm an
anchor's content, and it cannot tell a good fixture from a bad one. Those
are the completeness and stage-assignment review passes (plan §5).

Usage:  python3 scripts/check-inventory.py
Exit:   0 = clean, 1 = accounting defects found, 2 = usage/IO error.

Ships with the inventory at G0; wired into CI at S0 (plan §4).
"""
from __future__ import annotations

import re
import sys
from collections import Counter, defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
INVENTORY = ROOT / "docs" / "v0-traceability-inventory.md"
PLAN = ROOT / "docs" / "v0-execution-plan.md"
SPEC = ROOT / "docs" / "v0-spec.md"

# Buckets the plan authorises for a row that no stage closes.
AUTHORISED_NON_STAGE = {"deferred"}
VALID_STATUS = {"planned", "verified-by-command"}
VALID_KINDS = {
    "schema-gate", "verb-contract", "catalog-convention", "verify-rule",
    "format", "process", "deliverable", "stated-limitation",
    "deferral-boundary",
}
COLUMNS = ["id", "anchors", "statement", "kind", "stage", "verification",
           "status", "evidence"]


def read_rows(path: Path) -> list[dict]:
    rows: list[dict] = []
    for lineno, line in enumerate(path.read_text().splitlines(), 1):
        if not line.startswith("| INV-"):
            continue
        parts = [p.strip() for p in re.split(r"(?<!\\)\|", line)][1:-1]
        if len(parts) != len(COLUMNS):
            rows.append({"_bad": f"line {lineno}: {len(parts)} columns, "
                                 f"expected {len(COLUMNS)}"})
            continue
        row = dict(zip(COLUMNS, parts))
        row["_line"] = lineno
        rows.append(row)
    return rows


def declared_stages(path: Path) -> set[str]:
    """Stage ids the plan's §4 table declares (bolded first cell)."""
    return {m.group(1) for m in
            (re.match(r"\|\s*\*\*(S\d+[a-c]?)\*\*\s*\|", line)
             for line in path.read_text().splitlines()) if m}


def spec_sections(path: Path) -> set[str]:
    """Top-level section numbers the spec actually has, e.g. {'5', '8'}.

    Only top-level numbers are validated: the spec's subsections (§5.7, §8.4)
    live inside SQL comments and prose, not in markdown headings, so a
    heading scan cannot confirm them. Anchors are checked against the section
    that must exist; confirming a subsection's content is a review job.
    """
    out: set[str] = set()
    for line in path.read_text().splitlines():
        m = re.match(r"#{1,3}\s+(\d+)\.?\s", line)
        if m:
            out.add(m.group(1))
    return out


MIN_TOKENS_FOR_DUP = 5


def normalise(statement: str) -> str:
    """Token signature for duplicate detection.

    Backticked identifiers are KEPT — they are usually the only thing that
    distinguishes two short rows (`Deferral: stats` vs `Deferral: redact`),
    and stripping them made distinct rows collide.
    """
    s = statement.lower().replace("`", " ")
    s = re.sub(r"[^a-z0-9 ]", " ", s)
    tokens = sorted(w for w in s.split() if len(w) > 3)
    if len(tokens) < MIN_TOKENS_FOR_DUP:
        return ""  # too short to compare safely; not a duplicate signal
    return " ".join(tokens)


def main() -> int:
    for p in (INVENTORY, PLAN, SPEC):
        if not p.exists():
            print(f"error: {p} not found", file=sys.stderr)
            return 2

    rows = read_rows(INVENTORY)
    stages = declared_stages(PLAN)
    sections = spec_sections(SPEC)
    if not stages:
        print("error: no stages parsed from the plan's §4 table",
              file=sys.stderr)
        return 2

    defects: list[str] = []
    seen_ids: set[str] = set()
    owned = {s: 0 for s in stages}
    unauthorised: Counter[str] = Counter()
    by_statement: dict[str, list[str]] = defaultdict(list)
    review_only = 0

    for r in rows:
        if "_bad" in r:
            defects.append(f"malformed row ({r['_bad']})")
            continue
        rid = r["id"]
        if rid in seen_ids:
            defects.append(f"{rid}: duplicate row id")
        seen_ids.add(rid)

        stage = r["stage"]
        if stage in stages:
            owned[stage] += 1
        elif stage in AUTHORISED_NON_STAGE:
            if "§17" not in r["anchors"]:
                defects.append(
                    f"{rid}: deferred rows must anchor their deferral in §17 "
                    f"(anchors: {r['anchors']})")
        else:
            unauthorised[stage] += 1
            defects.append(
                f"{rid}: bucket '{stage}' is not a stage declared in the "
                f"plan's §4 table and is not an accounting bucket the plan "
                f"authorises — it needs an accepted amendment, not a "
                f"tolerant gate")

        for part in re.split(r"\s*\+\s*", r["kind"]):
            if part not in VALID_KINDS:
                defects.append(f"{rid}: unknown kind component '{part}'")

        if not r["verification"]:
            defects.append(f"{rid}: empty verification column")
        elif r["verification"].startswith("review:"):
            review_only += 1

        if r["status"] not in VALID_STATUS:
            defects.append(f"{rid}: unknown status '{r['status']}'")

        if r["status"] == "verified-by-command" and not r["evidence"]:
            defects.append(
                f"{rid}: claims verified-by-command with no evidence link")

        for anchor in re.findall(r"§(\d+)", r["anchors"]):
            if anchor not in sections:
                defects.append(
                    f"{rid}: anchor §{anchor} names a section the spec "
                    f"does not have")

        by_statement[normalise(r["statement"])].append(rid)

    for sig, ids in by_statement.items():
        if sig and len(ids) > 1:
            defects.append(
                f"rows {', '.join(ids)} restate the same obligation — one "
                f"obligation is one row carrying every anchor")

    for stage, n in sorted(owned.items()):
        if n == 0:
            defects.append(
                f"stage {stage} is declared in the plan but owns no inventory "
                f"row — either the stage is empty or the walk missed its "
                f"obligations")

    real_rows = [r for r in rows if "_bad" not in r]
    print(f"inventory rows: {len(real_rows)}")
    print(f"stages declared in plan: {len(stages)}")
    print("rows per stage: " + ", ".join(f"{s}={owned[s]}"
                                         for s in sorted(owned)))
    print(f"review-only obligations (no fixture possible): {review_only}")
    if unauthorised:
        print("rows in unauthorised buckets: " + ", ".join(
            f"{b}={n}" for b, n in sorted(unauthorised.items())))

    if defects:
        print(f"\nACCOUNTING DEFECTS ({len(defects)}):")
        for d in defects:
            print(f"  - {d}")
        return 1

    print("\naccounting clean: every row owned, every stage covered")
    return 0


if __name__ == "__main__":
    sys.exit(main())
