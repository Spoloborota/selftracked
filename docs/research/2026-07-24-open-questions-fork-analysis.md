# Open-questions fork analysis — pre-decision critic round

Date: 2026-07-24. Status: **reviewed** — five fresh critics ran on this
document (spec-fidelity, code correctness, architecture, agent-flow/
hallucination-surface, governance); the adjudication appendix at the end
records every accepted and refuted finding and **supersedes the in-body
recommendations where they differ** (notably F1, reversed α→β). The body is
kept as reviewed, per the correction-on-record convention. All rulings are
the owner's.

Owner input on record for this round (2026-07-24, recorded in English by
meaning per the language convention): *prefer the architecturally sound
option over the patch; a workflow that grows extra manual steps or misleading
surfaces raises the hallucination probability of the agents that consume it,
and that cost counts.*

Scope: the six items open on the ledger after the pre-S10 bugfix batch —
ledger open question 2 (`pause` orphan), the stale open question 3
(link-tables), S9 escalations E1–E3, and the two spec-wording notes
(§11.1 `load`, §9 rc-triage).

## Part 1 — dissolved under research (findings, not forks)

### D1. Ledger open question 3 (link-tables) is stale

The amendment `link-tables-are-relations-not-history` was accepted
2026-07-19 (spec rev 3.14) and applied at S5b; today's DDL carries
no-delete triggers on entity tables only — none on
`task_links`/`task_artifacts`/`epic_artifacts`
(`internal/schema/ddl.sql:243-255` lists the entity-table set). The
"open question" entry describes a proposal that was ratified months of
work ago. Residue: mark the ledger entry resolved. No decision content
remains.

### D2. E1 (does an explicit `date` field require `--legacy`?) — the spec answers it

Spec §6.2's `import` row gives `--legacy` a closed relaxation list —
synthesized timestamps, `commits='legacy: …'`, terminal-state INSERTs —
and places the explicit date field as ordinary source (2) in the
git-first priority chain for *any* import. The restrictive reading
(explicit date ⇒ `--legacy`) was empirically tried during S9 (the first
cut of fix A5), broke legitimate non-legacy batch creation, and was
reverted after the verification re-critic caught it (RC-1,
`docs/research/2026-07-21-s9-import-critic-round.md`). The ambiguity
lives only in INV-056's parenthetical — "its backfilled timestamps
(git-derived or explicit-field, §6.2) are events-marked" — which can be
misread as claiming the explicit field is a legacy feature. The
inventory is a derived artifact; the spec is authoritative.

Residue: a one-line clarifying edit to INV-056's wording (no meaning
change — the row's obligation and fixture stay as shipped). Proposed
reading: the parenthetical enumerates which timestamp *sources* are
events-marked when imported, not which require `--legacy`.

### D3. E2 (calendar-day disagreement recorded in `note`) — INV-261 is satisfied

INV-261 demands: warn, git wins, both values recorded. Shipped: the
git date lands in the `date` column; the explicit value survives
verbatim in the worklog note with a **stable, greppable format** —
`[explicit date YYYY-MM-DD disagreed; git YYYY-MM-DD used]`
(`internal/verb/import_dates.go:294-297`) — plus the import-time
warning. Both values are in the database and the dump; the fixed
bracket pattern is machine-findable by substring. The structured
alternative (encoding disagreement into the per-epic source map) would
reopen S9's frozen deterministic map format for marginal audit value.
Residue: none. Recommendation: accept as shipped.

### D4. E3 (md-table cannot express a bundled increment) — an authoring rule, not a fork

Already carried into the S10 opening record as a watch-item with its
answer: the ledger corpus splits bundled increments row-per-increment,
or is authored in JSON — the format whose split path INV-444 actually
exercises. Widening the md-table grammar would be S9 scope reopened by
amendment, and nothing needs it. Residue: none.

## Part 2 — surviving forks

### F1. `pause` can orphan an IN-PROGRESS story into a non-active epic

