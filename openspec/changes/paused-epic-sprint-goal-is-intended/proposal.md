# Change: a paused epic's sprint goal is intended, and §11.1 says so

Target: `docs/v0-spec.md` §11.1 (revision 3.17 → 3.18); no code, no
inventory motion
Status: **accepted** · raised at the S8c close (data/semantics critic,
2026-07-20, ledger open question 2) · ratified by the owner 2026-07-24 on
the adjudicated fork analysis
(`docs/research/2026-07-24-open-questions-fork-analysis.md`, F1 → β) ·
applied under D-EP14

## Why

`epic pause` can leave a story IN-PROGRESS inside a PAUSED epic, and
`prime` then lists that story in `sprint_goals[]` while `epics_active[]`
omits its epic — an entry the reader cannot attribute to any active epic.
The S8c critic asked whether `pause` should refuse (the way `dissolve`
does). The 2026-07-24 critic round killed that option on the evidence:

- The state has at least three other legal creation paths — the story
  verbs are epic-status-blind, and a plain `import` corpus can insert a
  PAUSED epic with an IN-PROGRESS story — so a pause-time guard relocates
  the hole rather than closing it, and no verify rule watches the state
  by design (INSERT paths are deliberately open; detection over
  prevention, §1.1).
- `ready[]`/`blocked[]` nest under `epics_active[]` only, so the
  sprint-goal entry is the ONE window into a paused epic's in-flight
  work. Suppressing it (by guard-plus-block or by scoping the list)
  hides that work — the S8c close already refuted the scoping variant on
  exactly this ground.
- `dissolve`'s precondition protects story rows from irreversible
  auto-DISSOLVE; `pause` touches no story rows. The symmetry argument
  does not transfer.

So the orphan is the design working, and the spec should say so where the
list is defined — the one place an agent reader meets the state.

## What changes

- **§11.1 `sprint_goals[]` definition** gains the clause: the list
  includes a story whose epic is PAUSED, deliberately — it is the
  nothing-hides (§2) window into paused in-flight work. Spec rev
  3.17 → 3.18.
- **Nothing else.** No verb changes, no inventory motion; ledger open
  question 2 closes as resolved-by-amendment.
