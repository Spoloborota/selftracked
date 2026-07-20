# S8b opening record

Stage: S8b — hooks + the §8.4 divergence matrix, full (plan §4). Opened
2026-07-20, per D-EP13. Spec revision at open: 3.17 (this open filed the
`gate-skip-joins-the-r8-carve-out` amendment that produced 3.17). Plan
revision at open: 15 (this open filed both amendments below that produced
rev 15). Rows owned at open: 35 → 34 after the placement correction below.

Scope: the pieces §9 and §8.4 add on top of S8a's `init`, none of which
S8a's bare scaffold carried —

- **Generated git hooks.** `init` writes `.selftracked/hooks/pre-commit`
  (verbatim §9, lines 1082–1103) and `.selftracked/hooks/post-commit`
  (warn-only). Both are **tracked**, not gitignored (INV-401). The
  pre-commit is the gate: binary-missing skip, `SELFTRACKED_SKIP=1` bypass
  through `gate skip-mark`, `verify --fast --quiet` rc-triage (rc=2 not
  bypassable, RED bypassable-once), `dump`, `state`, `git add`, a
  non-blocking `stale`. The post-commit warns on an untraced production
  commit and on a sidecar/blob mismatch (the only in-repo trace of a
  `git commit -n` bypass).
- **`gate skip-mark`** — the marker writer the pre-commit skip path calls;
  writes `.selftracked/skip-pending`, no DB write mid-commit (INV-276).
- **Marker → events conversion.** The next write verb, or `load`, converts
  a pending marker into a `gate-skip` events row and clears it (INV-277).
- **init's activation print.** The takeover command + trust note on a clean
  repo (INV-419); a chaining recipe covering both hooks when `core.hooksPath`
  is set or an incumbent hook is non-empty (INV-420–424).
- **§8.4 matrix completion.** The decision core (`divergenceCore`) landed at
  S5a; S8b adds the fixtures the plan §4 DoD enumerates — sidecar match,
  regenerate-match heal, differ refuse, missing sidecar — plus the
  two-writer textual-conflict and file-sync-exclusion rows (INV-003, 351,
  362, 363).
- **R11 closing.** The detector was built at S7 (`internal/verify/r11.go`);
  S8b adds the fixture that runs it against `init`'s actual generated hooks
  (INV-307), which did not exist until this stage.

## Placement corrections (two amendments, filed at this open)

| Rows | To | Why |
|---|---|---|
| INV-361 (`prime` reports `dump_divergence` read-only) | S8c | It is a `prime` contract, and `prime` is built at S8c — every other `prime` row is homed there. The §8.4 machinery it reads stays at S8b; the verb that reads it read-only rides its verb. Amendment `prime-divergence-rides-prime-at-s8c`. Same shape as `r14-rides-its-renderer-at-s8c`. |
| INV-302, INV-137 (R8 / events.entity grammar) | stay S7 | Not moved — but their statements widen. `gate-skip` is instance-scoped like `paths`/`config` and has no §4 entity; R8 would flag the first converted row. The carve-out (already accepted for paths/config) widens to gate-skip. Amendment `gate-skip-joins-the-r8-carve-out`. The code change to r8 and its fixture land here, at S8b, where the event is first born. |

Distribution updated (S8b 35 → 34, S8c 29 → 30); `check-inventory` exits 0.
Plan §4 S8b/S8c DoD amended; plan rev 14 → 15; spec rev 3.16 → 3.17.

## Cross-stage dependency, named at open (not deferred)

The generated `pre-commit` invokes `selftracked state` (INV-431) and
`selftracked stale` (INV-433); `state` does not exist until S8c. This is not
a deferral — the hook is **generated text**, quoted verbatim from §9, and it
is the full v0 contract by design (as PROMPT.md already names `prime`/`state`
at S8a). Its branches are tested by stubbing a fake `selftracked` on `PATH`
and driving the script under `sh`; no test needs the real `state` verb.
Live activation of the hooks against this repo is the S10 dogfood switchover,
by which point every referenced verb exists. `init` never auto-activates the
hooks — it prints the opt-in command — so nothing self-breaks at S8b.

## Per-row content + placement verdicts

Grouped by deliverable; every row's placement is confirmed S8b unless the
table above moved it.

**Hook generation (init/scaffold):** INV-396 (layout), INV-399 (dump.hash
sidecar), INV-400 (skip-pending), INV-401 (tracked `hooks/`), INV-419–424
(activation print / chaining recipe / exit-propagation / top-placement /
subprocess-not-source / skip-scope), INV-426–433 (pre-commit branches),
INV-434–435 (post-commit warnings), INV-436/486/488 (POSIX limit, visible
staging, per-machine execution). All are §9 obligations that need the hook
files to exist — none could close at S8a, where the scaffold explicitly
excluded hooks ("Hooks are S8b's").

**gate skip-mark + conversion:** INV-276 (marker write, no DB write),
INV-277 (conversion by next write verb / load), INV-279 (per-machine
visibility limit). The `gate` verb is new; the conversion extends the S5a
write pipeline and `load`.

**§8.4 matrix:** INV-003 (single-writer violation surfaces loud), INV-351
(dump.hash convention), INV-362 (two-writer textual conflict, no merge
driver), INV-363 (file-sync exclusion advice in generated docs), INV-290
(R1-from-fast rationale: one serialization pass). The core exists; these are
the fixtures and the doc row that complete the matrix.

**R11:** INV-307 — detector built S7, closing fixture against generated
hooks lands here.

