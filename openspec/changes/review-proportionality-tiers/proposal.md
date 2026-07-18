# Change: size the review to the change class

Target: `docs/v0-execution-plan.md` (revision 6 → 7)
Status: **accepted** · raised 2026-07-19 · applied same day

## Why

§5 prescribes one review shape for everything: a fresh-context pass that
re-runs the stage's verification commands and audits five checklist items.
That is right for a stage close — S6 owns 65 inventory rows — and absurd for
a one-line correction to a document.

A process whose lightest gear is heavy does not get run carefully; it gets
run nominally, or skipped with a justification invented at the time. The plan
already predicts this failure for itself: §8 names "a session writing code
with no stage at all" as its weakest link, and an unusable review procedure
is how a session ends up there.

The fix is to make the size of the review an explicit, planned decision with
named tiers, rather than an in-flight improvisation. **The tier is chosen
when the unit of work is planned and recorded with it** — choosing it while
finishing the work is how the tier becomes a rationalisation.

## What changes

**§5** gains a proportionality clause with three tiers:

- **LIGHT** — verification commands only, no reviewer. For changes that
  cannot alter behaviour or a published claim: formatting, a typo, a comment.
- **MEDIUM** — verification commands plus one fresh-context reviewer, no
  re-run of the full checklist. For a bounded change inside one stage that
  touches no inventory row's meaning.
- **FULL** — the procedure as written: fresh-context reviewer re-runs the
  commands and works the whole checklist. Mandatory for a stage or sub-batch
  close, for anything touching the spec, the plan, the inventory, or a
  published document, and for anything a reviewer has already found a defect
  in once.

Two guards, because the tier system's own failure mode is under-selection:

- The tier is recorded with the work item **before** the work starts; a tier
  lowered afterwards is a deviation and needs an amendment.
- Uncertainty resolves upward. A change that might touch a published claim is
  FULL until shown otherwise.

**§8** records the residual: tier selection is a judgement, and the
accounting gate cannot see it.

**§10** records the decision as D-EP7.

## Re-walk consequence (plan §3 rule 3)

Plan-native change; no inventory row is touched and none loses `verified`
status (§3 rule 5).

## Ratification

Owner, 2026-07-19, selecting from a consolidated list of proposed additions
to the agent-facing rules: items **1, 2, 3, 5, 6** accepted, where item 6 is
this one. The owner was told in the same message that item 6 would require
an amendment to §5 rather than a direct edit.
