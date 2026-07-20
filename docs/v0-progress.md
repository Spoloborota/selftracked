# selftracked v0 — progress ledger

The bootstrap window's living state (execution plan §6). **Read this first in
every session; update it last.** Disposable by design: at S10 its contents
move into `.selftracked/` and this file is deleted.

Status values: `not started` · `in progress` · `blocked` · `done`.
A stage reaches `done` only with an evidence link — a CI run or a committed
log artifact proving the verification commands exited 0 (plan §5).

## Where things stand

Read this section first; the tables below carry the detail.

**Done:** G0, S0, S1a, S1b, S1c, S2, S3, S4, S5a, S5b, S6, S7, S8a, S8b. The
full v0 verb catalog, its integrity engine, and `init` are live; S8b adds
what makes the gate self-enforcing. `init` now generates the tracked git
hooks (`.selftracked/hooks/{pre,post-commit}`): the pre-commit is §9
verbatim — verify `--fast`, dump + STATE refresh, staging, a non-blocking
`stale` — and the post-commit is warn-only (untraced production commit;
the sidecar/blob mismatch that is the only in-repo trace of `git commit
-n`). `gate skip-mark` writes the per-machine skip marker with no DB write
mid-commit; the next write verb, or `load`, converts it into a `gate-skip`
event. `init` prints the per-machine activation: the takeover command on a
clean repo, a chaining recipe (exit-propagated pre-commit, top-placed
post-commit, subprocess-not-source) when a hooksPath or incumbent hook
exists. The §8.4 sync matrix is complete: R11 detects chaining against the
real generated hooks, a two-writer edit conflicts textually with no merge
driver, and the sidecar hashes the last dump. The repository carries
`internal/{schema,ref,cli,verb,dump,load,rules,verify,state,scaffold}`. All
local, nothing pushed.

**Next:** S8c — `state`, `prime` (§11.1 contract, incl. the `dump_divergence`
read-only report that rode here from S8b), the SessionStart chain, and R1
check 3 / R14 (STATE.md byte-equals its render) landing with the renderer.
Open per D-EP13. Watch-item at open: `load` must refresh STATE.md whether or
not a gate-skip marker is pending (parked below).

**How to verify anything:** `make gates` runs the whole chain. It must exit 0
before a stage closes, and a fresh reviewer re-runs it rather than trusting
the report.

**What is waiting on the owner:** nothing blocking S8c. Post-review items,
none blocking: (1) four amendments applied under D-EP14 —
`r14-rides-its-renderer-at-s8c`, and the two filed at the S8b open
(`gate-skip-joins-the-r8-carve-out`, spec rev 3.17; `prime-divergence-rides-prime-at-s8c`,
plan rev 15); (2) the §9 pre-commit's rc-triage does not distinguish a
signal-killed verify from a RED one — a spec-wording note, since the script
is quoted verbatim (parked below); (3) the **poison-pill bug in the closed
`set-status` verb** from the S7 close — out of scope, not blocking, but it
lets two legal verbs build a tracker no fresh clone can `load` (open
question below).

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
| `epic-close-story-cardinality` | **spec** §6.4 | accepted 2026-07-19 | spec rev 3.13 |
| `link-tables-are-relations-not-history` | **spec** §5 triggers, `ddl.sql`, INV-153/154/155 | accepted 2026-07-19 | spec rev 3.14 |
| `pre-authorized-amendment-cadence` | execution plan §5, §10 | accepted 2026-07-19 (D-EP14) | plan rev 13 |
| `instance-scoped-events-and-r8` | **spec** §5.9/§7 (R8 carve-out for `paths`/`config` events) | accepted 2026-07-20, D-EP14 | spec rev 3.15 |
| `dod-shape-is-authoring-convention` | **spec** §2; INV-017 verification | accepted 2026-07-20, D-EP14 | spec rev 3.16 |
| `r14-rides-its-renderer-at-s8c` | execution plan §4 (S7/S8c); INV-275/293 → S8c | accepted 2026-07-20, D-EP14 | plan rev 14 |
| `gate-skip-joins-the-r8-carve-out` | **spec** §7 R8 + §5.9; `internal/rules` r8; INV-302/137 | accepted 2026-07-20, D-EP14 | spec rev 3.17 |
| `prime-divergence-rides-prime-at-s8c` | execution plan §4 (S8b/S8c); INV-361 → S8c | accepted 2026-07-20, D-EP14 | plan rev 15 |

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
| Once S8c wires the `stateRender` stub, `load`'s STATE.md refresh will depend on whether a gate-skip marker happened to be pending (the standalone conversion runs `regenerateDerived`, `load`'s no-marker path does not) | S8b close review (data/semantics critic) | An **S8c watch-item**, not a current bug (stateRender is a no-op today). S8c must render STATE.md on `load` regardless of the marker, or the refresh couples to an unrelated per-machine fact. |
| The §9 pre-commit's rc-triage treats every non-{0,2} exit as a bypassable RED, including signal deaths (130/137/143) | S8b close review (shell-robustness critic) | The script is quoted **verbatim** from §9, so changing it is a spec-wording decision. Raised for owner post-review; not an implementation change. |

## Open questions for the owner

1. **`set-status DUPLICATE` can build an unloadable tracker** (raised at
   the S7 close by the semantics critic, confirmed by hand, 2026-07-20).
   *What breaks:* mark task A `DUPLICATE --dup-of B` while B is OPEN
   (allowed), then mark B `DUPLICATE --dup-of C` while C is OPEN (also
   allowed). Now A.dup_of = B and B.status = DUPLICATE — a chain. R7
   ("no dup_of target is itself DUPLICATE") correctly flags it, and R7 runs
   inside `load`'s pre-rename checks (§8.5), so a fresh clone of that
   tracker **refuses to load**. *Why it arises:* `duplicateTarget`
   (`internal/verb/tasks.go:554`) checks that the *target's* status isn't
   DUPLICATE, but never checks whether the task being marked DUPLICATE is
   already someone else's `dup_of` target. §5.5 intends the verb to enforce
   no-chains "re-checked by R7"; the verb enforces one direction only.
   *Scope:* the defect is in `set-status` (a closed stage), not in `verify`
   — R7 is correct — so it did not block S7. *What it needs:* a reverse
   guard in the verb (refuse marking a task DUPLICATE while it is a dup
   target, or re-point the dependant), plus a fixture; likely a small
   bugfix batch of its own. Recommendation: schedule it before S10
   (dogfood switchover), since self-hosting makes an unloadable tracker a
   live hazard rather than a hypothetical.

2. **Amendment `link-tables-are-relations-not-history`** (raised at S5a
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
