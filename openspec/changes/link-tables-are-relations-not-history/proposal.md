# Change: link tables are current relations, not history — their no-delete triggers must go

Target: `docs/v0-spec.md` §5 (schema-gates comment + trigger set),
`internal/schema/ddl.sql`, three S1b inventory rows
Status: **accepted** · raised 2026-07-19 at S5a implementation · ratified and applied same day

## Why — a three-way contradiction no code can satisfy

1. §5's trigger block declares a BEFORE DELETE refusal on **every** entity
   table, task_links, task_artifacts and epic_artifacts included ("history
   is moved, never deleted" is schema-enforced). S1b built and verified
   those triggers (INV-153/154/155).
2. Three verbs the spec itself defines MUST delete link rows through the
   driver path the triggers bind: `reopen` "clears dup_of + link" (§6.2);
   `rel rm <id> <type> <id>` removes a relation (§6.2); `unlink` removes
   an artifact link (§6.2).
3. R7 (§7) demands duplicates links ⇔ dup_of one-to-one, both directions —
   so a reopened duplicate whose link cannot be deleted is a permanent R7
   violation manufactured by the schema itself.

Any two of the three can hold; not all three. The contradiction was
invisible until the first verb that deletes a link was implemented — the
S1b fixtures proved the triggers fire, and nothing before S5a tried to be
the sanctioned deleter.

## What the thesis actually protects

"History is moved, never deleted" (§1 principle 6) protects the *audit
record*. For link tables the audit record is not the row — it is the
events trail: §5.9's vocabulary carries `rel`, `link`, `unlink` precisely
because every relation change writes an event. Deleting a `task_links`
row loses no history the design ever promised to keep; refusing the
delete breaks three verbs and R7. Entity tables (tasks, epics, stories,
epic_criteria, artifacts, worklog, events, path_dictionary) keep their
triggers unchanged — nothing about their append-only posture moves.

## What changes

**Spec §5** — the trigger-block comment's enumeration excludes the three
link tables, with the reason stated: link rows are current relations
whose history is the events trail; their sanctioned deleters are `rel rm`,
`unlink`, and `reopen` (duplicates only). The three
`*_no_delete` triggers disappear from the canonical DDL.

**`internal/schema/ddl.sql`** — the same three triggers removed. Schema
stays v1: no released database or committed dump exists yet (S10 is the
switchover), so the byte-contract pair has no external party to break.

**Inventory follow-through** — INV-153/154/155 invert: their obligation
becomes "link-table rows ARE deletable on the driver path (the sanctioned
verbs' mechanism); audit lives in the events trail". They return to
`planned`, re-anchored §5+§6.2, and close at the stage that builds each
sanctioned deleter (S5a for reopen's duplicates link, S5b for `rel rm`
and `unlink`) — re-verified there with fixtures proving both the delete
and the events row. S1b's ledger entry is not rewritten; the amendment
log records why three of its rows moved on.

## Interim behaviour until ratified

S5a ships `reopen` with the dup_of clear and an explicit refusal on
DUPLICATE tasks naming this pending amendment — the spec-required
link-clearing lands only after ratification, per the repository's
proposal-first rule.

## Ratification

Owner, 2026-07-19: ratified as proposed, in the same message that granted
the standing pre-authorization recorded as amendment
`pre-authorized-amendment-cadence` (D-EP14). (Recorded in English by
meaning; given in conversation.)
