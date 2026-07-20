# selftracked v0 — Execution governance & staged implementation plan

Status: **ACTIVE, revision 14.** Revision history: rev 1 →
five-lens adversarial critic round (methodology fidelity, spec
applicability, tooling comparison, process soundness,
fidelity/publication) → rev 2 → three-lens control round on the fixes →
rev 3 → owner decisions closing D-EP1–D-EP3 (§10) → rev 4; G0 ran and
closed → amendment `plan-accounting-scope` (D-EP4, D-EP5) → rev 5 →
amendment `stage-open-plan-crosscheck` (D-EP6) → rev 6 → amendment
`review-proportionality-tiers` (D-EP7) → rev 7 → amendment
`local-commits-and-interim-evidence` (D-EP8) → rev 8 → amendment
`s0-minimal-package` (D-EP9) → rev 9 → amendment `split-s1` (D-EP10) →
rev 10 → amendment `evidence-across-a-squash` (D-EP11, D-EP12) → rev 11 →
amendment `stage-open-record` (D-EP13) → rev 12 →
amendment `pre-authorized-amendment-cadence` (D-EP14) → rev 13 →
amendment `r14-rides-its-renderer-at-s8c` (R14/STATE.md check moves
S7 → S8c, where its renderer is built) → this revision. Governs the implementation of `docs/v0-spec.md`
and the project's spec lifecycle beyond it. Derived from
`docs/research/2026-07-18-spec-to-execution-planning.md`; tooling facts
re-verified against primary sources on 2026-07-18 (see that document's
Addendum). This document is itself governed: changes to a stage's scope or
DoD travel the same amendment flow as spec changes (§2.1) — a proposal may
target this plan; the owner, or the coordinating agent after owner
ratification, applies the delta; **the agent implementing a stage never
edits that stage's own text** (a self-graded goalpost move is the failure
mode, not a workflow).

## 1. What this document is

The spec says what v0 *is*. This document says how v0 gets built without
the spec silently rotting — and which process survives past v0. Handing a
1400-line spec to coding agents fails in documented, recurring ways
(research doc §1). Mapping of those failure modes to this plan's
mechanisms, stated honestly including the gaps:

| Failure mode (research §1) | Mechanism here | Enforcement level |
|---|---|---|
| 1.1 Silent spec↔code drift | Amendment gate (§2.1) + per-stage critic item (iv) (§5) | Process + review |
| 1.2 Silent goal drift within one run | Stage/sub-batch sizing (§4) — bounded units shorten the unobserved window; DoD checked at each close | Partial by construction; the residual (drift inside one bounded unit) is §8 item (e) |
| 1.3 Verification theater | Exit-code DoDs; **critics re-run the commands themselves**; evidence links required in the ledger (§5) | Machine (commands) + review |
| 1.4 Context loss across sessions | Progress ledger with a SessionStart hook carrier (§6) | Mechanical (hook) + convention |
| 1.5 Mid-task requirement change | Changes land as amendments between stages, never mid-stage silently (§2.1); stage re-opened via inventory re-walk (§3) | Process |
| 1.6 Destructive/uncontained action | Bootstrap-window containment: tests run in temp dirs (testscript by design), implementation sessions run under the repo's permission settings, CI on a clean checkout is the only "done" authority; deletion discipline per repo conventions | Partial; largely environment-level — stated, not claimed solved |
| 1.7 Underspecification | The spec's 8 critic revisions + G0 inventory review surface remaining ambiguity as amendment proposals, not code-side guesses | Process |

Two layers, deliberately split:

| Layer | What it covers | Why |
|---|---|---|
| **Adopted, ready-made** | Spec *change management* (amendment proposals, review, merge, archive) — OpenSpec; acceptance-criteria *notation* — EARS; persistence-model *vocabulary* — Spec Kit's naming, as prose | Maintained, Claude Code-native; re-inventing an amendment workflow is undifferentiated work |
| **Hand-rolled, disposable** | Spec-id ⇄ stage ⇄ verification **traceability**, the staged plan, the per-stage critic protocol, the bootstrap progress ledger | No existing tool fits without rewriting the spec's format (see §7); and adopted *trackers* would compete with the tracker this project builds — these artifacts are shaped to be `import`-ed into selftracked at switchover |

