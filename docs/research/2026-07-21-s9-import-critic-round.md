# S9 `import` — critic round and adjudication

Date: 2026-07-21. Stage: S9 (`import` + `--legacy`). Reviewed the uncommitted
implementation (`internal/verb/import*.go`, `import_test.go`,
`internal/cli/testdata/import.txtar`) after `make gates` reached exit 0.

Five fresh adversarial critics ran in parallel, read-only, sandboxed
(per-critic `HOME`), distinct non-overlapping lenses, maximum-flaws /
zero-solutions (`.claude/rules/critic-protocol.md`): spec-fidelity,
code-correctness, data/semantics, test-design, security/privacy. Verdicts below
are the coordinator's, refute-by-default; security/privacy refutations
hand-verified.

## Confirmed clean (refutations verified by hand)

- **No command injection.** `authorDate`'s only argument is gated by
  `shaShape = ^[0-9a-fA-F]{7,40}$` before `exec.CommandContext`; the regex
  forbids a leading `-`, and `firstCommitDate` passes the operator's `--file`
  after an explicit `--`. `exec.Command` invokes no shell. (security #8)
- **No privacy leak.** The only address is `importer@test.invalid` (RFC 2606
  reserved); no hostnames, absolute paths, secrets, or non-English prose in the
  new files. (security #9)

## Accepted — defects to fix

- **A1 — DUPLICATE task import writes no `task_links('duplicates')` row → R7
  red** (spec #2, data #3, security #3; empirically confirmed by 3 critics). A
  `--legacy` corpus with a DUPLICATE task imports, then `verify` reports R7 on
  the next run. No `dup_of` validity check either: a chain (`dup_of` → a
  DUPLICATE) or a nonexistent target surfaces as a raw driver error. Fix: pair
  the `dup_of` write with the `duplicates` link; validate the target resolves
  and is not itself DUPLICATE, with a clean `refuse`.
- **A2 — `epic_criteria.criterion`/`.evidence` bypass the §8.1 control-char
  gate** (code #1, security #2; empirical). `validateText` skips criteria; the
  sibling `criteria add` validates them. A raw control byte reaches the DB. Fix:
  validate both fields.
- **A3 — `firstCommitDate` computes the INV-549 bound wrong on non-monotonic
  history** (code #3; empirically reproduced). It takes `git log --format=%aI
  --follow`'s LAST line as the earliest, but git-log is topological, not
  author-date sorted. A rebase/squash-affected history (the exact case this verb
  imports) yields a wrong lower bound. Fix: take the MIN over all `%aI` lines.
- **A4 — commits-cell classification defeats the `--legacy` gate and pollutes
  R5** (security #1, code #2, data #6; empirical). `hasSha` is set on shape
  alone and `storedCommits` returns the RAW cell verbatim before the `legacy:`
  and DONE-without-range checks. So `commits:"legacy: 1234567"` persists WITHOUT
  `--legacy` (the `1234567` is hex-shaped), and `commits:"see abc1234def for
  context"` stores the whole prose verbatim, making R5 run `git cat-file` on
  "see"/"for"/"context". Fix: check the `legacy:` prefix FIRST (gated by
  `--legacy`); treat a cell as a commit citation only when EVERY token is a
  sha-shape or an `a..b` range; a mixed prose+token cell falls to the
  placeholder path; store canonical tokens, never surrounding prose. A lone
  unresolvable sha-shaped token (a typo) is still stored verbatim for R5, per
  §6.2.
- **A5 — an explicit-dated non-terminal task backdates `created_at` with no
  events marking** (data #1, spec #6; empirical). A backdated OPEN task with no
  `import` events row is indistinguishable from a genuine old one. Fix (as
  corrected after the re-critic's RC-1): **(a)** the only date source that needs
  `--legacy` is the **synthesized import-time** one — §6.2 lists exactly three
  `--legacy` relaxations (synthesized timestamps, `legacy:` commits, terminal
  INSERTs), and an *explicit* date field is not among them, so an explicit date
  is admitted without `--legacy` (`import` is "batch creation" — a plain import
  with provided dates must work). Refuse only `source == srcImport && !legacy`
  in both `resolveEpisode` and `resolveTasks`. **(b)** Every imported task gets
  an `import` events row (not only terminal ones) — that marking is what keeps an
  explicit-dated task from being a hidden forgery, so the concern is closed by
  *marking*, not by refusal. NOTE — my first cut of A5 refused *any* non-git
  date without `--legacy`; the re-critic (RC-1) correctly showed that
  over-reached: it pre-decided the escalated E1 question in the restrictive
  direction and broke plain task creation entirely. The behavior above is the
  corrected, non-presumptuous interim; whether an explicit date should
  *additionally* require `--legacy` (the stricter INV-056 reading) is E1.
- **A6 — `epics.created_at` is always `now()`** (data #2; empirical). A CLOSED
  epic's creation postdates its own `close_sweep`, and the value re-derives on
  every import (import-time nondeterminism in the tracked dump). Fix: derive it
  deterministically — an explicit field if given, else the EARLIEST anchor among
  {the epic's earliest imported worklog date, its `close_sweep` when CLOSED},
  else the import moment. (The re-critic's RC-2 caught that the first cut fell
  back to `now()` for a CLOSED epic with no worklog, still postdating
  `close_sweep`; folding `close_sweep` into the fallback closes it.)
- **A7 — re-import of a worklog row for an already-live story crashes with a raw
  `UNIQUE constraint failed`** (data #4; empirical). `insertStories` checks only
  the current corpus and this batch, never the live DB, so backfilling episodes
  onto an existing story (the §10 partial-migration case) aborts with a driver
  string. Fix: skip materialization when the story already exists in the DB;
  refuse an explicit story collision cleanly by name. (RC-3: the re-critic caught
  that this pattern was applied to stories only — `insertEpics` and `insertPaths`
  still raised a raw `UNIQUE` on a re-imported epic slug or `(class,scope)`;
  extended the same existence-check-then-clean-refuse to both.)
- **A8 — warnings/reports flushed to stderr BEFORE `Write`** (spec #4, code
  #16). A subsequently-failed import still printed an INV-549 "reported" line or
  a disagreement warning for rows that never committed. Fix: buffer, flush only
  after `Write` succeeds.
- **A9 — md-table parser silently loses data** (code #4–9, security #7). The
  separator row is discarded without being checked (a missing separator drops a
  data row); duplicate `## section` headers overwrite; a data row with a
  different cell count than its header is truncated/misaligned; an unknown or
  misspelled section is silently ignored; a malformed `dup_of` cell swallows its
  parse error. Fix: make every one of these a loud `refuse`, never a silent
  drop. (GFM `\|` escaped-pipe support is deferred to S10 with a note — the
  strict cell-count check converts its corruption into a loud error meanwhile.)
- **A10 — `legacy:` prefix not space-normalized** (data #7). Cosmetic; folded
  into the A4 rework so one canonical `legacy: ` form is stored.

## Accepted — test strengthening

- **T1** Add a sha-shaped-but-unresolvable (typo) test: stored verbatim, R5
  flags it. This closes the gap the S9 opening record WRONGLY claimed the
  placeholder fixture covered (it uses prose, not a sha-shaped typo) — the
  opening record is corrected at close. (test #9)
- **T2** Add INV-446's required negative: a single summary events row leaves the
  other terminal entities R12-red. (test #6)
- **T3** Split the without-`--legacy` refusal into per-relaxation cases (terminal
  task, terminal epic, terminal story, legacy-commits, synthesized date) so each
  gate is proven, not just the first loop hit. (test #3)
- **T4** Assert the literal `commits` column value is `legacy: …` after a
  round-trip and that R6 accepts it (INV-441). (test #4)
- **T5** `assertGreen` runs only DB-only rules; the round-trip fixture would
  fail R2 (path roots not on disk). Relabel it honestly and note that the
  txtar's `selftracked verify` is the full-verify-green evidence (it `mkdir`s the
  roots). (test #5)
- **T6** Make the INV-266 fixture demonstrate the limitation (a rewritten author
  date is what gets recorded) with an honest comment, not a tautology. (test #1)
- **T7–T10** New tests for A1 (DUPLICATE round-trip R7-green), A6 (epic
  created_at ≤ close_sweep, deterministic), A3 (non-monotonic first-commit), A7
  (re-import onto an existing story).

## Refuted

- **future_increment field carries no control flow** (spec #3, security #6).
  INV-448's obligation — "future-increment rows homed to the epic, not parked" —
  HOLDS: the importer never parks and homes via `epic`. Refuted as an unmet
  obligation. To remove the dead field / make the test meaningful, a
  future_increment task is now required to name an epic (a real gate), and the
  test asserts homed-AND-not-parked distinctly.
- **git dates exempt from the two bounds** (code #18). By design —
  `import-date-bounds` scopes the bounds to non-git dates.
- **Marker spoofing via the `note` field** (security #5). The corpus is trusted
  operator input; owner-steer→note folds prose with no authority claim. No
  privilege boundary exists to protect.
- **`commitTokens` doesn't split on `;`** (code #10). The delimiter set is a
  format choice; the A4 "clean citation" check treats `a;b` as prose (safe).
- **No subprocess timeout / no git-lookup memoization / no DoS cap** (code
  #12/13, security #4). Local single-writer trusted tool; perf/robustness, not
  correctness. A git-lookup cache is a cheap future win — parked, not blocking.
- **Empty corpus is a no-op success** (code #15); **splitIncrements ignores
  top-level commits when increments present** (code #17); **within-batch-only
  worklog ordering** (data #8). Minor / consistent-by-design; parked.
- **`storyTerminal` reuses `epicDissolved`** (spec #7). Cosmetic; a
  `storyDissolved` const is added for clarity.
- **epic_criteria import is unaccounted scope** (spec #8). Importing epics
  includes their criteria (needed to round-trip a CLOSED epic); acknowledged in
  the opening record's scope, no new inventory row.

## Escalated to the owner (post-review, non-blocking)

- **E1** The `--legacy` / explicit-date question. §6.2's relaxation list makes
  only synthesized (import-time) timestamps a `--legacy` feature, so the shipped
  interim behavior admits an *explicit* date without `--legacy` (and marks every
  imported task with an `import` events row so nothing backdates invisibly).
  INV-056's wording ("backfilled timestamps — git-derived or explicit-field —
  are events-marked" under "the one backfill path is `import --legacy`") could be
  read to require `--legacy` for explicit dates too. Which reading governs? The
  interim errs toward §6.2's literal list (plain batch-create works, marked); the
  owner decides whether to tighten. This is the reading my first A5 cut
  prematurely baked in as restrictive — now backed out (RC-1) pending this
  ruling.
- **E2** A calendar-day disagreement is recorded only as prose in `worklog.note`
  (§6.2 "both values recorded"), not in the machine-findable per-row source map
  — a disagreeing `g` is byte-identical in the map to a clean `g` (data #5). Is
  the note sufficient, or should the map encode the disagreement?
- **E3** md-table cannot express a bundled-increment row, so INV-444's split is
  exercised only through JSON; the real S10 ledger corpus is md-table (spec #5).
  Flag for S10's corpus design.
