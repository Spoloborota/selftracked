# Change: DoD-is-a-command is an authoring convention, not a machine gate

Target: INV-017's verification (S6); a one-line §2 clarification (spec rev 3.15 → 3.16)
Status: **accepted** · raised 2026-07-20 at S6 close · applied under D-EP14

## Why

INV-017 was extracted with the verification "DoD field rejects
free-text-only value / requires executable form" — a machine gate. But
"a command or invariant, never prose" is not machine-distinguishable:
no algorithm separates an executable command from a prose sentence
(`go test ./...` and `the tests pass` differ only in intent). S6 tried a
proxy — `story ready` refusing an empty DoD — which the close review
flagged as invented scope (an empty-check is not a shape-check, and
readiness's real precondition is DoR, the empty-blocked field). That gate
was removed.

The DoD-shape rule is the same class as the `PO:` prefix (INV-240–246)
and threat-model equivalence (INV-231): a prompt-enforced authoring
convention verified by review, not by a gate that cannot exist.

## What changes

**INV-017** verification becomes review-only. **§2** gains the honest
parenthetical that the command/invariant shape is a writing discipline,
not a schema/verb gate.

## Ratification

Applied under D-EP14; subject to owner post-review.