## 2. Adopted tooling (lifecycle layer)

### 2.1 OpenSpec — spec change management

Facts verified 2026-07-18 (research doc, Addendum): `Fission-AI/OpenSpec`
v1.6.0, MIT, actively developed, generated Claude Code skills + `/opsx:*`
commands; its brownfield guidance explicitly keeps an existing spec doc
authoritative and unconverted.

Adopted contract for this repo — and, stated plainly, the **adopted
surface is a fraction of the tool**: the proposal/review/archive workflow,
the slash-command ergonomics, and the change-artifact trail. The
`specs/`-tree delta-merge engine is unused in v0 (D-EP3), and two shipped
features are deliberately NOT used:

- `/opsx:verify` — a single-pass self-review that emits recommendations;
  it violates this project's critic constraints (fresh-context,
  findings-only, no fixes proposed by the reviewer) and does not replace
  the §5 protocol. It MAY run as a cheap pre-critic smoke check; its
  output is never a gate.
- Per-change `tasks.md` checkbox tracking — a change-scoped micro-tracker.
  Project tracking never lives there (v0: the ledger §6; post-S10: the
  selftracked tracker itself). Each change's `tasks.md` holds a single
  pointer line to the governing stage/story. This is the stated exemption
  to the tracker-collision principle (§7): the collision rule bars
  *project backlog* tools, and OpenSpec's checklist is confined to one
  amendment's internals — anything more migrates.

The flow: every deviation discovered during implementation becomes an
OpenSpec change proposal (`openspec/changes/<name>/` — proposal + spec
delta) **before the deviating code merges**. A verbal owner approval in a
working session is not an amendment: the proposal is written first, the
code lands after — "approved in chat" has the same standing the parent
project's documented lesson assigns a decision living only in chat: none.
The owner reviews;
on acceptance the delta is applied to `docs/v0-spec.md` by hand (revision
bump, history line), and the change is archived as the durable record. Two
verification duties attach: the critic pass on an amendment diffs the
applied spec text against the archived proposal (hand-application is
itself a "declared done" surface — §8); and the §3 inventory re-walk runs
on the affected sections.

