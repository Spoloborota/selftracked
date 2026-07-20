# S8c opening record

Stage: S8c — `state`, `prime`, the SessionStart chain, and R1 check 3 / R14
(STATE.md byte-equals its render) landing with the deterministic renderer
(plan §4). Opened 2026-07-20, per D-EP13. Spec revision at open: 3.17. Plan
revision at open: 15 → 16 (this open files the amendment below). Rows owned
at open: 30 → 29 after the placement correction below.

## Scope: the reader half of the tracker

Everything through S8b writes state and defends it; S8c is where an agent
*reads* it. Four deliverables, all on top of machinery that already exists —

- **`state` verb.** Regenerates `STATE.md` from the DB via the deterministic
  renderer built at S8a (`internal/state.Render`). Read-only against tracker
  state: it opens the DB read-only, renders, and writes the one tracked file.
  No DB mutation, so it does not run the write pipeline — it is `Read` + a
  file write (INV-274).
- **Wire the `stateRender` stub.** `internal/verb/pipeline.go` carries
  `var stateRender = func(...) error { return nil }` (line 44) as the
  post-commit tail's STATE.md slot, called by `regenerateDerived` between the
  dump write and the sidecar (§6.1 order). S8c points it at `state.Render` so
  every write verb refreshes STATE.md. The wiring lives in the `verb` package
  (the stub's home); `internal/state` stays a pure DB→bytes function with no
  file I/O, so `state` the verb and the pipeline share one renderer and R14
  asserts against the same bytes.
- **`prime` verb.** The §11.1 stable JSON contract, read-only. The large
  deliverable: `epics_active[]`, `epics_paused[]`, `epics_backlog[]`,
  `ready[]`, `triage[]`, `in_review[]`, `stale[]`, `sprint_goals[]`,
  `totals{}`, `dump_divergence`, `dump_requires_newer_binary`, with `prime_cap`
  capping the backlog-type lists and the two uncapped lists
  (`sprint_goals[]`, `epics_active[]`) left whole (INV-273, 458–470).
- **R1 check 3 / R14.** STATE.md byte-equals its render, folded into R1
  (INV-274/275/293). The renderer exists as of this stage, so the check that
  reuses it lands here — the reason the S7 open moved R14/R1-check-3 to S8c
  (amendment `r14-rides-its-renderer-at-s8c`, already filed).

The SessionStart hook JSON already ships in the scaffold (`init` writes
`.claude/settings.json`, S8a); S8c does not add the hook text — it makes
`prime --json` real so the hook's three branches (healthy `prime`; `load`
then `prime`; static error) function, and adds the fixture that drives them
(INV-454–457).

## Placement correction (one amendment, filed at this open)

| Row | To | Why |
|---|---|---|
| INV-464 (`prime` field `migrated`, present only when this invocation migrated) | S11 | The field's contract slot is §11.1, but its verification — "trigger a migrating `prime` invocation, assert `migrated` present with vK→vN" — cannot execute at S8c: the migration engine (§8.6: versioned DDL, transform chain, escalating read verb) is built at S11 (INV-379–389), and nothing at S8c migrates, so `prime` can never emit the field to assert on. INV-387 (§8.6, the sourcing twin that says `prime` reports `migrated:vK→vN`) already lives at S11; INV-464 homes beside it. Amendment `migrated-field-rides-migration-at-s11`. Same shape as `r14-rides-its-renderer-at-s8c` and `prime-divergence-rides-prime-at-s8c`: an obligation whose verification needs machinery a later stage builds rides to that stage. The contract *slot* is still built here — `prime`'s struct carries the `omitempty` field so S11 only populates it — but the row that proves the value closes at S11. |

Distribution updated (S8c 30 → 29, S11 25 → 26); plan §4 S8c/S11 DoD
amended; plan rev 15 → 16. **Spec unchanged** (`migrated` stays in the §11.1
contract; only the inventory row's owning stage moves — the same plan+inventory
shape as `prime-divergence-rides-prime-at-s8c`, which left the spec text
intact). `check-inventory` must exit 0 after the move.