**The defect surface.** `epicTransition` — shared by `epic pause` and
`epic activate` — is a bare status UPDATE with no preconditions
(`internal/verb/epics.go:122-124`); `epic dissolve` refuses while a
story is IN-PROGRESS via `dissolveBlockers`
(`internal/verb/epics.go:146-151`; spec §6.4). So an epic can be
PAUSED around a story that is actively being worked. `prime` is
spec-conformant: §11.1 defines `sprint_goals[]` as "every IN-PROGRESS
story", no epic-status qualifier — so the paused epic's story appears
as a sprint goal while `epics_active[]` omits the epic. Raised by the
S8c data/semantics critic (2026-07-20), confirmed by hand.

**Stakes.** No data loss, no unloadable state. But from S10 on,
`prime` is the session-start context of every agent session: a sprint
goal whose epic is in no active list is a contradiction the reader
must resolve — resume the paused epic's work? report it? — exactly the
kind of ambiguous surface that invites a wrong guess.

**Options.**

- **(α) `pause` refuses while a story is IN-PROGRESS** — symmetric
  with `dissolve`. To pause an epic mid-story: `story block --why
  "epic paused"` first (honest recorded state; the story moves to
  `blocked[]`, no orphan). Cost: a spec §6.4 amendment (one clause), a
  guard in `epicTransition` shaped like `dissolveBlockers`, one
  fixture, one inventory row; a review cycle touching the closed S6
  surface. Flow cost: pausing an epic with in-flight work becomes two
  explicit commands instead of one.