Maintenance-risk honesty: OpenSpec's contribution history is dominated by
a single author (Addendum). Accepted because the exit cost is one
directory and the flow degrades gracefully to bare-files delta proposals
(the research's original recommendation) with no data loss.

### 2.2 EARS — acceptance-criteria notation

Adopted as a writing discipline (Mavin et al., RE'09). New criteria —
stage DoD items, `criteria add` texts during dogfooding — use the five
EARS patterns where a conditional/temporal shape exists. Existing spec
text is not retrofitted; EARS applies at next-touch. Honesty note
(Addendum): its benefit is proven for human review clarity; no primary
source shows agent-side execution gains — adopted at zero tooling cost,
not on an overclaim.

### 2.3 Spec Kit vocabulary

We run the **Living Spec** persistence model (Spec Kit's taxonomy, adopted
as naming only): the spec is edited first; downstream artifacts regenerate
from it. Spec Kit itself is not installed (its scaffolding owns per-feature
spec authoring — wrong shape for one existing house-style file). BMAD and
Taskmaster are rejected outright: both embed a project-backlog layer that
collides with selftracked itself.

## 3. Traceability inventory (artifact 1)

`docs/v0-traceability-inventory.md` — tracked; one row per normative
obligation:

```
INV-### | anchors (one or more: § / R# / verb / trigger — cross-section
obligations list every anchor) | one-line statement | kind (schema-gate /
verb-contract / catalog-convention / verify-rule / format / process /
deliverable / stated-limitation / deferral-boundary) | closing stage |
verification (command or fixture name) | status | evidence (CI run / log
link once verified)
```

Rules:

1. **G0 (gate zero): the full inventory is built and owner-reviewed before
   ANY stage starts — including S0** (S0 closes real spec rows: toolchain
   pin, `go fix` gate, LICENSE/NOTICE intake). Extraction is delegated
   section-wise to fresh-context extractors; the coordinating agent
   verifies each section; a critic pass checks for missed obligations.
   Extraction completeness is review-checked, not machine-checked — named
   in §8.
2. **Total accounting:** every row lands in exactly one stage's closure
   list, or is `deferred` with its §17 anchor, or is a `stated-limitation`
   closed by the stage that ships the limited feature (DoD: the limitation
   is documented and, where cheap, demonstrated by a boundary fixture —
   limitations are shipped, not fixed, and never silently dropped). A row
   in no bucket is a defect that blocks all stages. This rule is
   machine-checkable (a CI script diffs inventory stage assignments
   against stage closure lists); the script ships with the inventory at G0
   and wires into CI at S0, when CI first exists.
3. **Re-walk on amendment:** after every archived change, the affected
   sections are re-walked; touched rows lose `verified` status until
   re-run; stages whose closure lists changed re-open. An amendment
   touching no row is itself flagged.
4. Rows are shaped for the S10 import: `verification` becomes a runnable
   `$`-criterion, `status`/`evidence` become criteria evidence, stage
   membership becomes story structure.
5. **Scope boundary** (D-EP5): the inventory walks the *specification*. A
   stage's definition of done may also contain plan-native work — artifacts
   this plan itself created, such as retiring the ledger and the inventory
   at S10 — which no row can carry by construction. Those items are closed
   by the stage's review pass (§5), not by the accounting rule, and a
   stage's row count understates its scope wherever that applies. Today
   S10 is the only such stage; a future stage that acquires plan-native
   work says so in its own row.

Expected size, calibrated against the spec's actual density (verb cells
bundle 3–15 fixturable clauses; 14 R-rules with sub-clauses; ~20 schema
gates): **roughly 250–300 rows** at obligation-cluster granularity with
rules/gates/refusals kept row-per-item (D-EP2).

## 4. Staged plan (artifact 2)

Stages are sized for verifiability. Oversized surfaces are split into
lettered sub-batches — **each sub-batch gets its own verification run and
critic pass** (a fresh-context review of S5 or S8 as one unit was judged
infeasible in round 1). Ordering follows build dependencies; two
stub→real ladders are explicit: the §8.6 version gate (stub S2 → real
S11) and the §8.4 sidecar procedure (core S5a → full non-migration matrix
S8b → migration branches S11, where a real migration first exists to
produce those states).

| Stage | Scope (closes — inventory refines) | DoD sketch & verification |
|---|---|---|
| **S0** | Repo bootstrap: go.mod (toolchain pin §3.2), Makefile catalog, golangci config, CI skeleton (build/vet/test, `go fix -diff` both-checks, govulncheck), README skeleton, LICENSE/NOTICE intake, and a minimal package carrying the binary's build identity so the build/vet/test/lint gates operate on real code rather than an empty match set (D-EP9) | `make build lint test fix-check` exit 0 on a clean checkout — **interim evidence, sufficient to close S0** (D-EP8); the CI workflow is authored here but the three-platform matrix is a precondition of the FIRST PUSH, not of this closure; the §16 `go fix -diff` exit-code re-verification runs here (a red-input probe confirming exit 1 on the pinned toolchain) |
| **S1a** | The schema as text: embedded DDL v1 (§5 verbatim), database open/create, PRAGMA choreography, `meta` seeding, views. The SQLite driver becomes a dependency here, so its pins and the licensing intake land with it | A fresh database contains every object §5 declares — object-by-object comparison against the compiled-in DDL; `meta` carries its seeded system rows |
| **S1b** | The gates: every CHECK, uniqueness and referential constraint, and every trigger | Red fixture per gate — a gate that cannot be shown failing is decoration (§7); the §5 enforcement map is the checklist, not the illustrative shortlist |
| **S1c** | Driver behaviour: the §16 re-verification items the specification refuses to assume — Serialize/Deserialize roundtrip on the full schema, extended vs primary result codes, RETURNING via Query, the recursive_triggers/REPLACE regression | Each re-verification item passes as a test against the real schema on the pinned driver; a probe that cannot run fails rather than reporting success |
| **S2** | Dispatcher: verb registry, §3.2 parsing obligations (a)–(c) plus the leading-dash shape rule, JSON errors, §6.1 exit contract, version-gate **stub** | testscript usage-error matrix: exact exit codes, JSON-only output, `-h` behavior |
| **S3** | Serializer + `dump` (§8.1); §16 cross-OS byte-equality | Golden dump `cmp`; determinism matrix incl. Windows; serializer mutation tests |
| **S4** | `load` (§8.5) + whitelist-parser fuzzing; VACUUM/rename flow re-verification; load-side skip-marker conversion (the marker's writer verb arrives at S8b — the fixture writes the marker file by hand) | dump→load→dump byte-equal; fuzz target in CI; red fixtures (PRAGMA smuggling, missing meta rows, forged boundary); marker-conversion fixture |
| **S5a** | Task-lifecycle verbs: create/show/list/ready/set-status/reopen/park/unpark/edit + events + write-verb pipeline with **§8.4 core** (match / crash-residue heal / external-change refuse / missing sidecar) | testscript per verb: happy path + every documented refusal; §14 no-identity-leak fixture (verbs run with sentinel HOME/USER — outputs clean) |
| **S5b** | Relation/artifact/dictionary verbs: rel, link/unlink (incl. archive/unarchive), paths (incl. `move --with-files`), config, log, **stale** | Same per-verb standard; `stale` ordering fixture; containment refusal fixtures (`..` relpath). Stated split: `epic:SLUG` link targets and the epic-linked `stale` path re-verify at S6 close, when epics exist |
| **S6** | Epic/story/worklog/criteria verbs (unblock --resolution, IN-REVIEW exit, corrections, `epic close`, `criteria check`) | §12 trace T1–T7, T9–T10 replayed end-to-end (T8 needs `prime` and joins the replay at S8c — stated deferral); exit codes + JSON shapes byte-asserted; message text via implementation-owned golden files — §6.3 prose is illustrative, §6.1 is the contract; close-blocker and criteria-regression fixtures |
| **S7** | `verify`: R1 (checks 1–2), R2–R13, R15 + `--fast` partition (R14 / R1 check 3 — STATE.md byte-equals its render — rides S8c with its renderer, amendment `r14-rides-its-renderer-at-s8c`) | Red fixture per R-rule + R11 variant table; `--fast` rule set asserted by fixture |
| **S8a** | `init` full: scaffold, seeded roots, class READMEs, ADR template, PROMPT.md (three authoring rules), STATE.md, **AGENTS.md, `.claude/` files (§11.2 rule + §11.3 skill as shipped content)**, `.gitignore`, meta seeding | Golden-file fixtures for every generated artifact; fresh `init` ⇒ `verify` green |
| **S8b** | Hooks + §8.4 full: generated pre/post-commit, chaining-recipe detection (all three incumbent states), `gate skip-mark`, sidecar divergence matrix | testscript with real git repos; the §8.4 non-migration matrix: (1) sidecar match, (2) mismatch + regenerate-match heal, (3) mismatch + differ refuse, (4) missing sidecar — each a fixture (branches (5) DB-version-ahead residue heal and (6) migration-in-externally-changed-state land at S11 with the real gate); `commit -n` backstop fixture. POSIX-runner scope per spec §9's stated limitation |
| **S8c** | `state`, `prime` (§11.1 contract), SessionStart chain; R1 check 3 / R14 (STATE.md byte-equals its render) landed with the renderer (amendment `r14-rides-its-renderer-at-s8c`) | prime golden JSON (caps, totals, payload-shape rule); three-branch SessionStart fixture; the R14 red fixture (STATE.md tampered ⇒ verify red) |
| **S9** | `import` + `--legacy` (§6.2/§10) | Synthetic legacy corpus round-trip → `verify` green → golden dump; date-priority matrix incl. calendar-day warn; source-map determinism fixture |
| **S10** | **Dogfood switchover**: inventory + ledger imported into `.selftracked/` as the first epic; ledger deleted; self-host begins. Most of this scope is plan-native (§3 rule 5) and therefore carries no inventory rows — the single row this stage owns understates it | `selftracked verify` green on this repo's live tracker; the import IS the importer's first rehearsal; the inventory file retires here too (kept in git history — §9). Placement is consistent with spec §16 as written: its sentence names both `init` and `import` as the switchover's ingredients |
| **S11** | §8.6 version gate real: two-comparison gate, forward-only refusals, escalation choreography (v1: refusal paths; golden migration corpus starts at schema v2, per spec §16) + §8.4 migration branches (5)–(6) | Refusal fixtures at load and prime (typed field); escalation-race/busy fixtures; §8.4 branch-(5)/(6) fixtures driven by a synthetic version bump |
| **S12** | Pilot ladder rungs 3–4 of D13's four (testscript synthetics → self-host — both closed by S0–S10 — → **gitignored disposable-clone import rehearsal** → colocated live install) + remaining §16 deliverables: README final, CONTRIBUTING (DCO + AI clause), the generic migration guide | Rehearsal: repeated from-scratch imports of the disposable clone corpus, `verify` green each round, importer defects filed as stories; colocated install: host-gates-stay-authoritative checklist executed; migration guide walked against the S9 fixture corpus; docs link-check |

Cross-stage rule: nothing later silently re-opens an earlier closure — a
regression is a new inventory row plus, if the spec was wrong, an
amendment.

## 5. Per-stage protocol (artifact 3)

The critic protocol, defined here (project convention): reviewers run in
fresh contexts, read-only on the repo, any execution sandboxed to temp
dirs; they report findings only — never fixes; findings are adjudicated
refute-by-default by the coordinating agent, with two mandatory owner
ratifications: any accepted deviation from spec/plan, and the adjudication
of any privacy/security-class finding. Three review rounds without
convergence escalates to the owner. Critic feasibility is a sizing input:
sub-batches (§4) exist so one review fits one context.

A stage (or sub-batch) **opens** by re-reading its own inventory rows, on
two axes:

- **Content** (D-EP4) — against the spec sections the rows anchor:
  statements that drifted from an amended spec, or that were miscast during
  the G0 walk, are corrected before any code is written, and a row whose
  `verification` is still a placeholder name is turned into a real command
  or fixture.
- **Placement** (D-EP6) — against this plan's own scope and DoD text for
  the stage: each row must be *executable by this stage*. A row whose
  obligation belongs elsewhere (a serializer check filed before the
  serializer exists, a `go.mod` pin filed after the module is written) is
  handed back and moved. A row that no stage can execute where it stands is
  a placement defect, fixed by moving the row, never by weakening it. The
  accounting script cannot see this class: a stage id that exists is a stage
  id that passes.

**The opening record** (D-EP13). The re-read's output is a committed
artifact — `docs/stage-openings/<stage>.md`, the stage id lowercased —
committed before the stage's (or sub-batch's) first implementation commit,
where an implementation commit is any commit touching files the stage's
verification commands examine. One line per inventory row: the content
verdict (`ok`, or what was corrected), the placement verdict (`ok`, or
where the row moved and why), and the concrete command or fixture the row's
placeholder verification name resolved to. The record is a point-in-time
trace of the open: the inventory stays authoritative for a row's current
state, and a row handed to the stage after its open enters the close
review through items (i)–(ii) below, not through the record. S0 and S1a
both reached their close with rows their stage could not execute, because
the re-read was the one step in this protocol whose omission left no
trace — an exhaustive open and a skipped one produced the same visible
state. The record does not make the verdicts true; it makes the check's
execution observable, and gives the close review a document to sample
instead of an absence to prove.

G0 verified fidelity by sampling, not exhaustively — this is where the
exhaustive check happens, at the moment the rows are about to be used and at
a size one reviewer can hold (9–65 rows). A correction that changes what the
spec *means*, rather than what the row *says*, is an amendment, not an edit.

**Proportionality** (D-EP7). The review is sized to the change class, and the
tier is recorded with the work item **before** the work starts — a tier
lowered afterwards is a deviation and needs an amendment. Uncertainty
resolves upward.

- **LIGHT** — verification commands only, no reviewer. For changes that
  cannot alter behaviour or a published claim: formatting, a typo, a comment.
- **MEDIUM** — verification commands plus one fresh-context reviewer, without
  the full checklist. For a bounded change inside one stage that touches no
  inventory row's meaning.
- **FULL** — everything below. Mandatory for a stage or sub-batch close, for
  anything touching the spec, this plan, the inventory or a published
  document, and for anything a reviewer has already found a defect in once.

A process whose lightest gear is heavy is not run carefully; it is run
nominally or skipped with a reason invented at the time. Named tiers make
that choice explicit and reviewable instead of silent.

**The publication boundary** (D-EP11). Evidence recorded before the first
push names commits a squash will delete, and a squash is many-to-one, so
rewriting those references preserves nothing. Before the first push the gates
run once against the squashed tree and every `verified-by-command` row is
re-stamped with that commit; a row whose check now fails returns to `planned`
and re-opens its stage. A row proven at one stage and quietly broken at a
later one is what this single re-run catches, and nothing else in the process
would.

A stage (or sub-batch) is "done" only when, in order:

1. Its verification commands exit 0 **on a clean checkout or CI** — and
   the ledger row links the evidence (CI run URL or committed log
   artifact), not a prose claim.
2. The critic pass runs, and the critic **re-executes the stage's
   verification commands itself** (they are exit-code commands precisely
   so that re-running is cheap) — reading the ledger's claim is not
   verification. It also checks: (i) every closed inventory row has a
   change and a passing check; (ii) scope vs the DoD text; (iii) code
   paths with no inventory row (added scope needs an amendment);
   (iv) deviations without an archived change — including in **generated
   artifacts** (PROMPT.md/AGENTS.md text count as surfaces);
   (v) a deviation sweep of the stage diff against the spec sections the
   stage closes; (vi) the opening record (D-EP13) exists, predates the
   stage's first implementation commit, covers every row the stage owned
   at open, and its verdicts match what actually happened — moved rows
   moved, corrected rows corrected.
3. Inventory rows flip to `verified-by-command` with the evidence link;
   the ledger records the run.

Owner cadence (D-EP14): amendment proposals are still written and
committed FIRST — the archived artifact with its reasoning is the record
— but the coordinating agent applies them without waiting for
ratification; the owner reviews after the fact and may revert any
application, which re-parks the affected work. A spec-contradicting
blocker still files immediately. Privacy- and security-class findings
still escalate BEFORE action — that mandatory ratification is unchanged.
This standing pre-authorization covers the v0 bootstrap window only
(§9's steady state keeps owner review in the loop).

## 6. Progress ledger (artifact 4)

`docs/v0-progress.md` — tracked, flat, disposable at S10. One row per
stage/sub-batch: status, last verification run (command + date + result +
evidence link), open questions, next unstarted unit; plus an amendments
log (change id → spec revision). Mechanical carrier for the bootstrap
window: the repo's Claude Code configuration adds a SessionStart hook that
prints the ledger (the same pattern the product itself ships in §11 — the
bootstrap gets the tool's own medicine), so "read it first" does not
depend on agent discipline; "update it last" remains convention — a stale
ledger surfaces at the NEXT session's start, when the hook prints a state
that visibly lags reality (§8 names this residual; no critic item checks
it directly). Where evidence links exist in both artifacts, the inventory
row is authoritative and the ledger mirrors the latest run.

## 7. Division of labor — capability by carrier

| Capability | Carrier | Ready-made alternative — verdict |
|---|---|---|
| Amendment propose/review/merge/archive | **OpenSpec** (adopted surface: §2.1) | Bare-files convention — fallback, retained as exit path |
| Acceptance-criteria language | **EARS** (notation) | Free-form — kept for existing text |
| Spec-id ⇄ stage ⇄ check traceability | **Hand-rolled inventory** | RTM tools exist (Sphinx-Needs, StrictDoc, Doorstop — Addendum) but all require rewriting the spec into their format; rejected on migration cost against an untouchable spec, **not on nonexistence** |
| Stage decomposition & DoD | **Hand-rolled plan (§4)** | Spec Kit/BMAD decompose their OWN artifacts, not an existing house-style spec |
| Project progress tracking | **Ledger → selftracked at S10** | Any adopted backlog tool collides with the product; OpenSpec's per-change `tasks.md` is the stated, confined exemption (§2.1) |
| Stage-gate review | **§5 critic protocol** | `/opsx:verify` exists but is single-pass, self-reviewing, recommendation-emitting — usable as smoke, never as gate (§2.1) |

## 8. Enforcement honesty

Machine-checked: verification commands; the inventory total-accounting
diff (CI, from G0); critic re-execution of DoD commands. Review-checked
(named residuals, weakest first): (a) a session writing code with no stage
at all — mitigated only by the SessionStart ledger hook and the next
critic pass; (b) hand-application of an accepted amendment to the spec —
mitigated by the critic diff against the archived proposal; (c) G0
extraction completeness — critic-checked, not machine-checked; (c2) row
**placement** — the accounting script validates that a row's stage id
exists, never that the obligation can be executed there; caught only by the
stage-open placement check (§5, D-EP6) or a review that diffs assignments
against this plan's stage text — since D-EP13 the check's *execution* is
visible (a committed opening record, one line per row), but verdict quality
is not: an `ok` written without reading leaves the same record as one
written after it, and only the close review's sampling of the record
catches that; (c3) **review tier selection** — a judgement
the accounting gate cannot see; under-selection is the tier system's own
failure mode, guarded only by recording the tier before the work and
resolving uncertainty upward (§5, D-EP7); (c5) **pre-push evidence is provisional** — it names commits that will not
survive the squash, so a green ledger before publication is a weaker claim
than the same ledger after (D-EP11); (c4) **interim evidence** — a stage closed on a local run is done on one
machine's word; the ledger names the grade, and the first push is where
every such claim meets a real matrix at once (D-EP8); (d) ledger
"update it last" — convention, surfaced only at the next session's start;
(e) goal drift INSIDE one bounded sub-batch — sizing shortens the
unobserved window, nothing eliminates it; (f) an unresponsive owner after
escalation or during proposal review — deviating work stalls by design
(parked, visible in the ledger), and that stall is accepted, not solved.
Failure mode 1.6 (destructive action)
remains environment-level (permissions, temp-dir tests, clean-checkout
CI) — this plan reduces exposure, it does not claim containment.

## 9. After S10 (the two-layer steady state)

Spec changes continue in OpenSpec: proposal → owner review → hand-applied
delta (critic-diffed) → archive. Work tracking lives in selftracked:
implementation of an accepted amendment is a story (or epic) in
`.selftracked/`, its acceptance checks are `criteria` rows, and the §3
re-walk duty transfers to the amendment's applier as part of the same
story's DoD — the inventory FILE is retired at S10 (kept in git history;
its rows live on as imported stories/criteria), so post-S10 re-walks
operate on tracker records, not on the markdown table. OpenSpec change
directories archive as usual and never hold work tracking (§2.1). The
progress ledger is deleted at S10; the amendments log moves into the
tracker as events/worklog records of the governing epic. Opening records
(`docs/stage-openings/`, D-EP13) freeze in git history as a procedural
trace of the bootstrap — nothing from them migrates into `.selftracked/`,
because they document how stages were opened, not tracker state. This plan
document itself freezes at S10 close as the bootstrap's historical record
— its stage table and artifact sections are not maintained further (they
describe retired artifacts by then); only §2's lifecycle contract remains
live, amendable through its own flow.

## 10. Owner decisions

All three decided by the owner (2026-07-18):

- **D-EP1 [DECIDED]**: adopt OpenSpec per §2.1 (npm tooling + generated
  commands in the repo), with the §2.1 usage boundaries and the stated
  bus-factor/fallback.
- **D-EP2 [DECIDED]**: hybrid inventory granularity — obligation-cluster
  rows by default; verify-rules, schema gates, and per-verb documented
  refusals always row-per-item (they map 1:1 to red fixtures). Expected
  total **250–300 rows as a floor, possibly above 300**; an inventory
  that comes in low signals a too-coarse walk, not a small spec.
- **D-EP3 [DECIDED]**: the spec is NOT split into OpenSpec domain
  `specs/` in v0; revisit after the first post-v0 feature epic.

Decided 2026-07-19, on the amendment `plan-accounting-scope` filed at G0
close:

- **D-EP4 [DECIDED]**: the inventory is ratified as the control artifact
  without a full up-front line-by-line audit; instead each stage re-reads
  its own rows against the spec when it opens (§5). Rationale: an audit of
  all 545 rows now spends equal effort on rows that will not be used for
  months and may be superseded by amendments before they are; the staged
  check spends the same effort where it pays.
- **D-EP5 [DECIDED]**: plan-native DoD items are outside the
  total-accounting rule and are closed by the stage's review pass (§3 rule
  5) — rather than extending the inventory to walk this plan as well, which
  would create a second accounting for artifacts that self-liquidate at S10.
- **D-EP6 [DECIDED]** (amendment `stage-open-plan-crosscheck`, 2026-07-19):
  the stage-open re-read checks **placement as well as content** — each row
  must be executable by the stage that owns it, judged against this plan's
  own scope text (§5). Raised by a real defect: three of spec §16's
  re-verification items sat on a stage that could not run them, correctly
  worded and wrongly placed, past three review passes that were each looking
  at something else.
- **D-EP7 [DECIDED]** (amendment `review-proportionality-tiers`, 2026-07-19):
  the review is sized to the change class — LIGHT / MEDIUM / FULL (§5) — with
  the tier recorded in the ledger's Tier column before the work starts and
  uncertainty resolving upward. Rationale: a procedure whose lightest gear is
  heavy gets run nominally or skipped with a reason invented at the time,
  which is how a session reaches §8's weakest-link state. Stated gap: nothing
  separates the chooser from the implementer, so under-selection is guarded
  by disclosure, not by mechanism (§8 (c3)).
- **D-EP8 [DECIDED]** (amendment `local-commits-and-interim-evidence`,
  2026-07-19): a local gate run is interim evidence sufficient to close a
  stage, with the CI matrix moved to the first push; and committing is
  separated from publishing — local commits are free, pushing stays
  owner-only. Rationale: CI-on-close fused a module skeleton to a
  publication decision, and approval-to-commit was buying nothing that the
  push prohibition did not already buy, while costing every restore point.
- **D-EP9 [DECIDED, ratified]** (amendment `s0-minimal-package`, 2026-07-19): S0 ships
  a minimal package so its gates are not vacuous. Raised by the S0 close
  review, which found the package as unnamed scope; with no Go source at all,
  `make build lint test` would pass by examining nothing. A gate that passes
  over nothing produces evidence without content.
- **D-EP10 [DECIDED]** (amendment `split-s1`, 2026-07-19): S1 splits into
  S1a (the schema as text), S1b (the gates), S1c (the driver). It held 129
  rows — more than either group already split for exceeding what one
  reviewer can hold. S0's close is the evidence: eight rows still produced
  six defects, four of them inside the stage's own gates.
- **D-EP9 [RATIFIED by the owner, 2026-07-19]** — S0 ships a minimal package
  so its gates are not vacuous.
- **D-EP11 [DECIDED]** (amendment `evidence-across-a-squash`, 2026-07-19):
  evidence recorded before the first push is provisional; the gates run once
  against the squashed tree at the publication boundary and every verified
  row is re-stamped against the commit that will actually be published.
- **D-EP12 [DECIDED]**: the `as of dump <sha12>` anchor ships in v0 as a
  convention with its validator deferred (§17). Before publication there is
  no durable commit for an anchor to name, so a gate built now would guard a
  period in which its subject cannot hold still.
- **D-EP14 [DECIDED, ratified]** (amendment
  `pre-authorized-amendment-cadence`, 2026-07-19): amendments apply under
  the owner's standing pre-authorization — proposal-first stands, waiting
  does not; owner review moves after the fact with revert power;
  privacy/security escalation stays pre-action. Granted in the same
  message that ratified the link-tables amendment.
- **D-EP13 [DECIDED, ratified]** (amendment `stage-open-record`,
  2026-07-19): the stage-open re-read produces a committed opening record —
  `docs/stage-openings/<stage>.md`, one verdict line per row, written before
  the stage's first implementation commit — and the close review verifies
  the record against what actually happened. Raised after S0 and S1a both
  reached their close with rows their stage could not execute: the re-read
  was the only step in the protocol whose omission left no trace, and twice
  it was omitted or truncated exactly where it mattered.
