# Change: the ≥2-stories definition gains an enforcing gate at epic close

Target: `docs/v0-spec.md` §6.4 (revision 3.12 → 3.13)
Status: **accepted** · raised 2026-07-19 at the S1b open · ratified same day

## Why

§2 defines an epic as "a goal decomposing into ≥2 stories", and nothing
enforces it: no CHECK touches story count, `epic close` (§6.4) checks
terminality, criteria, homed tasks and commit evidence but never counts
stories, and no §7 R-rule does either. The S1b stage-open re-read found
inventory row INV-016 carrying the definition as a schema-gate no stage
could execute, and escalated the fork: rule the clause definitional, or
give it a mechanism.

The owner chose enforcement. The spec's own §1 principle 4 decides where
it lands: "a prose rule without a failing gate is an anti-pattern" — and
the natural gate is the close boundary, where the epic's shape is already
adjudicated. A goal that never decomposed past one story is a task that
borrowed an epic's ceremony; refusing its close as an epic is the
definition doing work.

## What changes

**§6.4** — the blocker list gains condition (6): the epic has at least two
stories, any status. Counting all statuses is deliberate: the §2 clause
says the goal *decomposed*, not that every path succeeded — an epic whose
second story was DISSOLVED still decomposed, and its close should not be
held hostage to retro-deleting history that an append-only ledger rightly
refuses to delete.

**Inventory follow-through** — INV-016 moves S1b → S6 (where the `epic
close` verb is built), re-typed schema-gate → verb-contract, its statement
naming condition 6 as the mechanism and its verification resolving to the
condition-6 red fixture. The row's other clauses (goal, criteria, worklog,
close stamp) were already carried row-per-item by the §5 schema rows; the
statement now says so.

## Re-walk consequence (plan §3 rule 3)

One spec section changes; the obligation it creates lands on the existing
row INV-016 (no row added, none lost, total unchanged at 549). Rows
INV-281…285 (conditions 1–5) are untouched — the new condition extends the
list without renumbering. No `verified` status is disturbed; S6 is
`not started`.

## Ratification

Owner, 2026-07-19, presented with the unenforced-definition finding and
both options, with enforcement-at-close recommended: **chose enforcement
(option b)** — accepted as proposed. (Recorded in English by meaning; the
direction was given in conversation.)
