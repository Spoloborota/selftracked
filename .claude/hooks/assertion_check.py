#!/usr/bin/env python3
"""Stop hook: flag factual claims about the repo that no command backed.

WHAT THIS CATCHES: a reply asserting a number, a `file:line`, or an on-disk
existence claim in a turn where NOT ONE evidence-producing tool ran. That is
an assertion from memory. It also catches an event date later than the wall
clock, which is provably wrong regardless of what ran.

WHAT THIS CANNOT CATCH, stated rather than hidden:

- A claim verified with the WRONG query — the tool ran, the answer was
  garbage. Only a review catches those.
- A claim in a turn where some *unrelated* command happened to run: any
  evidence tool in the turn suppresses the whole turn's numeric check.
- A number that is not about this repository. There is no domain gate, so
  "the default port is 8080" is flagged like a row count; the cost of a
  domain gate is missing real claims, and nag beats silence here.
- Any phrasing outside the shapes below — the verb list and the extension
  list are enumerations, not a grammar.

A quiet hook is not evidence that the numbers are right.

Design: the detector keys on the SHAPE of the claim (a number, a path with a
line, an existence verb) plus an allowlist of identifiers that are references
rather than measurements. Keying on trigger WORDS was rejected: the sentences
this project actually got wrong shared no vocabulary, only shape.

A detector that does not pass its own corpus of real failures has no right to
be trusted — the corpus is `tests/test_assertion_check.py`, drawn from claims
this project actually published unverified.

Never blocks. Exit 0 = silent. Exit 2 + stderr = the note reaches the agent.
"""
from __future__ import annotations

import datetime
import json
import re
import sys

EVIDENCE_TOOLS = {"Bash", "Read", "Grep", "Glob", "NotebookRead"}

RE_FILE_LINE = re.compile(
    r"\b[\w./-]+\.(?:go|py|md|sql|sh|json|toml|yaml|yml|txt|rs|ts|js|c|h|rb|java)"
    r"\s*:\s*\d+"
)
RE_BIG_NUM = re.compile(r"(?<![\w.:/-])\d{3,}(?![\w.:/%-])")
RE_COUNTED = re.compile(
    r"\b(?:all|every|both|each)?\s*(?:\d{1,3}|one|two|three|four|five|six|seven|"
    r"eight|nine|ten|eleven|twelve)\s+"
    r"(?:rows?|files?|lines?|findings?|stages?|commits?|obligations?|"
    r"entries|items?|defects?|checks?|documents?|docs?|tests?|bugs?|"
    r"rules?|verbs?|sections?|columns?)\b",
    re.I,
)
RE_EXISTENCE = re.compile(
    r"\b(?:there (?:is|are)|exists?|is present|are present|is missing|"
    r"does not exist|no such file|contains?|lives? (?:in|at|under)|"
    r"sits? (?:in|at|under)|find (?:it|them)? ?(?:in|at|under)|"
    r"located (?:at|in|under))\b",
    re.I,
)
RE_SENTENCE = re.compile(r"(?<=[.!?])\s+|\n+")
RE_ISO_DATE = re.compile(r"(?<![\w/-])(\d{4})-(\d{2})-(\d{2})(?![\w-])")
RE_MARKED = re.compile(r"[✓?]")

# Identifiers that look numeric but are references, not measurements.
ALLOWED = [
    re.compile(r"\bINV-\d+\b"),                 # inventory row ids
    re.compile(r"\bD-EP\d+\b"),                 # plan decisions
    re.compile(r"\bD\d+\b"),                    # spec decisions
    re.compile(r"\bR\d+\b"),                    # verify rules
    re.compile(r"\bS\d+[a-c]?\b"),              # stage ids
    re.compile(r"§\s*\d+(?:\.\d+)?"),           # section anchors
    re.compile(r"\bv?\d+\.\d+(?:\.\d+)?\b"),    # versions
    re.compile(r"\b(?:19|20)\d{2}\b"),          # bare years
    re.compile(r"\b\d{4}-\d{2}-\d{2}\b"),       # ISO dates (future ones below)
    re.compile(r"\b[0-9a-f]{7,40}\b"),          # git object ids
    re.compile(r"#\d+\b"),                      # task refs
    re.compile(r"\bexit (?:code )?\d\b", re.I),  # exit codes
]