**Stated limitations / threat model (review-only):** INV-231 (criteria add
no attack surface), INV-436 (POSIX-runner scope), INV-488 (per-machine hook
execution disclosed). These are review rows — no fixture proves a
threat-model equivalence; the review states what was checked.

## Resolved verification commands (fixture-name column → real files)

- `internal/cli/testdata/init.txtar` — extended: assert `.selftracked/hooks/`
  and its two hook files exist and are NOT gitignored (INV-401), the layout
  rows (INV-396/399/400), and init's stdout carries the activation command
  on a clean repo.
- `internal/scaffold/hooks_test.go` (new) — the activation/chaining-recipe
  logic (INV-419–424): a Go test that runs `init` against temp git repos in
  each incumbent state (no hooksPath, hooksPath set, incumbent pre-commit
  file present) and asserts the printed lines.
- `internal/scaffold/hookscript_test.go` (new) — the generated pre-commit's
  branches (INV-426–433) and post-commit's warnings (INV-434/435), each
  exercised by writing a stub `selftracked` onto `PATH` and running the
  generated script under `sh` with the branch's forced condition; INV-424's
  source-vs-exec hazard demonstrated (INV-422's exit-propagation, INV-423's
  top-placement) with stub incumbent hooks.
- `internal/verb/gate_test.go` (new) — `gate skip-mark` writes the marker
  and touches no DB (INV-276); the conversion path (INV-277) via a write
  verb and via `load`; the gate-skip row leaves R8 green (amendment fixture).
- `internal/cli/testdata/gate.txtar` — the CLI surface: `gate skip-mark`
  then a write verb, asserting the `gate-skip` event lands and the marker
  clears; `verify` reports the pending marker (R15) meanwhile (INV-279).
- `internal/verb/pipeline_test.go` / a divergence scenario — the §8.4
  matrix branches already partly covered at S5a; S8b adds the two-writer
  textual-conflict fixture (INV-362) and the single-writer-loud assertion
  (INV-003).
- `internal/verify/r11_test.go` — extended with a case that generates real
  hooks via `init` and asserts R11 detects the chained/unchained states
  (INV-307).
- Doc rows INV-363/436/488/231 — assertions over the generated docs
  (file-sync exclusion advice, POSIX-runner note, per-machine disclosure)
  and a review note for the threat-model equivalence.

The `make gates` chain (build/vet/test-race/lint/govulncheck/check-pins/
check-inventory) is the close gate; interim evidence per D-EP8 (no CI, no
push).

## Correction at close

The close review (four fresh critics) found three places where this record,
written before the code, did not match what was delivered. Recorded here
rather than silently rewritten, so the diff between plan and reality is
legible (D-EP13's close-review obligation):

- **Line citation.** The scope section said the pre-commit is "verbatim §9,
  lines 1082–1103." The correct span is `docs/v0-spec.md` **1089–1108**
  (fence at 1088). The generated template byte-matches that block — only the
  citation was off (a secondary-source number, the exact failure the
  provenance rule names).
- **Verification-file locations.** The "Resolved verification commands"
  section named files that were delivered under different names/mechanisms:
  the activation/chaining tests live in `internal/scaffold/activate_test.go`
  (not `hooks_test.go`); the INV-307 R11 fixture is the testscript
  `internal/cli/testdata/hooks-r11.txtar` (not an extension of
  `internal/verify/r11_test.go`); the INV-362/INV-003 two-writer fixture is
  `TestTwoWriterDumpConflictsLoudly` in `internal/scaffold/sync_test.go` (not
  in `internal/verb/pipeline_test.go`). All the tests exist and pass; only
  their promised addresses were wrong.
- **INV-425 was omitted.** The coverage enumeration stepped from "419–424" to
  "426–433", skipping INV-425 (SELFTRACKED_SKIP bypasses only our gate). The
  close review added the fixture
  (`TestChainingRecipeHazards/...INV-425`) and the row closes with the rest.

## Accepted critic fixes and refutations

Applied at close (commit dfe7daf): the hook executable-bit chmod on refresh
(+ test); the ConvertSkipMarker marker-clear moved before the derived-file
tail (symmetry with the write pipeline); activation's incumbent-hooks-dir
resolved via git (subdir-safe, + test); the post-commit sidecar check gated
on `git cat-file -e` to kill an empty-hash false positive; the config-ls test
stub made realistic; the gate-skip conversion tests extended to assert
dump/sidecar consistency.

Refuted, and why: the single marker collapses multiple pre-write skips into
one event — the spec models one marker, not a counter; concurrent-writer
duplicate events fall under the single-writer axiom (§1) the system does not
defend against by design; the pre-commit's rc-triage does not distinguish a
signal-killed verify from a RED one — but the script is §9 **verbatim**, so
this is a spec-design observation raised for owner post-review, not an
implementation change here.

## Parked for later (owner post-review / S8c)

- **STATE.md refresh coupling (S8c watch-item).** Once S8c wires the
  `stateRender` stub, `load`'s STATE.md refresh will depend on whether a
  gate-skip marker happened to be pending (the standalone conversion runs
  `regenerateDerived`, `load`'s no-marker path does not). S8c must render
  STATE.md on `load` regardless, or the coupling becomes observable.
- **Signal-death rc-triage (owner).** The §9 pre-commit treats every
  non-{0,2} exit as a bypassable RED, including 130/137/143 (signal deaths).
  A spec-wording question for the owner, since the script is quoted verbatim.
