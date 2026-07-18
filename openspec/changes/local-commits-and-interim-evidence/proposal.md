# Change: separate committing from publishing, and CI from stage closure

Target: `docs/v0-execution-plan.md` (revision 7 → 8), `.claude/CLAUDE.md`
Status: **accepted** · raised 2026-07-19 · applied same day

## Why

Two rules were blocking work for reasons that turned out to belong to a
different rule.

**S0 could not close without publishing.** Its definition of done requires
"CI green Linux+macOS+Windows". Continuous integration runs on push; pushing
a public repository is publishing. So a stage about creating a Go module
skeleton was gated on a decision about when the project goes public — two
unrelated things fused by one clause.

**Committing was gated on approval meant for publishing.** The standing rule
was "commits are owner-approved; agents do not push". But a commit is
invisible until pushed: the protection the approval was providing is
provided, completely, by the push prohibition. What the approval actually
cost was restore points — a long working session accumulated dozens of files
with no recoverable checkpoint, and untracked files are not recoverable at
all.

## What changes

1. **§4, S0's DoD** — the local gate run is interim evidence. `make build
   lint test fix-check` exiting 0 on a clean checkout closes S0; the
   three-platform CI matrix becomes a **precondition of the first push**,
   not of S0's closure. The CI workflow file is still authored at S0; it is
   simply not required to have run.
2. **§8** — records that stages closed on interim evidence carry a weaker
   claim than stages closed on CI, and that the ledger must say which.
3. **Commit policy** (`.claude/CLAUDE.md` rule 6) — local commits are made
   freely, as often as work reaches a coherent state. **Pushing remains
   prohibited without the owner**, and that prohibition is now the only
   thing standing between this repository and publication, so it is stated
   alone rather than bundled with committing.
4. **§10** — recorded as D-EP8.

## Consequences worth stating

- History becomes granular and is published later; the owner's stated plan
  is to squash before the first push if the intermediate history is not
  wanted. That works for states that no longer exist at the tip; it does not
  remove anything still present in the final tree.
- "Done" now has two grades. A stage closed locally is done on one machine's
  word. The first push, when it happens, is also the moment the whole
  history's claims meet a real matrix — and any that fail there were never
  as done as the ledger said.

## Re-walk consequence (plan §3 rule 3)

The S0 DoD change touches no inventory row's obligation — the `go fix -diff`
re-verification row and the twelve other S0 rows are unchanged. No row loses
`verified` status.

## Ratification

Owner, 2026-07-19, selecting from an analysis of what blocks autonomous
implementation: accepted the interim-evidence change to S0 and the split of
committing from pushing, in the words "let us separate them as you proposed
— commit locally freely, never push without you", and instructed work to
begin.
