# selftracked v0 — progress ledger

The bootstrap window's living state (execution plan §6). **Read this first in
every session; update it last.** Disposable by design: at S10 its contents
move into `.selftracked/` and this file is deleted.

Status values: `not started` · `in progress` · `blocked` · `done`.
A stage reaches `done` only with an evidence link — a CI run or a committed
log artifact proving the verification commands exited 0 (plan §5).

## Where things stand

Read this section first; the tables below carry the detail.

**Done:** G0 (the traceability inventory), S0 (repo bootstrap), S1a (the
schema as text). The schema exists as `internal/schema` — embedded DDL,
connection settings, `meta` seeding — with 12 commits of history, all local,
nothing pushed.

**Next:** S1b implementation — every gate the schema declares, with a red
fixture each. The stage is **open**: the D-EP13 opening record
(`docs/stage-openings/s1b.md`) holds the per-row verdicts and the resolved
commands. The open moved three rows to the stages that can execute them
(INV-018→S7, INV-097→S5b, INV-143→S5a), leaving S1b at 86; one row
(INV-016) is blocked on an owner ruling and blocks nothing else. Write the
fixtures red-first from the record's command list.

**How to verify anything:** `make gates` runs the whole chain. It must exit 0
before a stage closes, and a fresh reviewer re-runs it rather than trusting
the report.

**What is waiting on the owner:** three items from the S1b open, none
blocking the gates work — see "Open questions for the owner" below.

## Stages

| Stage | Tier | Status | Last verification (command · date · result · evidence) | Open questions |
|---|---|---|---|---|
| G0 — traceability inventory | FULL | done | `python3 scripts/check-inventory.py` · 2026-07-19 · **exit 0, accounting clean** (545 rows, all 16 stages covered, 16 review-only obligations) · evidence link pending (no CI yet — S0 wires it) | Ratified by the owner 2026-07-19 (D-EP4). Three review passes done; findings applied. Fidelity was sampled, not exhaustive — each stage re-reads its own rows at open (plan §5). |
| S0 — repo bootstrap | FULL | done (interim evidence) | `make gates` · 2026-07-19 · all green · local run @ `e09204a`, **no CI has run** (D-EP8) | 5 of 8 rows `verified-by-command`; INV-492/499/513 stay `planned` — they assert CI gates that cannot be proven before the first push |
| S1a — schema as text | FULL | done (interim evidence) | `make gates` · 2026-07-19 · all green · local run @ `6cf73a1`, no CI has run | All 10 rows `verified-by-command`. INV-053 closed once the owner ratified the spec amendment that made its claim true (spec rev 3.10) |
| S1b — schema gates | FULL | in progress | — | Opened 2026-07-19 per D-EP13 (`docs/stage-openings/s1b.md`). 86 rows after three placement moves; red fixture each; INV-016 blocked on owner ruling |
| S1c — driver behaviour | FULL | not started | — | 10 rows, behavioural probes only |
| S2 — CLI dispatcher | FULL | not started | — | — |
| S3 — serializer + `dump` | FULL | not started | — | — |
| S4 — `load` + parser fuzzing | FULL | not started | — | — |
| S5a — task-lifecycle verbs | FULL | not started | — | — |
| S5b — relation/artifact/dictionary verbs | FULL | not started | — | — |
| S6 — epic/story/worklog/criteria verbs | FULL | not started | — | — |
| S7 — `verify` | FULL | not started | — | — |
| S8a — `init` scaffold + generated docs | FULL | not started | — | — |
| S8b — hooks + sidecar matrix | FULL | not started | — | — |
| S8c — `state`, `prime`, SessionStart | FULL | not started | — | — |
| S9 — `import` | FULL | not started | — | — |
| S10 — dogfood switchover | FULL | not started | — | — |
| S11 — version gate + migration branches | FULL | not started | — | — |
| S12 — pilot ladder + remaining deliverables | FULL | not started | — | — |

## Amendments log

| Change | Targets | Status | Resulting revision |
|---|---|---|---|
| `plan-accounting-scope` | execution plan §3, §4, §5, §10 | accepted 2026-07-19 (D-EP4, D-EP5) | plan rev 5 |
| `stage-open-plan-crosscheck` | execution plan §5, §8, §10 | accepted 2026-07-19 (D-EP6) | plan rev 6 |
| `review-proportionality-tiers` | execution plan §5, §8, §10 | accepted 2026-07-19 (D-EP7) | plan rev 7 |
| `local-commits-and-interim-evidence` | execution plan §4 (S0), §8, §10; `.claude/CLAUDE.md` rule 6 | accepted 2026-07-19 (D-EP8) | plan rev 8 |
| `s0-minimal-package` | execution plan §4 (S0), §10 | accepted 2026-07-19, ratified by the owner (D-EP9) | plan rev 9 |
| `split-s1` | execution plan §4 (S1), §10 | accepted 2026-07-19 (D-EP10) | plan rev 10 |
| `nullable-columns-preamble` | **spec** §5 preamble + §8.1 | accepted 2026-07-19 — first amendment to the specification | spec rev 3.10 |
| `evidence-across-a-squash` | execution plan §5, §8, §9, §10 | accepted 2026-07-19 (D-EP11, D-EP12) | plan rev 11 |
| `import-date-bounds` | **spec** §6.2 `import` | accepted 2026-07-19 | spec rev 3.11 |
| `stage-open-record` | execution plan §5, §8, §10 | accepted 2026-07-19 (D-EP13) | plan rev 12 |
| `worklog-story-guard-rule-pointer` | **spec** §5.7 (one comment line) | accepted 2026-07-19 | spec rev 3.12 |

