# Change: the stage-open re-read leaves a committed record

Target: `docs/v0-execution-plan.md` (revision 11 → 12)
Status: **accepted** · raised 2026-07-19 · applied same day

## Why

The stage-open re-read (content, D-EP4; placement, D-EP6) is the only step
in the per-stage protocol whose execution leaves no trace on disk. A stage
that walked all of its rows and a stage that skipped the walk produce the
same visible state: the same inventory, the same ledger, the same first
implementation commit. Twice in a row that invisibility hid a real failure
until the close review:

- **S0** closed with three rows it had never been able to execute — the
  driver pin, the driver/libc pair, the licensing deliverable — all passing
  vacuously. They moved to S1 only when the close review caught them.
- **S1a** closed with twenty of its twenty-nine rows needing verbs, events
  or a serializer the stage does not build. The ledger's own post-mortem:
  "the stage-open placement check had not been run properly on all
  twenty-nine, which is how they survived to the close."

Both times the check existed and was required; both times it was omitted or
truncated, and nothing could show that before the close. The failure mode is
not a missing rule — it is an unobservable one. S1b is next and owns 89
rows, the largest sub-batch in the plan: the same silent truncation there
costs the most.

## What changes

**§5** — the re-read's output becomes a committed artifact: an opening
record at `docs/stage-openings/<stage>.md` (stage id lowercased), committed
**before the stage's or sub-batch's first implementation commit** — defined
as any commit touching files the stage's verification commands examine —
with one line per inventory row carrying three things: the content verdict
(`ok`, or what was corrected), the placement verdict (`ok`, or where the
row moved and why), and the concrete command or fixture the row's
placeholder verification name resolved to. The record is a point-in-time
trace of the open; the inventory stays authoritative for current row
state, and a row handed to the stage later enters the close review through
the existing checklist items, not through the record. The close checklist
gains item (vi): the record exists, predates the stage's first
implementation commit, covers every row the stage owned at open, and its
verdicts match what actually happened.

The record does not make the verdicts true; it makes the check's execution
observable, and gives the close review a document to sample instead of an
absence to prove.

**§8** — residual (c2) narrows honestly: the check's *execution* is now
visible on disk (a committed record, one line per row — no gate reads it;
the visibility is to a reviewer), but verdict quality is not — an `ok`
written without reading leaves the same record as one written after it.
That gap stays review-checked.

**§9** — the records' post-bootstrap fate is stated where every other
artifact's is: they freeze in git history as a procedural trace; nothing
from them migrates into `.selftracked/` at S10.

**§10** — recorded as D-EP13.

**Header (ancillary correction, explicitly named):** the governs sentence
cites `docs/v0-spec.md` "(revision 3.9)"; the specification is at 3.11, and
neither spec amendment bumped this pointer. The revision number is dropped
rather than updated — the same principle that removed revision numbers from
`.claude/CLAUDE.md`: a copied number goes stale unnoticed, and each
document states its own.

## Transition

S1b is the one stage already marked open (`in progress` in the ledger)
when this rule lands. It has no implementation commits yet — the git log
holds only S0/S1a work — so it complies on the normal path: its record,
`docs/stage-openings/s1b.md`, is committed before its first implementation
commit. No grandfathering clause is needed, and none is written: a stage
that had already begun implementing would have been a real transition case,
and this one is not it.

Closed stages (S0, S1a) are not retrofitted. Their opens happened without
the rule; a record written after the fact would be the record lying about
when the check ran.

## Re-walk consequence (plan §3 rule 3)

This amendment targets the plan, not the spec: no inventory row changes and
no row loses `verified` status. Rule 3 flags an amendment that touches no
row; as with `stage-open-plan-crosscheck`, the flag resolves as expected
rather than suspicious, because the change is plan-native (§3 rule 5). The
opening records themselves carry no inventory rows for the same reason the
ledger carries none (D-EP5's scope boundary): they are process artifacts
the plan owns, not obligations extracted from the specification.

## Ratification

Owner, 2026-07-19, presented with the two-stage failure pattern and the
proposed mechanism (a committed per-row content/placement verdict as the
condition for opening S1b): **directed that this proposal be executed
before S1b opens** — accepted as proposed, no scope changes requested.
(Recorded in English by meaning; the direction was given in conversation.)
