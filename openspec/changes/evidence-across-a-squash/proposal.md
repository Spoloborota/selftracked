# Change: evidence that survives a squash, and the anchor's window

Target: `docs/v0-execution-plan.md` (revision 10 → 11)
Status: **accepted** · raised 2026-07-19 · applied same day

## Why

Every closed row currently records evidence as a command and a commit —
`local: make check-pins @ 6cf73a1`. The owner intends to squash the pre-push
history into a single commit before publishing. That destroys every SHA those
records point at: after the squash, `6cf73a1` does not exist, and the
inventory's evidence column becomes a column of dangling references.

Rewriting the SHAs mechanically at squash time would not help. A squash is
many-to-one: a dozen commits collapse into one, so every record would point
at the same commit regardless of which tree state actually proved it. The
precision the SHA was carrying is not preserved by rewriting it; it is gone
the moment the history is.

The honest reading is that **pre-push evidence is provisional**. It records
that a claim held at some point in a history that will not be published. What
a reader of the published repository needs is that the claim holds on the
tree they are looking at.

## What changes

**§5** gains a closing step for the publication boundary: before the first
push, the gates are run once against the squashed tree, and every row
carrying `verified-by-command` is re-stamped with that commit. A row whose
check now fails is not re-stamped — it returns to `planned`, and its stage
re-opens. This is not ceremony: a row verified at S1a and quietly broken at
S5 is exactly what a single re-run against the final tree catches, and
nothing else in the process would.

**§8** records that pre-push evidence is provisional by construction, so a
green ledger before publication is a weaker claim than the same ledger after.

**§9** states the same for the `as of dump <sha12>` convention: an anchor
written before the first push cites a commit the squash will delete, so the
convention only begins to bind at publication.

**§10** records the decisions as D-EP11 and D-EP12.

## The anchor decision this makes obvious

The open question was whether the `as of dump <sha12>` anchor should ship in
v0 with its validator deferred (§17), or whether the gate should be pulled
forward.

Deferring costs nothing in the window that actually exists. Before the first
push there is no durable commit for an anchor to name — the squash removes
them — so an anchor written now cannot be verified by any validator, present
or deferred. The window in which the gate would earn its keep begins exactly
when the history becomes permanent, which is also when the deferred doc-lint
core is scheduled. Pulling it forward would build a check for a period during
which its subject cannot hold still.

Accepted as specified: the convention ships in v0, the validator stays
deferred, and the honest statement is that neither binds until publication.

## Ratification

Owner, 2026-07-19: D-EP9 ratified ("d-ep9 ratified"); the squash's effect
on evidence raised as a thing to solve before it bites ("we will very likely
squash these commits before the push, so we need to work out what to do
meanwhile"); and the anchor question delegated to be decided ("take the
decision on the anchor"), decided as above.