def strip_allowed(text: str) -> str:
    for pattern in ALLOWED:
        text = pattern.sub(" ", text)
    return text


def future_dates(text: str, today: datetime.date) -> list[str]:
    found = []
    for match in RE_ISO_DATE.finditer(text):
        # A date embedded in a path or filename is a citation, not a claim.
        start, end = match.span()
        around = text[max(0, start - 1):min(len(text), end + 1)]
        if "/" in around or "\\" in around:
            continue
        try:
            when = datetime.date(*(int(g) for g in match.groups()))
        except ValueError:
            continue
        if when > today:
            found.append(match.group(0))
    return found


def risky_claims(text: str) -> list[str]:
    """Flag claims sentence by sentence.

    Per-sentence, not per-reply: a provenance mark suppresses only the
    sentence carrying it. Whole-reply suppression was the first version and
    was defeated by any question mark anywhere — an agent that ends with
    "shall I proceed?" would silence every claim above it.
    """
    hits = []
    for sentence in RE_SENTENCE.split(text):
        if RE_MARKED.search(sentence):
            continue  # this claim is marked ✓ verified or ? unverified
        stripped = strip_allowed(sentence)
        for pattern, label in (
            (RE_FILE_LINE, "a file:line citation"),
            (RE_COUNTED, "a counted quantity"),
            (RE_BIG_NUM, "a bare number"),
            (RE_EXISTENCE, "an on-disk existence claim"),
        ):
            match = pattern.search(stripped)
            if match:
                hits.append(f"{label} ({match.group(0).strip()!r})")
                break  # one note per sentence is enough
    return hits[:4]


def analyse(reply: str, tools_used, today: datetime.date) -> str | None:
    notes = []

    stale = future_dates(reply, today)
    if stale:
        notes.append(
            f"event date(s) later than the wall clock ({today}): "
            f"{', '.join(stale)} — a future date cannot be an event date; "
            f"take it from `date`, an mtime, or `git log`"
        )

    if not set(tools_used) & EVIDENCE_TOOLS:
        claims = risky_claims(reply)
        if claims:
            notes.append(
                "this reply asserts " + "; ".join(claims) +
                " but no evidence-producing tool ran in this turn — run the "
                "command and mark it ✓, or mark the claim ? as unverified"
            )

    if not notes:
        return None
    return "assertion provenance (.claude/CLAUDE.md):\n  - " + "\n  - ".join(notes)


def last_turn(transcript_path: str) -> tuple[str, list[str]]:
    """Return (assistant text, tool names) for the turn that just ended."""
    try:
        with open(transcript_path, encoding="utf-8") as fh:
            lines = fh.readlines()
    except OSError:
        return "", []

    texts: list[str] = []
    tools: list[str] = []
    for raw in reversed(lines):
        try:
            entry = json.loads(raw)
        except (json.JSONDecodeError, ValueError):
            continue
        kind = entry.get("type")
        content = (entry.get("message") or {}).get("content")
        if not isinstance(content, list):
            content = []
        if kind == "user":
            # A tool result is also a "user" entry; a real user message ends
            # the turn we are inspecting.
            if any(b.get("type") == "tool_result" for b in content if isinstance(b, dict)):
                continue
            break
        if kind == "assistant":
            for block in content:
                if not isinstance(block, dict):
                    continue
                if block.get("type") == "text":
                    texts.append(block.get("text", ""))
                elif block.get("type") == "tool_use":
                    tools.append(block.get("name", ""))
    return "\n".join(reversed(texts)), tools


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        return 0
    path = payload.get("transcript_path")
    if not path:
        return 0

    reply, tools = last_turn(path)
    if not reply.strip():
        return 0

    note = analyse(reply, tools, datetime.date.today())
    if note:
        print(note, file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
