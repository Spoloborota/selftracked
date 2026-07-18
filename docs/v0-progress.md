# selftracked v0 — progress ledger

The bootstrap window's living state (execution plan §6). **Read this first in
every session; update it last.** Disposable by design: at S10 its contents
move into `.selftracked/` and this file is deleted.

Status values: `not started` · `in progress` · `blocked` · `done`.
A stage reaches `done` only with an evidence link — a CI run or a committed
log artifact proving the verification commands exited 0 (plan §5).

## Stages

| Stage | Tier | Status | Last verification (command · date · result · evidence) | Open questions |
|---|---|---|---|---|
| G0 — traceability inventory | FULL | done | `python3 scripts/check-inventory.py` · 2026-07-19 · **exit 0, accounting clean** (545 rows, all 16 stages covered, 16 review-only obligations) · evidence link pending (no CI yet — S0 wires it) | Ratified by the owner 2026-07-19 (D-EP4). Three review passes done; findings applied. Fidelity was sampled, not exhaustive — each stage re-reads its own rows at open (plan §5). |
| S0 — repo bootstrap | FULL | not started | — | — |
| S1 — schema package | FULL | not started | — | — |
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
content. Five rows moved (S0 9→12, S1 130→125, S3 30→31, S4 14→15); the
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

## Parked — out of scope, no decision needed yet

Work found mid-stage and deliberately not done (`.claude/CLAUDE.md`, scope
discipline). Nothing here blocks anything; it exists so it is not rediscovered.

| Item | Found during | Note |
|---|---|---|
| Plan §2.1 describes an OpenSpec change as "proposal + delta" with a `tasks.md` pointer; the three change directories hold only `proposal.md` | first-commit review | The tool is adopted but not installed, so the convention has nothing to run against yet. Revisit when it is. |

## Open questions for the owner

1. Task-level narrative dates sit outside the git-first import dating rule
   (task rows carry no commit citation) — accept as a stated v0 limitation,
   or extend the mechanism?
2. The `as of dump <sha12>` anchor ships as a convention in v0 with its
   validator deferred to the doc-lint core (spec §17) — accept, or pull the
   gate into v0?
3. Whether the English-only convention should name quoted owner decisions
   explicitly: two amendment proposals had a verbatim non-English quote
   pasted into them and removed on review. Parked here rather than fixed,
   since it changes a stated convention.
4. Approval to commit the current tree (spec, plan, inventory, ledger,
   research documents, `.claude/`, `scripts/`, the amendment directory).