- **(β) declare the orphan intended** — a spec note ("a paused epic's
  IN-PROGRESS story stays in `sprint_goals[]` — nothing hides"), no
  code. Cost: one prose amendment. Flow cost: the contradictory
  `prime` state persists forever, documented; every consumer must
  carry the special case.
- **(γ) scope `sprint_goals[]` to active epics** — already refuted at
  the S8c close: a §11.1 deviation that would *hide* in-flight work
  (against §2 nothing-hides). Listed as considered-and-rejected.

**Recommendation: (α).** PAUSED means "not being worked"; an
IN-PROGRESS story contradicts it; the state machine should refuse the
contradiction rather than emit it for every downstream reader to
re-interpret. `dissolve` already establishes the precedent and the
code shape. Under the owner's stated preference this is also the
architectural option: (β) documents a permanently ambiguous surface
into the contract; (α) removes the ambiguity at the single write site.

### F2. §11.1 (and §9 hook text) misdescribe `load`

**The defect surface.** Spec §11.1 says the SessionStart fallback
`load` "fast-forwards a missing/behind DB and refuses a divergent one"
(line 1176); §9's post-pull advice says "`selftracked load`
(fast-forward the DB to the pulled dump)" (line 873). The built `load`
refuses *any* existing DB without `--force`; a behind-DB is handled by
`prime`'s `dump_divergence` flag and never reaches `load`. The chain
is correct end-to-end (tested at S8c); the prose describes machinery
that does not exist. Raised by the S8c spec-fidelity critic.

**Stakes.** Zero behavioral risk today. Deferred risk: S11 (version
gate + migration branches) works directly on `load`'s behavior — an
implementer reading §11.1's fast-forward claim as normative would
build against a phantom, or "fix" `load` to match the prose and open a
second sync path beside the §8.4 matrix.

**Options.**

- **(α) wording amendment** — both prose sites rewritten to the built
  behavior (`load` fills a *missing* DB; a behind DB is surfaced by
  `prime`'s flag; divergent state errors). No code, no inventory
  motion.
- **(β) make `load` actually fast-forward a behind DB** — code on the
  closed S4/S8c surfaces, new rows, full review; duplicates what the
  chain already does via `prime`; implicitly rejected by the S8c
  adjudication ("the chain relies on `prime`'s flag by design").
- **(γ) leave it** — a normative contract that misstates its own
  fallback, entering S11 which builds on exactly that contract.

**Recommendation: (α)**, before S11 opens.

### F3. §9 pre-commit rc-triage labels a signal death as bypassable RED

**The defect surface.** The verbatim generated hook (spec
§9:1097-1102): `rc=2` → "verify could not run … not bypassable";
`rc≠0` otherwise → "verify RED … SELFTRACKED_SKIP=1 bypasses ONCE".
A verify killed by a signal (rc = 130/137/143) takes the RED branch:
a false diagnosis with a bypass hint attached. Raised by the S8b
shell-robustness critic; the script is quoted verbatim in the spec, so
any change is a spec amendment plus the scaffold template plus its
fixtures.

**Stakes.** The lowest of the three: both branches exit 1 and abort
the commit either way — the defect is purely the diagnostic text.
Worst case: an operator whose verify died under Ctrl-C reads "RED",
believes the tracker is broken, and burns the one recorded skip on a
healthy tracker. For an agent operator the miscue is worse than for a
human: the message *names* the wrong situation, and agents act on
messages.

**Options.**

- **(α) accept as-is** — the ledger note stands; zero cost; the
  misleading branch stays and self-hosting puts agents behind it.
- **(β) amend the script** — route `rc` outside {0,1,2} to the
  could-not-run branch (treat an interrupted verify as "did not run",
  which is what it is). Three lines in the spec's verbatim script +
  the scaffold template + a fixture asserting the branch; a review
  cycle on the closed S8b surface.

**Recommendation — revised under the owner's stated preference: (β).**
The earlier lean toward (α) priced only the review cost. The owner's
criterion — surfaces that misinform agents are architecture, not
cosmetics — flips it: the message is the interface of the gate, v0's
operators are agents, and "RED + here is the bypass" for a not-RED
state is a wrong-action prompt built into the flow. The change is
small, mechanical, and testable.

## Proposed package if the recommendations survive review

1. Amendment `pause-refuses-in-progress-story` (spec §6.4 + guard +
   fixture + row) — F1α.
2. Amendment `load-prose-matches-load` (spec §11.1 + §9 wording) — F2α.
3. Amendment `rc-triage-signal-death` (spec §9 script + scaffold
   template + fixture) — F3β.
4. INV-056 wording clarification — D2 residue.
5. Ledger cleanup: open question 3 marked resolved (D1), E1–E3 closed
   with their dispositions, waiting-on-owner list updated.

All amendments proposal-first; D-EP14 cadence applies; the owner
ratifies the package (each item is a spec deviation or an owner-class
question by plan §5).

## Adjudication appendix (2026-07-24, post-critic-round)

Five fresh critics, distinct lenses, read-only per the protocol; findings
adjudicated refute-by-default; every load-bearing claim below re-verified by
hand against the primary source before acceptance.

### F1 — recommendation REVERSED: (β), not (α)

Three independent findings kill (α) as scoped and one reframes the defect:

1. **The state has at least three other legal creation paths** (code
   critic, verified): the story verbs are epic-status-blind — no function in
   `internal/verb/stories.go` reads the epics table, so
   `block → epic pause → unblock → start` legally recreates the orphan after
   any pause-time guard; a plain no-`--legacy` `import` corpus can insert a
   PAUSED epic with an IN-PROGRESS story (`internal/verb/import_insert.go`
   terminal sets gate only CLOSED/DISSOLVED/DONE); and no R-rule detects the
   state by any path. Guarding `pause` alone relocates the hole — the
   "single write site" claim in the body is false.
2. **The system's own philosophy says detection, not prevention**
   (`internal/schema/ddl.sql:262-263`: "INSERT paths are deliberately open
   for import (§1.1); R12 is the compensating detection control"). Making
   (α) complete would demand epic-status checks in four story verbs, an
   import cross-check, and a new verify rule — a cascade of guards against
   a state the design does not treat as illegal. That is the patch profile
   the owner's criterion rejects.
3. **(α) reduces visibility instead of adding it** (architecture + flow
   critics, verified): `prime` nests `ready[]`/`blocked[]` under
   `epics_active[]` only (`internal/verb/prime.go:151-153`), and
   `sprint_goals[]` has no epic filter (`prime.go:361-363`) — so the
   orphaned sprint-goal entry is the ONLY window into a paused epic's
   in-flight story. Block-then-pause removes the story from every list in
   the contract: a silent state, strictly worse under §2 nothing-hides than
   the visible anomaly. The S8c close already ruled the same way about (γ):
   "Surfacing a dangling in-progress story is aligned with 'nothing hides',
   not against it" (`docs/stage-openings/s8c.md`).
4. **The dissolve-symmetry argument was unsound** (architecture critic):
   `dissolve`'s guard protects story rows from irreversible auto-DISSOLVE;
   `pause` touches no story rows — the precondition's justification does
   not transfer.

Adjudicated recommendation: **(β)** — one wording amendment declaring the
behavior intended (a paused epic's IN-PROGRESS story stays in
`sprint_goals[]`; the entry is the nothing-hides window into paused
in-flight work), anchored where `sprint_goals[]` is defined (§11.1) —
which is also where an agent-facing reader meets the state. Refuted along
the way: "block fabricates BLOCKED-ON-OWNER state" (downgraded — §6.2
explicitly sanctions non-owner external blocks through the same verb; the
state label is pre-existing naming, and the point is moot under β).

### F2 — direction (α) stands; scope grows (it is not prose-only)

Accepted (flow + governance + architecture critics, all verified by hand):

- The live agent-facing surface carries the same falsehood: the §8.4
  divergence refusal (`internal/verb/pipeline.go:283-284`) instructs
  "run selftracked load (fast-forward to the pulled dump)" — and `load`
  refuses any existing DB without `--force`
  (`internal/load/verb.go:53-65`), so the instructed command produces a
  second refusal. The body's "zero behavioral risk today" was wrong.
- Spec §8.4's own sentence (lines 872-874) is the source of that message;
  §9:873 and §11.1:1176 are the other two prose sites (exactly two
  'fast-forward' hits — confirmed).
- INV-456 (S8c, verified-by-command) restates the false fast-forward claim
  and its fixture never exercised that clause — the row rides the
  amendment and re-verifies.
- `internal/load/verb.go:55-57` carries a stale "the full §8.4 divergence
  matrix arrives at S8b" comment (it landed in the pipeline instead) —
  removed with the same change.
- Noted for post-v0, not scope: `dump_divergence` is a boolean and cannot
  distinguish behind-with-nothing-to-lose from true divergence.

### F3 — (β) survives, reshaped

Accepted: the destination message must not lie either — "busy/corrupt/env,
fix the environment" is wrong for an interrupt (rc=130), so β's routing
(rc outside {0,1} → the not-bypassable branch) comes with an honest
message ("verify did not complete — re-run; not bypassable as RED",
final wording at proposal). Adjudicated against the "swallows undocumented
exit codes" objection: an undocumented exit landing in a not-bypassable
re-run branch is the safe failure mode; today it lands in a bypassable RED
— the unsafe one. INV-428/429 (S8b) pin the current branch semantics; both
ride the amendment and re-verify, per the INV-087 precedent.

### D2 — residue expands to a spec wording amendment

The ambiguous parenthetical lives in the spec itself, not only the row:
§5 preamble lines 279-281 — "the one backfill path is `import --legacy`,
whose backfilled timestamps — git-derived or explicit-field, §6.2 — are
events-marked" — is the sentence INV-056 copies. The clarification is one
wording amendment touching that parenthetical plus the row.

### D1, D3, D4 — stand as written

Every critic that touched them confirmed the evidence (link-table triggers,
the single-call-site disagreement-note format, the S10 watch-item).

### Governance corrections to the package (accepted)

- **Tier declared now, before work**: FULL for every item (each touches
  the spec, the inventory, or a published document — plan §5 forces it).
- **Per-item amendments, per-item ratification** — no blanket package
  ratification; the plan's precedent (`plan-accounting-scope`) records
  rulings per question.
- **Cadence**: these are the S8b/S8c/S9 closes' escalated owner-class
  questions — the owner rules on each option here; proposals are then
  committed first and applied under D-EP14 as usual.
- **Scope discipline, named**: this round is outside S10's scope and is an
  explicit owner-directed switch (scope rule 3); S10 resumes after.
- **Row handling** stated per amendment (INV-456, INV-428/429, INV-056
  ride their amendments; F1β needs no row motion — it adds a spec note in
  place).

### Refuted findings (recorded per protocol rule 7)

- "F1γ's S8c refutation has no corroborating source" — refuted: it exists
  at `docs/stage-openings/s8c.md` (sprint_goals scoping, same two grounds),
  confirmed independently by two critics.
- "The §6.2 relaxation-list reading is contestable" — no critic produced a
  spec sentence supporting the restrictive E1 reading; §5 preamble's
  ambiguity is a wording defect, not a second norm.
- "The document misquotes the ledger" — no misquote found by either
  fidelity pass.
- Minor accepted without debate: the body cites "§6.4" where the epic verb
  catalog row is §6.2's table; corrected in the amendment texts.
