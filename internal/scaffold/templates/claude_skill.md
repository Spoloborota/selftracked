---
name: selftracked-loop
description: The selftracked working loop — how to prime, refine the backlog, pick and execute a story, and close out a session. Invoke at the start of any work session on a selftracked repo.
---

# The selftracked working loop

1. **`prime`.** Run `selftracked prime --json`. If it reports
   `dump_divergence: true`, **stop and reconcile first** — do not write
   over a diverged tracker. Which side is authoritative is a **decision,
   not a command**: divergence has two directions, they call for
   opposite and irreversible moves, and **the wrong branch is total loss
   in exactly one of them**. Decide before writing anything:
   - **The test.** `git log -1 --stat -- .selftracked/dump.sql` and `git
     status --short .selftracked/dump.sql` say whether the tracked dump
     moved because another commit arrived or because this working tree
     changed it; `prime` is read-only and safe while diverged, so it can
     also be asked what the database holds. The rule: **the good side is
     the one holding work that exists nowhere else.**
   - **Tracked dump is the good side** (a pull brought another machine's
     newer state, the local database is stale): `selftracked load
     --force` replaces the local database with the tracked dump (it
     prints what it discards first); re-apply any unsynced local writes
     through verbs, then re-prime.
   - **Local database is the good side** (the working-tree dump was
     clobbered by a checkout, a merge resolution or a hand edit, while
     the database holds writes that never reached git): discard nothing
     — `selftracked dump`, then `selftracked verify`. A `FAIL R1:
     STATE.md does not match the database (stale projection); run
     selftracked state` is part of the procedure, not a new failure: run
     the `state` its message names and verify again. Then the
     bookkeeping commit (`git add .selftracked/dump.sql STATE.md && git
     commit`), without which the divergence is unresolved on the git
     side.
   - **Both sides hold unique work**: the two-writer accident PROMPT.md
     names. Neither branch is safe — stop and reconcile deliberately.

   Plain `load` refuses when a database already exists.
2. **Backlog refinement** when `totals.triage > 0`: triage each
   NEEDS-TRIAGE task to `OPEN`, `IN-REVIEW`, park it, or `WONT-DO`. The
   `triage[]` list is capped (`prime_cap`), so when the queue is larger,
   re-`prime` between passes until `totals.triage` reaches 0.
3. **Pick** from `ready[]`, honoring `sprint_goals[]` — every IN-PROGRESS
   story is a sprint goal; multiple entries mean "finish or choose
   explicitly", never a silent pick.
   **Work that did not come from `ready[]`** — an owner's request, a defect
   found mid-task, a follow-up, which is how most work arrives — is
   classified before any write by PROMPT.md's "Where new work goes": work
   advancing an ACTIVE epic's goal is a **story** under it, and that branch
   **stops for the owner**, because opening a story is a scope change an
   implementing agent never authorizes for itself; anything else is a
   **task**. Work matching no existing task, story or epic is named out
   loud first — `prime`'s `no-workable-story` notice names the epic-scoped
   case. This branch is for work you are **taking up now**; work you merely
   *notice* while a story is in progress is the drift rule below.
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

A new idea **discovered while a story is in progress** is **`create` +
park, one command** — capture it, do not pivot to it, whatever its size.
Work you are taking up now, with no story holding it, is step 3's
classification instead: the two answer different questions, and this one
answers only "something surfaced mid-story".

## When the product owner is absent

If every remaining story is blocked and `in_review` is non-empty:

- **Stop.** Do not pivot to out-of-scope work, and never answer the PO
  questions yourself.
- Ensure each open question is an `IN-REVIEW` task, so it surfaces in every
  future `prime`.
- If an in-progress story is what blocks, `story block --reason` it to free
  the WIP slot.
