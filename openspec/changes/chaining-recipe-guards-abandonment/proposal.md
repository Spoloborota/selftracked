# Change: the chaining recipe's lines survive abandonment

Target: `docs/v0-spec.md` §9 (chaining recipe) (revision 3.23 → 3.24)
Status: **accepted and applied** · 2026-07-24 · owner-directed (task #10)

## The defect

The chaining recipe writes selftracked's invocation into the **host
project's** hook — the one place our code outlives our installation. The
printed pre-commit line was:

    .selftracked/hooks/pre-commit || exit $?

If the adopter later abandons the pilot by deleting `.selftracked/` (the
documented rollback for a colocated install), that line starts failing with
127 — command not found — and `|| exit $?` propagates it: **every commit in
the host repository is blocked** until a human edits the host's hook by
hand. The recipe's failure mode lands on the adopter's project, not ours,
and it lands exactly when they have decided to stop using us.

Found at the S12 pilot preparation review (task #10).

## The correction

Both printed lines gain an existence guard:

    [ ! -x .selftracked/hooks/pre-commit ] || .selftracked/hooks/pre-commit || exit $?
    [ ! -x .selftracked/hooks/post-commit ] || .selftracked/hooks/post-commit

Semantics, proven empirically (all three cases run, 2026-07-24):

- hook absent (abandonment): guard short-circuits, rc 0, the host's own
  gates still run;
- hook present and RED (rc 7): rc 7 propagates, the host commit is
  blocked — INV-422's exit propagation is preserved unchanged;
- hook present and green: rc 0, the host's gates continue.

A present-but-non-executable hook is treated as absent — the same
semantics git itself applies to hooks, so the guard introduces no posture
git does not already have.

## Why an amendment

§9 states the printed line verbatim; changing a spec-quoted literal is a
deviation however small the diff. The owner directed the fix on 2026-07-24
(task #10 routing: "fix before the pilot").
