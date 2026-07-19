# Change: split S1 into three sub-batches

Target: `docs/v0-execution-plan.md` (revision 9 → 10)
Status: **accepted** · raised 2026-07-19 · applied same day

## Why

S1 owns 129 inventory rows — more than any other stage, and more than either
of the two groups that were already split for being too large to review in
one pass. When the plan was reviewed, S5 (fifteen verbs) and S8 (init, hooks,
prime) were both judged beyond what one fresh-context reviewer can hold, and
were divided into S5a/S5b and S8a/S8b/S8c. S1 was not measured at the time and
kept its single row.

The S0 close supplies the evidence that this matters. S0 carried eight rows;
its review still found six defects, four of them inside the stage's own gates,
and two rows that were passing vacuously. A reviewer with sixteen times that
surface does not find sixteen times as much — it finds what fits in one
reading and reports the rest as clean. That is the failure the tier system
names: a procedure too heavy to run properly gets run nominally.

## What changes

**§4** replaces the S1 row with three sub-batches, each closing under its own
review:

- **S1a — the schema as text.** Embedded DDL v1 verbatim, database open and
  create, PRAGMA choreography, `meta` seeding, views. This is where the
  SQLite driver becomes a dependency — a schema package cannot open a
  database without one — so the driver and libc pins and the licensing
  intake land here, with the dependency they describe. Closes when a fresh
  database contains every object §5 declares.
- **S1b — the gates.** Every CHECK, every uniqueness and referential
  constraint, every trigger, each with the red fixture the specification
  requires: a gate that cannot be shown failing is decoration.
- **S1c — driver behaviour.** The implementation-phase re-verification items
  the specification refuses to assume: Serialize/Deserialize roundtrip on the
  full schema, extended versus primary result codes, RETURNING via Query,
  the recursive_triggers/REPLACE regression. Behavioural probes, each needing
  a real schema to run against, which is why they follow S1a and S1b.

**§10** records the decision as D-EP10.

## Consequences worth stating

The split moves no obligation and weakens no check; it changes only how many
of them a single reviewer is asked to judge at once. S1c depends on S1a
existing, and S1b's fixtures need S1a's schema, so the order is fixed rather
than parallel. The placement of the pins moved once during S1a's opening
re-read: they had been written into S1c, but the dependency they pin arrives
at S1a, and a pin check with nothing to check is the vacuous-gate defect this
project has now hit twice.

## Re-walk consequence (plan §3 rule 3)

Every S1 row keeps its obligation and its verification; only the stage label
changes, from S1 to one of S1a/S1b/S1c. No row loses `verified` status —
none of them had one.

## Ratification

Owner, 2026-07-19, on a recommendation to split S1 before starting it, with
the reasoning that a review of 129 rows in one pass is where the FULL tier
starts being performed rather than done: "split it and begin".
