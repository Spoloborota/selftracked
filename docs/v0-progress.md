# selftracked v0 — progress ledger

The bootstrap window's living state (execution plan §6). **Read this first in
every session; update it last.** Disposable by design: at S10 its contents
move into `.selftracked/` and this file is deleted.

Status values: `not started` · `in progress` · `blocked` · `done`.
A stage reaches `done` only with an evidence link — a CI run or a committed
log artifact proving the verification commands exited 0 (plan §5).

## Where things stand

Read this section first; the tables below carry the detail.

**Done:** G0, S0, S1a, S1b, S1c, S2, S3, S4, S5a, S5b. Eighteen verbs live: the full task lifecycle, relations with cycle refusal, artifact links with real-path containment, the path dictionary with safe root moves, validated config, the stale detector, per-entity logs. The repository now carries the full
schema layer (`internal/schema`: DDL, connection posture, three test
suites) and the CLI skeleton (`internal/ref` grammar, `internal/cli`
dispatcher with a closed registry and structural `--json`, the §6.1 exit
mapper, the version-gate stub, the testscript e2e harness, `cmd/selftracked`
built under both decided names). All local, nothing pushed.

**Next:** S6 — epic/story/worklog/criteria verbs, the largest verb
stage (80 rows): the epic lifecycle with close's atomic retro (including
the ratified condition 6), the story state machine with WIP/DoR, worklog
appends with corrections, runnable criteria. Open per D-EP13.

**How to verify anything:** `make gates` runs the whole chain. It must exit 0
before a stage closes, and a fresh reviewer re-runs it rather than trusting
the report.

**What is waiting on the owner:** nothing blocking. Amendments now apply
under the D-EP14 pre-authorization: proposals are still filed as
artifacts first, the owner reviews after the fact and may revert.

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
| `epic-close-story-cardinality` | **spec** §6.4 | accepted 2026-07-19 | spec rev 3.13 |
| `link-tables-are-relations-not-history` | **spec** §5 triggers, `ddl.sql`, INV-153/154/155 | accepted 2026-07-19 | spec rev 3.14 |
| `pre-authorized-amendment-cadence` | execution plan §5, §10 | accepted 2026-07-19 (D-EP14) | plan rev 13 |

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

## Open questions for the owner

1. **Amendment `link-tables-are-relations-not-history`** (raised at S5a
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