Not moved, and why they stay: **INV-463** (`dump_requires_newer_binary`) stays
S8c though its §8.6 sourcing twin INV-391 sits at S11 — unlike `migrated`, this
field is fully buildable and testable here: `prime` reads the tracked dump's
header `schema_version` and compares it to the binary's `schema.Version`, no
migration engine required. Its verification ("force a newer-than-binary dump
header on a readable DB, assert `prime` sets the field true") runs at S8c by
hand-writing a dump header with `schema_version` 2. INV-391 then closes at S11
under the full version-gate integration (the forward-only *refusal* for other
verbs). Two distinct obligations on one field at two integration depths — no
conflict.

## Watch-item carried from S8b, researched to dissolution

The S8b close parked a watch-item: "`load`'s STATE.md refresh will depend on
whether a gate-skip marker happened to be pending; S8c must render STATE.md on
`load` regardless, or the coupling becomes observable." Researched at this
open against the load path (`internal/load/verb.go`) and the pipeline
(`regenerateDerived`), the fork **dissolves — `load` must NOT unconditionally
regenerate STATE.md**, and here is why:

- On a fresh clone (no DB, no marker — the marker is gitignored, so a clone
  never carries one), the git-tracked `STATE.md` was committed consistent with
  the git-tracked `dump.sql`. `load` rebuilds the DB from that dump, so
  `Render(DB)` byte-equals the tracked `STATE.md` by construction. No
  regeneration is needed and none happens; nothing is inconsistent.
- The marker path (`ConvertSkipMarker` → `regenerateDerived`) regenerates
  because it legitimately *adds* an event (the converted `gate-skip`): state
  genuinely changed, so STATE.md and the dump must move. That asymmetry is
  correct, not a bug — regeneration tracks a real state change.
- The **only** way `load` could leave STATE.md stale is if the *committed*
  STATE.md was already inconsistent with the committed dump (a hand-edit or a
  `commit -n` bypass). Regenerating STATE.md inside `load` would **mask** that
  committed drift — but R14 / R1 check 3, built *this* stage, is exactly the
  check that must catch it. `load` regenerating would paper over the forgery
  signature verify is meant to surface.

Resolution: **no change to `load`.** The asymmetry is the intended
faithful-rebuild semantics. Instead of an unconditional regenerate, S8c adds a
fixture that loads a repo whose committed STATE.md is stale relative to its
dump and asserts **verify (R1 check 3) flags it** — turning the watch-item
into a positive design assertion rather than a silent coupling.

## Per-row content + placement verdicts

Grouped by deliverable; every row stays S8c unless the amendment above moved
it.

**`state` + STATE.md render (INV-274):** the verb regenerates STATE.md;
`internal/state.Render` (S8a) is the renderer, wired into the pipeline stub
here so writes and `state` and R14 share it.

**R14 / R1 check 3 (INV-275, 293):** STATE.md byte-equals its render, folded
into R1. Red fixture: tamper STATE.md, verify goes red.

**`prime` contract (§11.1):** INV-273 (catalog stub cross-ref, signature
`prime` no args), INV-458 (`epics_active[]` shape: slug, goal, stories
{done, in_progress, ready[], blocked[]}, `criteria_unmet` a count from
`epic_criteria` where `met=0`, never criterion text), INV-459
(`epics_paused[]`/`epics_backlog[]` slug-only), INV-460 (`ready[]`,
`triage[]`=NEEDS-TRIAGE, `in_review[]`, `stale[]`), INV-461 (`totals{}` counts
every capped list plus `parked`, one authoritative representation), INV-462
(`dump_divergence` bool, from the §8.4 read-only divergence report — INV-361,
which rode here from S8b with the verb), INV-463 (`dump_requires_newer_binary`
bool, from the tracked dump header vs `schema.Version`), INV-465
(`sprint_goals[]` = every IN-PROGRESS story, no silent pick), INV-466
(backlog-type lists capped at `prime_cap`, default 20), INV-467 (deterministic
order: id ASC; `stale[]` path ASC; epic lists slug ASC), INV-468
(`sprint_goals[]`/`epics_active[]` never capped), INV-469 (exactly two naming
fields: epic `goal`, story `title`), INV-470 (no note/verdict/reason/DoD text;
O(active) not O(history)).

**`dump_divergence` read-only report (INV-361):** rode from S8b with the verb
(amendment `prime-divergence-rides-prime-at-s8c`). `prime` runs the §8.4
comparison read-only and reports `dump_divergence:true` without touching the
DB or the dump file.

**SessionStart chain (§11.1):** INV-454 (single `sh -c`, three branches, one
JSON object each), INV-455 (healthy `prime` reports `dump_divergence:true` on a
post-pull moved dump, surfaced as a flag not the error branch), INV-456
(`load` in the fallback is the no-`--force` form: fast-forwards behind/missing,
refuses divergent → error branch), INV-457 (stated limitation: fresh clone
whose dump needs a newer binary → `load` refuses → only the static error JSON,
no typed `dump_requires_newer_binary`). The hook text already ships; these
rows verify its behavior against the now-real `prime`.

**Process / design rows (review or skill-owned):** INV-001 (agent-state
discoverability — `prime` is the discovery surface), INV-008 (harness-friendly
core, verified via the §11 integration surface `prime` completes), INV-013
(no-raw-SQL is prose not a gate — documented rationale), INV-022 (WIP=1 schema
index already enforced at S6; `prime` surfaces every sprint goal — the
`prime`-side obligation closes here), INV-023 (backlog refinement loop named
and driven by the skill — the `prime`→triage loop `prime` now feeds), INV-254
(`--corrects` correction-row state matches the corrected row — verb rule, its
`prime`/state relevance is the render), INV-298 (R4 conjunct 3 re-checks the
same at the DB level), INV-349 (skill ends every session with a bookkeeping
commit — §11.3 process, fixture at the hook level).

## Resolved verification commands (fixture-name column → real files)

Fixture names in the inventory are intentions; the real files are resolved at
open so the close review checks addresses, not promises (the S8b close found
three that had drifted — this open pins them up front):

- `internal/verb/state_verb_test.go` (new) — the `state` verb: writes
  `STATE.md` byte-equal to `state.Render` over a seeded DB, mutates the DB,
  re-runs `state`, asserts the file tracks (INV-274).
- `internal/cli/testdata/state.txtar` (new) — the CLI surface for `state`.
- `internal/verb/pipeline_test.go` — extended: a write verb refreshes
  `STATE.md` through the now-wired `stateRender` (the stub becomes live).
- `internal/verify/rules_fs_test.go` (or the R1 test home) — R1 check 3:
  tamper `STATE.md`, assert verify red; a clean tree passes (INV-275/293).
  Plus the watch-item fixture: load a repo whose committed STATE.md is stale
  vs its dump, assert R1 check 3 flags it (load does not mask committed drift).
- `internal/verb/prime_test.go` (new) — the `prime` contract: schema-validate
  the JSON against the §11.1 shape; caps (`prime_cap` truncation on the six
  backlog-type lists, no truncation on `sprint_goals`/`epics_active`);
  deterministic order (id/path/slug ASC per list); `totals{}` enumerates every
  capped list plus `parked` with no duplicate scalar; `criteria_unmet` numeric
  with no criterion text; naming fields present only at `epics_active.goal` and
  `sprint_goals.title` (INV-458–470 less the moved INV-464).
- `internal/verb/prime_test.go` — `dump_divergence` on a diverged dump
  (INV-361/462, read-only: no DB/dump file modified); `dump_requires_newer_
  binary` true on a forged newer header (INV-463).
- `internal/cli/testdata/prime.txtar` (new) — golden `prime` JSON.
- `internal/cli/testdata/sessionstart.txtar` or a Go test driving the `sh -c`
  hook (new) — the three branches: healthy DB → `prime`; missing DB → `load`
  then `prime`; irreconcilable → static error JSON. Exactly one JSON object
  per branch (INV-454–457).
- Doc/process rows (INV-001/008/013/022/023/349) — assertions over the
  generated docs and the skill/hook fixtures, or review notes where no fixture
  is possible (the review states what was checked).

The `make gates` chain (build/vet/test-race/lint/govulncheck/check-pins/
check-inventory) is the close gate; interim evidence per D-EP8 (no CI, no
push).
