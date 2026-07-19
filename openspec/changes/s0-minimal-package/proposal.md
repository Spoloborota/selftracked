# Change: S0 ships a minimal package so its gates are not vacuous

Target: `docs/v0-execution-plan.md` (revision 8 → 9)
Status: **accepted** · raised 2026-07-19 by the S0 close review · applied same day

## Why

S0's scope sentence lists "go.mod, Makefile catalog, golangci config, CI
skeleton (build/vet/test…), README skeleton, LICENSE/NOTICE intake" — no Go
package. The implementation added `internal/version`, and the close review
correctly flagged it as scope the plan does not name.

The package is not decoration. With no Go source in the tree, `go build ./...`
matches no packages and exits 0, `go test ./...` reports no tests and exits 0,
and the linter has nothing to lint. S0's own definition of done — "`make build
lint test fix-check` exit 0" — would then be satisfied by four commands that
each examined nothing. The stage would close on a green that means only that
the tree is empty.

So the choice was between removing the package and admitting the gates prove
nothing, or naming it. Naming it is the honest option, and this is the
amendment the review's finding requires rather than a silent retention.

## What changes

**§4, S0's scope** gains: a minimal package carrying the binary's build
identity, present so the build, vet, test and lint gates operate on real code
rather than on an empty match set.

**§10** records the decision as D-EP9.

## Consequences worth stating

A gate that passes over nothing is worse than no gate: it produces evidence
without content, and evidence without content is what the ledger's grade
column exists to prevent. This amendment makes S0's green mean something,
and it is the second time in one stage that a check was found to pass
vacuously — the first being the driver-pin check, whose rows moved to S1
where the driver actually arrives.

## Re-walk consequence (plan §3 rule 3)

No inventory row changes obligation. The package is plan-native scope, not a
spec obligation, and therefore carries no row (§3 rule 5).

## Ratification

Pending owner review. The change is applied because leaving the tree in the
reviewed state would mean either unnamed scope or vacuous gates; the owner
may reverse it, in which case `internal/version` is removed and S0's
verification commands are re-graded as operating on an empty tree.
