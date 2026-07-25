---
name: selftracked-loop
description: The selftracked working loop — how to prime, refine the backlog, pick and execute a story, and close out a session. Invoke at the start of any work session on a selftracked repo.
---

# The selftracked working loop

1. **`prime`.** Run `selftracked prime --json`. If it reports
   `dump_divergence: true`, **stop and reconcile first** (`selftracked
   load`, then re-prime) — do not write over a diverged tracker.
2. **Backlog refinement** when `totals.triage > 0`: triage each
   NEEDS-TRIAGE task to `OPEN`, `IN-REVIEW`, park it, or `WONT-DO`. The
   `triage[]` list is capped (`prime_cap`), so when the queue is larger,
   re-`prime` between passes until `totals.triage` reaches 0.
3. **Pick** from `ready[]`, honoring `sprint_goals[]` — every IN-PROGRESS
   story is a sprint goal; multiple entries mean "finish or choose
   explicitly", never a silent pick.
4. **Execute:** `story start` → do the work → commit with `#NN` and/or the
   epic slug in the message → `story done --commits … --gate …`.
5. **At epic end:** `epic close` (it runs the criteria, sweeps the stories,
   and stamps the close in one transaction).
6. **End every session with a bookkeeping commit** — the dump refreshed by
   your last write must reach git, or the next `verify` reports a dirty
   dump. Stage explicitly (`git add .selftracked/dump.sql STATE.md && git
   commit`): a commit whose only content is what the pre-commit hook
   stages is refused by git when the index started empty.

## Drift rule

A new idea while working is **`create` + park, one command** — capture it,
do not pivot to it.

## When the product owner is absent

If every remaining story is blocked and `in_review` is non-empty:

- **Stop.** Do not pivot to out-of-scope work, and never answer the PO
  questions yourself.
- Ensure each open question is an `IN-REVIEW` task, so it surfaces in every
  future `prime`.
- If an in-progress story is what blocks, `story block --reason` it to free
  the WIP slot.
