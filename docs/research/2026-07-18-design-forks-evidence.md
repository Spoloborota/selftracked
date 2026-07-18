# Design-fork evidence: dump format, tracked DB, paths, alias, MCP, events, kinds, ratchets

Status: EXECUTED → v0-spec.md §15 (verdicts adopted in the spec's decision
log). Method: web research over primary sources plus local git experiments
(three-way merges on synthetic dumps; transcripts preserved in the local
research archive, not committed). Third-party figures are observations, not
measurements made in this repo.

## D1 — Path-dictionary roots: repo-relative vs absolute → REPO-RELATIVE

- Absolute paths in committed state die on every fresh clone, CI runner and
  second machine; portable-paths guidance is uniform across ecosystems.
- Tool-regret precedent: Shotcut stored absolute paths for in-project
  resources and had to add relative-path support after portability bug
  reports (mltframework/shotcut#710).
- The one legitimate absolute-path need (references outside the repo) is
  solved by mature tools as an explicit, named indirection — DVC's external
  dependencies use remote aliases, never absolute paths in ordinary state.
  The path dictionary can add an `external` class later without redesign.

## D2 — Dump format: canonical SQL vs JSONL-per-table → CANONICAL SQL

Empirical (three-way git merges on synthetic states, two branches):

| Scenario | SQL dump | JSONL per table |
|---|---|---|
| Both branches create "next task" (same id) | CONFLICT (one hunk swallowed the adjacent events section) | CONFLICT (two small per-file hunks) |
| One edits a row, other appends | AUTO-MERGED | AUTO-MERGED |
| Edits to different rows | AUTO-MERGED | AUTO-MERGED |
| Append-only file, both append | — | CONFLICT (plain) / AUTO-MERGED with `merge=union` **but produced a duplicate seq — silent corruption** |

Conclusion: merge behavior is format-neutral; only the single-writer axiom
prevents conflicts. Precedent: classic beads needed a custom git merge driver
for its JSONL which "silently dropped fields" during merges
(steveyegge/beads#1481) — multi-writer pain no format fixes. SQL wins on:
carried DDL (self-contained), direct load, `textconv`/sqldiff ecosystem.
JSONL's one real advantage: smaller conflict blast radius (per-table files).

## D3 — Short binary alias → `st` and `stk` DISQUALIFIED; owner picks among sdt / strk / none

- `st`: Homebrew formula `st` exists and itself declares a conflict with
  another `st` binary; Debian has a documented file conflict around
  suckless st (bug #629998); npm `st` ships a bin. Three ecosystems already
  fight over the name.
- `stk`: the name is owned in Homebrew and Debian by the Sound Synthesis
  Toolkit — the formula/package name is not obtainable for distribution.
- `sdt`: clean everywhere checked except a dead 2014 npm package. `strk`:
  clean, but 4 awkward chars. `selftracked` itself is clean everywhere.

## D4 — Track db.sqlite in git alongside the dump? → DUMP-ONLY

- Decisive: SQLite file bytes are not deterministic for identical logical
  content (page/freelist layout; even vacuumed twins can differ) — a tracked
  binary cannot satisfy a byte-equality pair gate and produces phantom churn.
- Binary merge is ours/theirs only (verified locally: git offers no 3-way).
- Bloat is NOT the argument: 61 commits of a realistic DB showed pack sizes
  168KB (binary) vs 180KB (text) — a wash.
- Precedent: beads in every architecture gitignores the DB and tracks only
  the text serialization.

## D5 — MCP server timing → v0.1+, thin wrapper over stable verbs

- All comparables (beads_rust, Backlog.md, PlanDB) ship MCP as an optional
  layer over the CLI/store; none makes it the core.
- The MCP spec had breaking revisions 2024-11 → 2025-03 → 2025-06 → 2025-11,
  and the 2026-07-28 release candidate is again explicitly breaking — a v0
  server would already need rework.
- Published 2026 measurements report CLI+skill interfaces cost far fewer
  tokens than MCP tool schemas for agent use (multiple independent posts).

## D7 — Events/audit inside the dump vs separate append-only file → IN-DUMP for v0

- The main argument for a separate file — "appends always merge cleanly" —
  is empirically false (see D2 table), and `merge=union` corrupts monotonic
  seq silently (git's own docs warn against union without understanding).
- Under single-writer no merges occur at all; a second tracked file would
  double the coherence surface the pair gate must guard.
- The real costs of in-dump events (review noise, unbounded growth) are
  addressed later by an `events archive` verb; the canonical CHANGELOG-in-git
  conflict crisis (GitLab's, solved by file-per-entry) is a multi-writer
  problem selftracked excludes by axiom.

## D8 — First-class 'question' task kind vs IN-REVIEW status convention → CONVENTION

Uniform precedent: GitHub models "question" as a label (its 2025 first-class
issue types are task/bug/feature only); Jira has it only as a custom type;
Linear has no types at all — triage is a queue/state; agent-facing trackers
(beads family, Backlog.md) have no question kind. A question is a lifecycle
state ("awaiting input"), not a species of work item; a kind would give the
same fact a second home.

## D10 — Tech-debt/findings register with graduation thresholds in v0 schema? → DEFER

The pattern is validated in the wild as a *quality ratchet* — Notion's
committed ratchet file with per-(file,rule) allowed-violation counts,
Betterer, DebtRatchet — but in every precedent it is a separate CI-layer
artifact, never tables inside a tracker's schema. The data shape is small and
purely additive later; deferral costs nothing structurally.
