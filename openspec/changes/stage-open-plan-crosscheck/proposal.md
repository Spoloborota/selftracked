# Change: the stage-open re-read also checks placement, not just content

Target: `docs/v0-execution-plan.md` (revision 5 → 6)
Status: **accepted** · raised 2026-07-19 · applied same day

## Why

Reviewing the G0 inventory, the owner traced the seven implementation-phase
re-verification items of spec §16 back to their source research document and
asked where each had landed. All seven were present — one row each, correctly
worded — but three sat on a stage that cannot execute them:

| Row | filed at | belongs to | why |
|---|---|---|---|
| VACUUM-INTO / rename flow | S1 | S4 | the atomic rename lands with `load` |
| cross-OS serializer byte-equality | S1 | S3 | S1 builds no serializer |
| `go fix -diff` exit-code behaviour | S1 | S0 | a toolchain probe, already named in S0's DoD |

Two adjacent rows (the driver's minimum version, the driver/libc pair rule)
sat at S1 as well, though `go.mod` is S0's deliverable.

In every case the plan's own stage table named the correct owner in plain
text. The rows were **right about the obligation and wrong about the place**,
and the accounting gate cannot see the difference: a stage id that exists is
a stage id that passes.

Three review passes had already run over this inventory. The completeness
pass confirmed the seven items were *present* and stopped there; the
stage-assignment pass sampled stages and caught a different, systematic
inversion, but never diffed assignments against the sentences of the plan's
own stage table. Nobody was tasked with that comparison — a hole in the
review design rather than in any reviewer's execution.

D-EP4 already requires each stage to re-read its rows when it opens, but only
"against the spec sections they anchor" — a content check. Content was never
the problem here.

## What changes

**§5** — the stage-open re-read gains a placement check alongside the content
check: the stage confirms each of its rows is *executable by this stage*,
against the plan's own scope and DoD text for it, and hands back any row
whose obligation belongs to another stage. A row that no stage can execute
where it stands is a placement defect, corrected by moving it — not by
weakening the row.

**§8** — the enforcement-honesty list records that placement correctness is
review-checked, not machine-checked: the accounting script validates that a
stage id exists, never that the obligation can be executed there.

**§10** — recorded as D-EP6.

## Re-walk consequence (plan §3 rule 3)

This amendment targets the plan, not the spec, so it touches no inventory
row and no row loses `verified` status. Rule 3 flags an amendment that
touches no row; the flag resolves here as expected rather than suspicious,
because the inventory walks the specification and this change is
plan-native (§3 rule 5).

## Ratification

Owner, 2026-07-19, on the finding above and the proposed one-clause change:
**"apply this amendment"** — accepted as proposed, no scope changes
requested.