The first amendment came out of G0 itself: fidelity had been verified by
sampling rather than exhaustively, and one stage's definition of done (S10)
contains plan-native work no inventory row can carry. It adds a stage-open
row re-verification step and states the accounting rule's scope boundary.
Proposal and the quoted ratification: `openspec/changes/plan-accounting-scope/`.

The second amendment came from the owner reading a research document and
asking where its items had landed: three of spec §16's re-verification items
were filed on a stage that could not execute them — right about the
obligation, wrong about the place — and three review passes had not been
looking for that. The stage-open re-read now checks placement as well as
content. Five rows moved (S1 130→125, S3 30→31, S4 14→15, and two into S0); the
accounting stayed clean throughout, because it cannot see this class.

Two deviations were found at G0 and closed **without** an amendment, because
neither needed a rule change:

- Two rows describing the spec's own decision register were parked in an
  invented `spec-record` bucket. Their verification is a citation check, and
  S12 already owns `docs link-check` — so they were reassigned to S12 and the
  bucket disappeared. The accounting check had been accepting the
  unauthorised bucket; it now rejects any bucket the plan does not declare.
- The first cut of the inventory carried an extra `src` (extraction
  provenance) column the plan's §3 format does not define. Rather than ship a
  silent format superset, the column was removed from the published document;
  provenance is retained in the private assembly artifact.

The S0 close review found six defects, four of them in the stage's own
gates. Two scripts failed open — `probe-gofix.sh` reported success when `go`
was absent from PATH, having mistaken a shell "command not found" for a
pending fix, and `check-pins.sh` skipped its import-graph check in the same
condition while still printing "policy satisfied". Both now refuse to report
success when they cannot run. `check-pins.sh` also only checked that a libc
version was *present*; it now resolves the version the driver's own go.mod
names and compares. Three rows (the driver pin, the driver/libc pair, the
licensing deliverable) moved to S1, where the driver actually arrives — at
S0 their checks passed vacuously, the same defect class D-EP6 exists for.
And the stage had not been closed by the plan's own procedure at all: the
inventory statuses and this ledger were untouched until the review said so.

S1a's close review found the connection settings contradicting §3.1 rather
than lagging it — the DSN set the one journal mode the specification rules
out — and proved the DDL tests could not fail, by deleting a trigger and
watching the suite stay green. Both are fixed and the mutation now fails.
The review also showed twenty of the stage's twenty-nine rows needed verbs,
events or a serializer that this stage does not build; they moved to the
stages that first have the machinery, leaving S1a with ten rows it can
actually execute. The stage-open placement check had not been run properly
on all twenty-nine, which is how they survived to the close.

## Parked — out of scope, no decision needed yet

Work found mid-stage and deliberately not done (`.claude/CLAUDE.md`, scope
discipline). Nothing here blocks anything; it exists so it is not rediscovered.

| Item | Found during | Note |
|---|---|---|
| Plan §2.1 describes an OpenSpec change as "proposal + delta" with a `tasks.md` pointer; the three change directories hold only `proposal.md` | first-commit review | The tool is adopted but not installed, so the convention has nothing to run against yet. Revisit when it is. |

## Open questions for the owner

Raised by the S1b open, 2026-07-19. None blocks the gates implementation.

1. **INV-016 — "an epic decomposes into ≥2 stories".** The cardinality is
   enforced nowhere: no CHECK, `epic close` (§6.4) never counts stories, no
   R-rule does either. Rule it definitional (the row narrows to its
   schema-carried parts) or add enforcement (a spec amendment naming the
   mechanism — e.g. an `epic close` blocker). The row waits at S1b either way.
2. **Amendment `worklog-story-guard-rule-pointer`** — §5.7 says the FK-free
   `worklog.story` column is "guarded by R5"; the guarding rule is R4
   (§7). One comment line; proposal filed, spec untouched until ratified.
3. **Anchor line numbers drift on every spec amendment.** Inventory anchors
   carry rev-3.9 line numbers; 3.10/3.11 shifted the file, so the numbers
   point a few lines off while section names stay right. Re-stamp after
   every amendment, or strip the numbers and anchor by section only?

Earlier rounds: all answered 2026-07-19 — the `as of dump <sha12>` anchor
keeps its deferred validator (D-EP12); D-EP9 ratified; the §5
nullable-column wording corrected in the specification itself; the
English-only convention covers quoted decisions explicitly
(`.claude/CLAUDE.md`).
