# selftracked v0 — progress ledger

The bootstrap window's living state (execution plan §6). **Read this first in
every session; update it last.** Disposable by design: at S10 its contents
move into `.selftracked/` and this file is deleted.

Status values: `not started` · `in progress` · `blocked` · `done`.
A stage reaches `done` only with an evidence link — a CI run or a committed
log artifact proving the verification commands exited 0 (plan §5).

## Where things stand

Read this section first; the tables below carry the detail.

**Done:** G0, S0, S1a, S1b, S1c, S2, S3, S4, S5a, S5b, S6, S7, S8a, S8b, S8c, S9.
The full v0 verb catalog, its integrity engine, `init`, and the self-enforcing
gate are live; S8c added the tracker's reader half; and **S9 adds `import
--legacy`** — the one sanctioned backfill door. A batch reader (json + md-table)
runs in front of the shared write pipeline in a single transaction, so every
schema trigger binds as for a live write; a git-first date engine dates each
worklog row from the newest cited commit's author date (else an explicit field,
else — under `--legacy` — the import moment), warns on a calendar-day
disagreement, and refuses a future date while reporting one older than the
source file's first commit. A deterministic per-epic source map records which
source dated each row; one `import` events row per terminal entity keeps R12
green; `--legacy` gates exactly three relaxations (synthesized dates, `legacy:`
commits, terminal INSERTs). It survived a five-lens critic round plus a
verification re-critic (`docs/research/2026-07-21-s9-import-critic-round.md`).
Back on the reader half: `state`
regenerates STATE.md from the DB via the deterministic renderer, and the
pipeline's `stateRender` slot is now wired to it, so every write verb refreshes
STATE.md between the dump and the sidecar (§6.1 order intact). `prime` emits
the §11.1 stable JSON contract: `epics_active` (story tallies + a
`criteria_unmet` count), the slug-only paused/backlog epic lists, the
ready/triage/in_review task lists, `stale`, `sprint_goals`, `totals` (every
capped list plus parked), and the two §8 booleans — the backlog-type lists cap
at `prime_cap`, `sprint_goals`/`epics_active` never do, and the divergence
report and `dump_requires_newer_binary` are read-only (touch neither the DB nor
the dump). The SessionStart hook's three branches work end to end (driven in
test through the exact command the scaffold ships). R1 check 3 (the folded R14)
lands with the renderer: STATE.md byte-equals its render, red on drift; `load`
faithfully rebuilds the DB and does NOT regenerate STATE.md, so committed drift
stays visible to `verify` rather than being masked. The repository carries
`internal/{schema,ref,cli,verb,dump,load,rules,verify,state,scaffold}`. All
local, nothing pushed.

Between S9 and S10 a pre-S10 bugfix batch (owner-ordered, 2026-07-23) closed
the S7-era `set-status DUPLICATE` poison pill: the verb now refuses both
chain-forming directions, so two legal verbs can no longer build a tracker a
fresh clone refuses to `load` — resolved open question 1 below (commits
b3cadcf + 9569d55; one-critic review round, findings adjudicated and applied).

**Next:** S10 — the dogfood switchover (plan §4): this repo's inventory and
ledger are imported into `.selftracked/` as the first epic, the bootstrap ledger
is deleted, and self-host begins. S10 is the importer's first real rehearsal —
S9 built the door it walks through. Most of S10 is plan-native (§3 rule 5), so
it owns a single inventory row (INV-437 already sits at S12; S9's amendment
moved INV-449/450 there too). Open per D-EP13.

**How to verify anything:** `make gates` runs the whole chain. It must exit 0
before a stage closes, and a fresh reviewer re-runs it rather than trusting
the report.

**What is waiting on the owner:** nothing blocking S10. Post-review items,
none blocking: (1) seven amendments applied under D-EP14 —
`r14-rides-its-renderer-at-s8c`; the two from the S8b open
(`gate-skip-joins-the-r8-carve-out`, spec rev 3.17;
`prime-divergence-rides-prime-at-s8c`, plan rev 15); the one from the S8c
open (`migrated-field-rides-migration-at-s11`, plan rev 16 — the `migrated`
field's value/verification rides to S11 with the migration engine); and the one
from the S9 open (`import-guide-reviews-ride-to-s12`, plan rev 17 — INV-449/450,
two migration-guide `review:` rows, move S9 → S12 where the guide is authored;
plan DoD prose and spec both unchanged). (2) Two
S8c spec/design notes from the close review: §11.1's "`load` fast-forwards a
missing/behind DB" overstates `load` (it refuses ANY existing DB; the chain
relies on `prime`'s `dump_divergence` flag for a behind DB, never on `load`) —
a wording question like S8b's rc-triage note; and the cross-statement snapshot
race in `prime`/`state` under concurrent processes (refuted under the
single-writer axiom §1, read-transaction is the remedy if wanted). (3) The
§9 pre-commit's rc-triage signal-death spec-wording note (S8b). (4) ~~The
poison-pill bug in the closed `set-status` verb~~ — **fixed 2026-07-23** in
the pre-S10 bugfix batch (open question 1 below, now resolved). (5) `pause` can orphan an IN-PROGRESS story
into a non-active epic (S8c close; open question below). (6) Three S9 close
escalations (`docs/research/2026-07-21-s9-import-critic-round.md`): **E1** —
does an explicit `date` field require `--legacy`? §6.2's relaxation list says no
(only synthesized timestamps, `legacy:` commits, terminal INSERTs are `--legacy`
features), so the shipped interim admits an explicit date without `--legacy`
while events-marking every imported task; INV-056's wording could be read to
require it. **E2** — a calendar-day date disagreement is recorded in the worklog
`note`, not the machine-findable source map; sufficient for "both values
recorded"? **E3** — md-table cannot express a bundled increment, so INV-444's
split is exercised only through JSON; flag for the S10 ledger corpus.

## Stages

| Stage | Tier | Status | Last verification (command · date · result · evidence) | Open questions |
|---|---|---|---|---|
| G0 — traceability inventory | FULL | done | `python3 scripts/check-inventory.py` · 2026-07-19 · **exit 0, accounting clean** (545 rows, all 16 stages covered, 16 review-only obligations) · evidence link pending (no CI yet — S0 wires it) | Ratified by the owner 2026-07-19 (D-EP4). Three review passes done; findings applied. Fidelity was sampled, not exhaustive — each stage re-reads its own rows at open (plan §5). |
| S0 — repo bootstrap | FULL | done (interim evidence) | `make gates` · 2026-07-19 · all green · local run @ `e09204a`, **no CI has run** (D-EP8) | 5 of 8 rows `verified-by-command`; INV-492/499/513 stay `planned` — they assert CI gates that cannot be proven before the first push |
| S1a — schema as text | FULL | done (interim evidence) | `make gates` · 2026-07-19 · all green · local run @ `6cf73a1`, no CI has run | All 10 rows `verified-by-command`. INV-053 closed once the owner ratified the spec amendment that made its claim true (spec rev 3.10) |
| S1b — schema gates | FULL | done (interim evidence) | `make gates` · 2026-07-19 · all green (114 subtests, `-race`, fresh cache) · local run @ `ad8ef15`, no CI has run (D-EP8) | All 85 rows `verified-by-command`. Opened per D-EP13 (`docs/stage-openings/s1b.md`); two close critics ran; five mutation probes shown red. Adjudications recorded below |
| S1c — driver behaviour | FULL | done (interim evidence) | `make gates` · 2026-07-19 · all green · local run @ `e5b2006`, no CI has run (D-EP8) | All 9 rows `verified-by-command`. Opened per D-EP13 (`docs/stage-openings/s1c.md`); INV-010 → S7 at open; close critic re-ran all probes individually. Adjudications in the close entry below |
| S2 — CLI dispatcher | FULL | done (interim evidence) | `make gates` · 2026-07-19 · all green · local run @ `739f8c9`, no CI has run (D-EP8) | All 20 rows `verified-by-command`. Opened per D-EP13 (`docs/stage-openings/s2.md`); five moves at open; close critic ran every resolved command by name and probed the built binary by hand. Adjudications in the close entry |
| S3 — serializer + `dump` | FULL | done (interim evidence) | `make gates` · 2026-07-19 · all green · local run @ `d20ecf1`, no CI has run (D-EP8) | 19 of 22 rows `verified-by-command`; INV-494/497/507 stay `planned` (CI-half rows, the S0 precedent). Two close-critic mutants killed; adjudications in the close entry |
| S4 — `load` + parser fuzzing | FULL | done (interim evidence) | `make gates` · 2026-07-19 · all green · local run @ `4ee8f22`, no CI has run (D-EP8) | 19 of 20 rows `verified-by-command`; INV-498 stays `planned` (CI fuzz job). Gained INV-346/347/489 at the S5a open — load behaviours mis-filed on S5a, already built and tested here. Security-class review: no parser bypass |
| S5a — task-lifecycle verbs | FULL | done (interim evidence) | `make gates` · 2026-07-19 · all green · local run @ `0699fba`, no CI has run (D-EP8) | All 40 rows `verified-by-command`. Opened per D-EP13; 18 placement moves at open; close critic hand-drove the binary and found five resolved-but-unfixtured rows plus a latent §6.1 order inversion — all closed before the flip. Adjudications in the close entry |
| S5b — relation/artifact/dictionary verbs | FULL | done (interim evidence) | `make gates` · 2026-07-20 · all green · local run @ `b051a2a`, no CI has run (D-EP8) | All 31 rows `verified-by-command`. Two amendments applied under D-EP14 during the stage; close critic found four blockers (symlink containment escape, root-move-into-existing-dir corruption, --with-files zero coverage, untested epic-link path) — all fixed before the flip |
| S6 — epic/story/worklog/criteria verbs | FULL | done (interim evidence) | `make gates` · 2026-07-20 · all green · local run @ `551bb98`, no CI has run (D-EP8) | All 73 rows `verified-by-command`. Largest verb stage; close critic found a real INV-119 blocker (self-transition re-affirm) and invented scope (ready-requires-DoD) — both fixed, one amendment filed. Adjudications below |
| S7 — `verify` | FULL | done (interim evidence) | `make gates` · 2026-07-20 · all green · local run @ `fed963f`, no CI has run (D-EP8) | All 36 rows `verified-by-command` (38 at open − 2: R14/STATE.md's INV-275/293 moved to S8c via amendment `r14-rides-its-renderer-at-s8c`, their renderer being S8c's). Opened per D-EP13 (`docs/stage-openings/s7.md`). Four close critics; one real code defect (R1 check 2 double-counting a DB-only violation), an over-strict-vs-spec R9, an unamended R10 deviation, and branch-level fixture gaps — all fixed before the flip. Critics also found a poison-pill in the closed `set-status` verb (out of scope; parked below). Adjudications in the close entry |
| S8a — `init` scaffold + generated docs | FULL | done (interim evidence) | `make gates` · 2026-07-20 · all green · local run @ `e15759f`, no CI has run (D-EP8) | All 39 rows `verified-by-command`. Opened per D-EP13 (`docs/stage-openings/s8a.md`). Three close critics found two real data-loss bugs — init clobbering a clone's tracked dump, and `--force` wiping the DB — plus a §6.1 write-order inversion, an over-broad adoption claim, and durable-doc rule-2 content dropped by paraphrase; all fixed before the flip. Adjudications in the close entry |
| S8b — hooks + sidecar matrix | FULL | done (interim evidence) | `make gates` · 2026-07-20 · all green · local run @ `dfe7daf`, no CI has run (D-EP8) | All 34 rows `verified-by-command` (35 at open − 1: INV-361 `prime` divergence → S8c via amendment `prime-divergence-rides-prime-at-s8c`, riding its verb). Opened per D-EP13 (`docs/stage-openings/s8b.md`); a second amendment widened the R8 carve-out to the S8b-born `gate-skip` event. Four close critics found six robustness defects — a non-executable-hook-on-refresh no-op, an asymmetric marker-clear window, subdir-blind activation, a post-commit false-positive, plus INV-425 uncovered and stale opening-record addresses — all fixed before the flip. Adjudications in the close entry; correction-at-close in the opening record |
| S8c — `state`, `prime`, SessionStart | FULL | done (interim evidence) | `make gates` · 2026-07-20 · all green · local run @ `c0d75c4`, no CI has run (D-EP8) | All 29 rows `verified-by-command` (30 at open − 1: INV-464 `migrated` field → S11 via amendment `migrated-field-rides-migration-at-s11`, riding the migration engine). Opened per D-EP13 (`docs/stage-openings/s8c.md`); the S8b watch-item (STATE.md on `load`) dissolved to "no change to `load`" with a positive R1-check-3 fixture. Five close critics (spec/code/data/governance/security); accepted fixes: the INV-469 reflective prose-scan, an `atomicWrite` TOCTOU (chmod after rename), a fourth SessionStart branch (present-but-divergent DB → flag, not error), a not-a-git-repo `stale` test. Refuted (single-writer axiom, spec-conformance) and escalated (§11.1 `load` wording, the snapshot race) in the close appendix |
| S9 — `import` | FULL | done (interim evidence) | `make gates` · 2026-07-21 · all green · local run @ `ce68f30`, no CI has run (D-EP8) | All 28 rows `verified-by-command` (30 at open − 2: INV-449/450 → S12 via amendment `import-guide-reviews-ride-to-s12`). Opened per D-EP13 (`docs/stage-openings/s9.md`). Five fresh critics (spec/code/data/test/security) + a verification re-critic; confirmed clean on injection and privacy; accepted fixes: commit-cell classification (legacy-gate bypass + R5 pollution), DUPLICATE→R7 link, §8.1 gate over criteria, MIN first-commit bound, deterministic `epics.created_at`, clean re-import refusals, backfilled-task events marking, md-table loud refusals. Self-corrected an over-reaching A5 (RC-1) caught by the re-critic. Escalated E1–E3 (below). Adjudication: `docs/research/2026-07-21-s9-import-critic-round.md`. INV-014 closed review-only, scope "through S9" |
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
| `epic-close-story-cardinality` | **spec** §6.4 | accepted 2026-07-19 | spec rev 3.13 |
| `link-tables-are-relations-not-history` | **spec** §5 triggers, `ddl.sql`, INV-153/154/155 | accepted 2026-07-19 | spec rev 3.14 |
| `pre-authorized-amendment-cadence` | execution plan §5, §10 | accepted 2026-07-19 (D-EP14) | plan rev 13 |
| `instance-scoped-events-and-r8` | **spec** §5.9/§7 (R8 carve-out for `paths`/`config` events) | accepted 2026-07-20, D-EP14 | spec rev 3.15 |
| `dod-shape-is-authoring-convention` | **spec** §2; INV-017 verification | accepted 2026-07-20, D-EP14 | spec rev 3.16 |
| `r14-rides-its-renderer-at-s8c` | execution plan §4 (S7/S8c); INV-275/293 → S8c | accepted 2026-07-20, D-EP14 | plan rev 14 |
| `gate-skip-joins-the-r8-carve-out` | **spec** §7 R8 + §5.9; `internal/rules` r8; INV-302/137 | accepted 2026-07-20, D-EP14 | spec rev 3.17 |
| `prime-divergence-rides-prime-at-s8c` | execution plan §4 (S8b/S8c); INV-361 → S8c | accepted 2026-07-20, D-EP14 | plan rev 15 |
| `migrated-field-rides-migration-at-s11` | execution plan §4 (S8c/S11); INV-464 → S11 | accepted 2026-07-20, D-EP14 | plan rev 16 |
| `import-guide-reviews-ride-to-s12` | execution plan §4 (S9/S12, prose unchanged); INV-449/450 → S12 | accepted 2026-07-21, D-EP14 | plan rev 17 |

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

S1b was the first stage under the D-EP13 opening record, and the open —
not the close — is where its placement defects surfaced: three rows moved
before any code existed, plus two document defects and one spec defect
(the R4 pointer) found the same way. The close ran two fresh critics; the
checklist critic re-ran the gates twice (once on a cleared test cache,
after noting the Makefile's `test` target reports cached results) and
enumerated every DDL gate against the fixture list — zero gates without a
fixture. Accepted findings, applied before close: the crash-recovery
fixture had claimed a mid-write kill it could not guarantee — it now
holds an uncommitted spilled transaction at the kill and asserts the hot
journal and the exact committed row count; the single-writer fixture
lacked the release-on-close and blocked-reader halves; two refusals
accepted any error; the raw-connection harness missed the trigger
families; the opening record lacked INV-018's content verdict. Refuted:
the "INV-160 tests no race" objection (the spec designs the contention
away; the fixture asserts the observable outcome), exact-expression
pinning of every CHECK (couples fixtures to SQLite's message format),
and the test-scoped lint exclusion as a deviation (a global disable of
the same linters is textually within §3.2's posture; a narrower scope
excludes strictly less — flagged to the owner rather than amended).
INV-030 was re-judged at close as the record requested: it stays at S1b,
executable here and nowhere else without a plan §4 amendment. One
process incident is on the record: the first suite commit rode in on a
false green — a shell-chaining mistake read the wrong exit code while
`make gates` was failing on lint — caught one commit later; exit codes
are now taken from the command itself, not a pipeline tail. The
before-first-write locking probe demanded by a critic proved
unprovable: establishing an EXCLUSIVE-mode connection already excludes
foreign writers (shared lock at open, held by the mode), recorded as an
empirical note in the test rather than asserted away.

S1c closed the same day on one critic's report. Accepted: the second
Serialize call type-asserted unchecked where its siblings fail through a
named error — a panic path in a parallel subtest, made uniform. Refuted,
reasons recorded: renaming INV-173's "bypasses" fixture (the slug is the
inventory's; the row's own text already states the honest scope — absence
of a mechanism, not defeat of one), and a dedicated inventory row for the
gocyclo test-file exclusion (same adjudication as the S1b lint ruling —
config infrastructure inside §3.2's stated posture, not spec scope). The
critic confirmed by literal re-check what the opening record had asked
the close to confirm: S1a's pragma test asserts four of INV-028's five
settings and the new `_dqs=0` behavioural probe is the fifth —
`synchronous`/`locking_mode` are S1a's other test's scope, not INV-028's.
The Serialize byte-identity assertion is reasoned-sound but was not
stress-run; if it ever flakes, the aborted-DELETE-between-snapshots
reasoning in the close report is where to look first.

S2 closed on one critic's report after the fixes landed. Accepted: the
testscript dependency was filed as indirect while directly imported (go
mod tidy; check-pins does not police tidiness); the opening record's two
dependency-audit greps matched the wrong universe — golangci-lint's own
transitive cobra and the sqlclosecheck linter's name — and were corrected
to source-scoped commands; and the bare word "help", scope §3.2 (c) never
granted, was removed outright rather than guarded — it had silently
shadowed any future verb of that name, and deleting the special case
dissolved the shadowing and the top-vs-subverb asymmetry at once.
Refuted: -h consumed as a string flag's value (stdlib parsing is the
design the spec adopts); the empty-string positional (the S2 rule
concerns dash tokens; domains are the verbs' to validate, from S5a on);
and strengthening the elision test past its disclosed mechanism-only
scope — per-verb conformance re-proves at each verb stage, as the
opening record stated. The critic also confirmed all five of the
record's own re-check requests, the §3.2 (b) trio verbatim, and the
extended→primary code masking against a live driver error.

S3's close ran one critic, and its round was the mutation story. The
implementation's own round had already produced two lessons the ledger
keeps: probes that mutate uncommitted files cannot be restored (two
"ok" runs were struck as no-ops after their pattern never matched the
gofmt-ed source — a green probe against an unmutated file proves
nothing), and the ORDER BY removal survived every black-box fixture
because SQLite's scan order coincides with PK order — killed by a
white-box guard that makes the declaration itself load-bearing. The
critic then proved a SECOND surviving mutant the same way: a duplicated
DDL block passed the Contains check and the header fixture, whose
comment whitelist was drawn from the DDL's own lines; the fixture now
demands the block exactly once, directly after the header, and the
mutation is red. Also accepted: NULL pinned by column position, not
table; INV-324's fixture owning both halves of its name. Refuted: an
error-code vocabulary for corrupt databases (no S3 row; it rides S5a's
pipeline contract). The shell-chaining false-green recurred once more
during this stage before the gates and the commit were fused into one
chain; the ledger says so because the third occurrence of a mistake is
a pattern, not an accident.

S4 built a security boundary, so its close review weighted adversarial
parser-bypass hardest — and found none: the statement-boundary attack (a
string value carrying `) VALUES (` and `);`) was hand-verified by the
coordinating agent, not just the critic, to round-trip byte-identically
as one literal, since security-class refutations are checked by hand
here. Four permissiveness gaps were tightened into hard refusals
(non-trailing blank lines, non-canonical integers) or defense-in-depth
(Build re-checks the table whitelist; the DBCONFIG_DEFENSIVE
unreachability note now lives in code). The two seams the opening record
flagged both held: the R-rules subset stays free of verb/reporting/exit
logic (S7's `verify` wraps the same code), and `load --force` implements
only the §8.3 discard floor, never §8.4's sidecar matrix. A minor note
for whoever wires INV-498's CI fuzz job: the local fuzzer's exec rate
falls to near zero after a few seconds against the strict grammar —
harmless for a boundary that refuses almost everything, but worth a
richer corpus when it runs for real.

S5a's close critic hand-drove the built binary in a sandbox and returned
the sharpest report of the build: five rows had been marked resolved
while their fixtures did not exist (the §8.2 diff shapes, the
one-commit-trailing rule, the edit-history pair, the superseded-verdict
trail, PRAGMA optimize), and the write pipeline carried a latent §6.1
inversion — the sidecar landed before the STATE.md slot, invisible only
because the renderer is still a stub. Everything was fixed before any
row flipped: dump diffs are now counted line-exact by a harness command,
the history scenarios cross real git commits, the pipeline order is
pinned by a white-box step trace whose last step is the optimize call,
and read-verb no-side-effects is asserted positively rather than by
omission. The review also confirmed both seams the opening record
flagged (the §8.4 core has exactly four branches; the version gate is
still the S2 stub) and that the interim reopen-of-DUPLICATE refusal
matches what the pending link-tables amendment promises.

S8c built the tracker's reader half — `state`, `prime`, the SessionStart
chain, and R1 check 3 — and drew five critics: spec fidelity, code
correctness, data semantics, governance, security. The open filed one
forced correction before any code: the `migrated` field's contract slot is
built here, but its value needs a real migration, so INV-464 rides to S11
beside the engine (proposal-first under D-EP14). The S8b watch-item —
would `load`'s STATE.md refresh couple to a pending gate-skip marker? —
dissolved under research to "no change to `load`": `load` faithfully
rebuilds the DB and must NOT regenerate STATE.md, because doing so would
mask committed drift that R1 check 3 exists to catch; a positive fixture
now asserts `verify` flags a stale committed STATE.md after a load. The
critics found no data-loss bug. Accepted and fixed before the flip: the
INV-469 prose guard was weaker than its promise (now a reflective walk of
the whole `primeOutput` type graph asserts only `goal`/`title` are prose);
an `atomicWrite` TOCTOU (chmod moved after the rename, so the temp is never
world-readable); a missing SessionStart branch (a present-but-divergent DB
reports `dump_divergence` via the flag and succeeds — the chain never falls
to `load`/error); and a `stale`-degradation test for a non-git directory.
Refuted, documented in the close appendix: the cross-statement snapshot race
in `prime`/`state` (the single-writer axiom §1, the S8b precedent — a
read-transaction is the remedy if the owner wants it); `sprint_goals[]` not
being epic-status-scoped and PLANNED stories being absent from the tallies
(both are §11.1's literal shape — scoping either would deviate); and R1
check 3's `--fast` cadence (the §7 partition by design). Two items escalated
to the owner — §11.1's "`load` fast-forwards a missing/behind DB" overstates
`load` (it refuses any existing DB; the chain relies on `prime`'s flag), and
the snapshot race — plus a parked S6 question: `pause` can orphan an
IN-PROGRESS story into a non-active epic, which `prime` then surfaces.

S8b gave the tracker its git hooks and finished the §8.4 sync matrix,
and drew four critics — spec fidelity, code correctness, shell
robustness, data/semantics + test design. The open filed two forced
corrections before any code: INV-361 (`prime`'s read-only divergence
report) rides its verb to S8c, and the S8b-born `gate-skip` event joins
the R8 instance-scoped carve-out (its entity is a fixed token, not a §4
ref) — both proposal-first under D-EP14. The critics found no data-loss
bug this time, but six real robustness defects, all fixed before the
flip. The sharpest was silent: `os.WriteFile` sets a file's mode only on
creation, so a `--force` refresh over a hook that had lost its executable
bit left it inert — git skips a non-executable hook with no diagnostic,
turning the gate into a no-op; `writeHooks` now chmods unconditionally and
a test asserts it (the golden compares bytes, never mode). Two more: the
standalone marker conversion (`load`'s path) cleared the skip marker only
after the whole derived-file tail, a wider failure window than the write
pipeline's clear-right-after-commit — now symmetric; and activation
resolved the incumbent hooks directory as `<cwd>/.git/hooks`, so an
`init` from a subdirectory missed a real top-level incumbent and would
have printed the takeover that disables it — now resolved via git, the
way verify's R11 already did. The shell critic reproduced a
false-positive in the post-commit: hashing an empty `git show` (dump.sql
absent from HEAD) yields the empty-string hash, a non-empty value that
fired a spurious "you bypassed the gate" warning — now gated on `git
cat-file -e`, and `command -v` replaces a `sha256sum||shasum` fallback
that could hash a drained stdin. The spec critic caught INV-425 skipped
in the coverage enumeration (SELFTRACKED_SKIP bypasses only our gate) —
its fixture and the row are added — and three stale addresses in the
opening record (test files delivered under other names); the correction
is recorded in `docs/stage-openings/s8b.md` rather than silently
rewritten. Refuted: the single marker collapsing multiple pre-write skips
into one event (the spec models one marker, not a counter); concurrent
duplicate events (the single-writer axiom the system does not defend
against by design); and the pre-commit's rc-triage not distinguishing a
signal-killed verify from a RED one — real, but the script is §9
**verbatim**, so it is a spec-wording note for the owner, not a change
here. One latent trap is parked for S8c: once its `stateRender` stub is
wired, `load` must refresh STATE.md whether or not a gate-skip marker
happened to be pending, or the refresh silently couples to an unrelated
per-machine fact.

S8a stood a tracker up from an empty repo and drew three critics —
content fidelity, code correctness, fixture adequacy. The two sharpest
findings were data-loss bugs the critics could reproduce by reading. First:
`init`'s existence guard checked only `db.sqlite`, which is gitignored — so
a fresh clone (tracked `dump.sql`, no local DB) running `init`, a plausible
mistake the generated docs never warned against, silently overwrote the
tracked dump with a near-empty one and clobbered PROMPT.md. `init` now
detects a clone and refuses, pointing at `load`, and `--force` does not
override that guard. Second: `--force` did `os.Remove(dbPath)` and rebuilt
from scratch, discarding every recorded task, epic, and story; it is now a
refresh — the derived files are regenerated from the existing database
(opened read-only), and nothing is dropped. A third code finding: `init`
wrote the dump and its sidecar before STATE.md — the exact §6.1/§8.3 order
inversion the S5a review had already found and fixed in the write pipeline,
recreated here because `init` reused the old composite `WriteFiles`; it now
interleaves the STATE.md render the way the pipeline does. The content
critic caught durable-doc rule 2 shipped as a paraphrase that dropped three
substantive clauses (the epoch-not-arithmetic honesty note, the
unverifiable-forever rationale, the sidecar elaboration) and a
self-contradiction in the `work/` README calling `report` ephemeral when it
is durable — both restored, and the §11.2 deny-list entry it named
(`Bash(sqlite3:*)`) added to the generated settings. The fixture critic
found the coverage broad but shallow: several rows leaned on the golden's
tautology (init writes what init writes), the path-seed count compared the
seed slice to itself, and INV-484/475/545 had no test at all — all now
carry real assertions, and the renderer's determinism is proven against a
multi-epic, thirteen-event fixture that actually exercises ordering and the
ten-event window. Refuted: the generated docs naming `prime`/`state`/the
pre-commit hook (verbs and hooks that land at S8b/S8c — the shipped docs
describe the full-v0 contract by design, and the opening record anticipates
`init` growing across the layers). The dump.sql 0600 file mode a critic
noted is pre-existing in `internal/dump`, not S8a's, and left for its
owning stage.

S7 gave `verify` its rules and drew four critics — spec fidelity, code
correctness, semantics, and fixture adequacy. The open had already made
one forced placement correction: R14 / R1's third check (STATE.md
byte-equals its render) cannot precede the renderer that produces STATE.md,
and that renderer is S8c's, so INV-275/293 moved there by amendment and R1
shipped with checks 1–2 only. The critics found one genuine code defect:
R1 check 2 reuses the real loader (`load.Build`), which re-runs the DB-only
rules and refuses on any of them — so a single R6/R9/R12 violation surfaced
twice, once correctly and once mislabelled "the dump does not rebuild", and
a real infrastructure failure (no temp dir) became a rule finding rather
than an exit-2 error. Check 2 is now gated on a clean DB-only pass and
reports infra failures as errors. Two smaller fidelity fixes: R9's boundary
clause accepted `"+0"`/`"00"` (a numeric-zero test where §8.2's tamper
argument is textual — tightened to the exact byte-string `"0"`), and R10
carried a `created_at` idle floor that §7 never states — an unamended
deviation, reverted to the literal rule. The fixture critic and the spec
critic together showed the red-fixture set was rule-complete but
branch-thin: R9's `sqlite_sequence` floor (a distinct row, INV-303) and
R7's chain clause (INV-301's named clause) had no fixture at all, and
several rules exercised only one of two code paths. All now covered, and
`TestRuleFixtureCoverage` is the mechanized per-rule audit INV-495 asks for
— the gate that would have caught the R7/R8 miss the first implementation
pass made and only a manual second pass found.

The semantics critic found the build's most dangerous defect, and it is
**not** in S7: two legal `set-status DUPLICATE` verbs can build a task
chain (A duplicate-of B, then B duplicate-of C leaves A pointing at a
now-DUPLICATE B) that R7 correctly flags — and because R7 runs inside
`load`, a fresh clone of that tracker cannot `load` it. R7 is right; the
`set-status` verb (a closed stage) fails to prevent the chain. Confirmed by
hand, parked for the owner below, and out of S7's scope — it did not block
the close. Refuted: the R12-authenticity note (§1.1/INV-012 already state
that a forger who also inserts a matching events row defeats R12 — detection,
not prevention) and the GLOB-looseness note (`V-[0-9]*` inherits the DDL's
own pattern; not new to S7).

S6, the largest verb stage, closed on the sharpest report of the build.
The blocker was subtle and real: the story transition verbs trusted only
the schema matrix trigger, which treats a same-status write as a legal
no-op — so `story done`/`block`/`dissolve` could be re-called on an
already-terminal story, each appending a duplicate worklog episode,
directly against INV-119 ("rework is a NEW story; the old story's history
is never altered"). Every transition now runs through a source-guarded
helper. The critic also caught invented scope — `story ready` refusing an
empty DoD, which the spec never asked for (readiness's precondition is
DoR, the empty-blocked field); removed, and INV-017's genuinely
unmachine-checkable DoD-shape rule reclassified review-only by amendment.
Close conditions (2) and (5) turned out unreachable through the verb
surface at all — the verbs write status and worklog atomically and
worklog is append-only, so only import (S9) or a raw writer can produce
their pathological states — proven now by a white-box test seeding that
state directly, their inventory verification corrected from a fictional
CLI fixture to that test. The regression flip, epic dissolve's cascade,
and a cross-story correction all gained the fixtures the review showed
they lacked.

S5b closed on a four-blocker report, every one fixed before a row
flipped. The sharpest: link's containment was lexical only — a symlink
inside the registered root reaching outside it linked cleanly (now the
real path must live under the real root); and a root move into an
existing directory made git mv NEST the old root inside it while the
dictionary pointed at the parent — exit 0, every artifact resolution
dangling, and invisible to the DB-only rules (now a move demands a
fresh destination, and --with-files carries fixtures for both
transports). The epic-link path had never been driven by a test;
INV-155 closes with its named fixture now real. Smaller accepted
findings: rel tree's undirected relates rendering, link-form flag
bleed, the files-stayed notice, unlink's delete-half assertion, git
stderr in stale's failures. Refuted: the space-in-glob limitation (the
§5.1 value grammar is whitespace-separated by design), "tree" naming,
and the dash-leading dispatcher refusal layer. Two amendments were
applied mid-stage under D-EP14: the ratified link-tables inversion
(rev 3.14) and the R8 carve-out for instance-scoped paths/config
events (rev 3.15) — the latter found because the spec obliged those
verbs to write events R8 would then flag.

## Parked — out of scope, no decision needed yet

Work found mid-stage and deliberately not done (`.claude/CLAUDE.md`, scope
discipline). Nothing here blocks anything; it exists so it is not rediscovered.

| Item | Found during | Note |
|---|---|---|
| Plan §2.1 describes an OpenSpec change as "proposal + delta" with a `tasks.md` pointer; the three change directories hold only `proposal.md` | first-commit review | The tool is adopted but not installed, so the convention has nothing to run against yet. Revisit when it is. |
| `internal/dump`'s `WriteDumpFile` creates `dump.sql` at mode 0600 (from `os.CreateTemp`+rename), not the 0644 the other tracked files use | S8a close review (code critic) | More restrictive, not a leak, and pre-dates S8a; git tracks content not mode, so it is cosmetic. If it matters, `internal/dump` is the owner. |
| Once S8c wires the `stateRender` stub, `load`'s STATE.md refresh will depend on whether a gate-skip marker happened to be pending (the standalone conversion runs `regenerateDerived`, `load`'s no-marker path does not) | S8b close review (data/semantics critic) | **RESOLVED at S8c open.** Researched to dissolution: `load` must NOT regenerate STATE.md — that would mask committed drift R1 check 3 exists to catch. No change to `load`; a positive fixture asserts `verify` flags a stale committed STATE.md after a load. |
| The §9 pre-commit's rc-triage treats every non-{0,2} exit as a bypassable RED, including signal deaths (130/137/143) | S8b close review (shell-robustness critic) | The script is quoted **verbatim** from §9, so changing it is a spec-wording decision. Raised for owner post-review; not an implementation change. |
| §11.1 says the fallback `load` "fast-forwards a missing/behind DB and refuses a divergent one"; the actual `load` refuses ANY existing DB without `--force` | S8c close review (spec-fidelity critic) | The SessionStart chain is nonetheless correct: `prime` fails only on a missing DB, so `load` only ever sees the fresh-clone case; a present-but-behind DB is handled by `prime`'s `dump_divergence` flag, never by `load`. The prose describing `load` is imprecise — a spec-wording question for the owner, like the rc-triage note. |
| `prime`/`state` read each capped list's total and its entries with two separate autocommit queries, so a concurrent writer committing between them can make `totals.X` and the list disagree | S8c close review (code + semantics critics) | Refuted under the single-writer axiom (§1), the same basis as the S8b concurrent-writer refutation; a momentary off-by-a-few in an advisory count that self-heals next `prime`. A read-transaction wrapping `prime`/`state`'s queries is the remedy if the owner wants snapshot consistency. |

## Open questions for the owner

1. **RESOLVED 2026-07-23 — `set-status DUPLICATE` can no longer build an
   unloadable tracker** (raised at the S7 close by the semantics critic,
   confirmed by hand, 2026-07-20; fixed in the pre-S10 bugfix batch the
   owner ordered). The poison: A `DUPLICATE --dup-of B`, then B `DUPLICATE
   --dup-of C` left A.dup_of pointing at a DUPLICATE — R7 flags it inside
   `load`'s pre-rename checks, so a fresh clone refused to load.
   `duplicateTarget` enforced §5.5's no-chains rule in one direction only
   (the canonical must not be DUPLICATE); it now also refuses marking a
   task DUPLICATE while some task's dup_of points at it, naming the
   dependant and the reopen-first unwind. Spec-conformant (§5.5: "refused
   by set-status" — the invariant, not one direction), so no amendment;
   INV-087's verification cell now carries both directions. Fixture: the
   exact hand-confirmed scenario, shown red without the guard by a
   mutation probe, plus the unwind and a `verify --fast` R7-clean end
   state (the latter from the one-critic review of the fix; its other
   accepted finding was this ledger entry itself). Commits b3cadcf +
   9569d55, `make gates` green on a cleared cache.

2. **`pause` can orphan an IN-PROGRESS story into a non-active epic**
   (raised at the S8c close by the data/semantics critic, confirmed by hand,
   2026-07-20). *What happens:* `epicTransition` (`internal/verb/epics.go`,
   used by `epic pause` and `epic activate`) runs a bare status `UPDATE`
   with no blocker check, unlike `epic dissolve` (which refuses while a
   story is IN-PROGRESS). So an epic can be PAUSED while it holds an
   IN-PROGRESS story. *What surfaces:* `prime`'s `sprint_goals[]` is "every
   IN-PROGRESS story" (§11.1, no status qualifier), so it then lists a goal
   for an epic that `epics_active[]` omits — a sprint goal the reader cannot
   attribute to any active epic. *Scope:* `prime` is spec-conformant (§11.1
   is explicit); the question is whether `pause` should refuse an
   IN-PROGRESS story (like `dissolve`) or the orphan is intended
   nothing-hides behaviour. An `epics.go`/spec decision, the owner's, not
   `prime`'s. *What it needs if changed:* a pause-time blocker in the verb
   (an S6-surface change via amendment), or an explicit spec note that the
   orphan is intended.

3. **Amendment `link-tables-are-relations-not-history`** (raised at S5a
   implementation, 2026-07-19): §5 puts no-delete triggers on
   `task_links`/`task_artifacts`/`epic_artifacts`, but `reopen` must clear
   the duplicates link, `rel rm` and `unlink` must delete link rows, and
   R7 demands links ⇔ dup_of one-to-one — any two hold, not all three.
   The proposal removes the three link-table triggers: link rows are
   current relations whose audit trail is the events log (`rel`, `link`,
   `unlink` events), while every entity table keeps its trigger.
   INV-153/154/155 re-open and close at the sanctioned deleters' stages.

Earlier: the S1b-open round — The S1b-open round (three items, 2026-07-19) was answered the
same day: cardinality enforcement chosen over a definitional reading
(amendment `epic-close-story-cardinality`, condition 6 at `epic close`,
INV-016 → S6); the R5→R4 pointer amendment ratified and applied; anchor
line numbers ruled out — stripped in favour of section-only anchors, the
same reasoning that removed revision numbers from the rules file.

Earlier rounds: all answered 2026-07-19 — the `as of dump <sha12>` anchor
keeps its deferred validator (D-EP12); D-EP9 ratified; the §5
nullable-column wording corrected in the specification itself; the
English-only convention covers quoted decisions explicitly
(`.claude/CLAUDE.md`).
