# Change: accounting scope and per-stage row re-verification

Target: `docs/v0-execution-plan.md` (revision 4 → 5)
Status: **accepted** · raised 2026-07-19 at G0 close · applied same day

Filed as bare files: OpenSpec (D-EP1) is adopted but not yet installed, and
§2.1 states the flow degrades to bare-files delta proposals with the same
artifact trail. When the tool lands, this directory is its first import.

## Why

Gate G0 produced the traceability inventory (545 rows) and, in doing so,
surfaced two things the plan did not account for:

1. **Fidelity was verified by sampling, not exhaustively.** Three review
   passes swept for missed obligations and audited classifications, but no
   pass compared all 545 statements against the spec line by line. The plan
   had no place where that comparison was ever supposed to happen, so the
   gap would have persisted silently into implementation.
2. **One stage's definition of done contains work no inventory row can
   carry.** S10 retires the progress ledger and the inventory itself —
   artifacts this plan created, not obligations the spec states. The
   inventory walks the spec, so the total-accounting rule structurally
   cannot see them, and S10's single-row count understates its real scope.
   A survey of the whole stage table found S10 is the only such case.

A third item raised at the same time turned out not to need a change: two
rows describing the spec's own decision register were parked in an
unauthorised `spec-record` bucket, but their verification is a citation
check, and S12's definition of done already owns `docs link-check`. They
were reassigned to S12; the bucket is gone and no rule needed amending.

## What changes

1. **§5 — a stage opens by re-reading its own rows.** Before any code is
   written, the stage compares its inventory rows against the spec sections
   they anchor and corrects statements that drifted from an amended spec or
   were miscast at G0. This is the exhaustive fidelity check G0 deliberately
   did not front-load, relocated to where the rows are about to be used.
2. **§3 — the accounting rule's scope boundary is stated.** A stage's DoD
   may contain plan-native work with no inventory row; such items are closed
   by the stage's review pass, not by the accounting rule, and a stage's row
   count understates its scope wherever that applies.
3. **§4 — S10's row records that its scope is mostly plan-native.**
4. **§10 — the two decisions are recorded as D-EP4 and D-EP5.**

## Ratification

Owner, 2026-07-19, answering the three questions raised at G0 close by
option letter — question 2: **(b)**, question 3: **(a)**:

- **(b)** for the ratification standard: ratify the inventory as the control
  artifact now, with per-stage row re-verification, rather than a full
  545-row audit up front.
- **(a)** for the scope gap: state the boundary in one clause of the plan,
  rather than extending the inventory to walk the plan itself.

The same decision ratifies the inventory as G0's exit condition. (Question 1
needed no decision: research before the fork showed the rows already had a
home stage, so it was closed as a routing fix.)
