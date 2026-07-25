# Change: R16 — an epic this tracker closed still satisfies what it was closed on

Target: `docs/v0-spec.md` §7 (new R16 row; §6.4 pointer; revision
3.26 → 3.27), `internal/verify/` (new advisory rule), its tests. No
schema change, no verb change.
Status: **accepted** · raised 2026-07-25 by tasks #35/#48 (S5 campaign,
block D, widened by coordinator drills) · ratified by the owner
2026-07-25 on
`docs/research/2026-07-25-terminal-epic-and-archived-home-semantics.md`
revision 2, option B2

## Why

§6.4 gates `epic close` on six conditions and then says "Post-close
validation = `V-n` rows" — one sanctioned post-close surface. But the
gate is evaluated only at the transition, and nothing re-checks it. A
CLOSED epic can hold a PLANNED story, an OPEN task homed to it, or a
criterion whose stored `met` is 0, and `verify` sees none of it; there is
deliberately no epic reopen (§5.4), so the divergence is permanent.

Drilled 2026-07-25: `story add`, the whole story lifecycle, `create
--epic`, `reopen`, `criteria add`, `criteria met`, `criteria check` and
`edit` all succeed against a CLOSED epic. The companion amendment
`terminal-epics-refuse-reopening-writes` closes those verb paths.
Detection is still needed, because `import` inserts rows directly and
never evaluates close conditions — and a rule, unlike a guard, also sees
what a raw-SQL writer left behind (§1.1's detection contract).

## What changes

**§7 gains R16, advisory:**

> R16 | Advisory: a CLOSED epic no longer satisfies the conditions it was
> closed on — a non-terminal story, a task in OPEN/IN-REVIEW/NEEDS-TRIAGE
> homed to it, or a criterion with stored `met = 0`. Scoped to epics
> whose closure this tracker performed (an `epic` event recording the
> close); an epic that arrived CLOSED through `import` never passed the
> gate and is not claimed to satisfy it.

- **Advisory, not red.** The state is bookkeeping divergence, not
  corruption, and an imported history must never block a commit.
- **Full partition only**, beside R10/R11/R13. The state it detects is
  not created by the act of committing, and the pre-commit path stays as
  quiet as it is today (§7's `--fast` split; see also task #46, which
  shows that split has operator-visible consequences).
- **It re-executes nothing.** R16 reads `epic_criteria.met` as stored,
  exactly as `epic show` does. This is stated because the obvious
  shortcut — reusing close condition (3)'s engine — would execute shell
  commands from repository state inside `verify` (`runCriteria` →
  `executeCriterion`) and write their results back, which `verify`'s
  read-only connection (`PRAGMA query_only(1)`) forbids and which would
  put repository-controlled execution on a path operators run freely,
  including inside the pre-commit hook.
- **CLOSED only, not DISSOLVED.** Dissolution is abandonment, not
  acceptance: its own preconditions are narrower than close's six, so
  there is no acceptance claim to re-check.
- **Conditions 1, 3 and 4 only.** Conditions 2, 5 and 6 are already
  covered or unreachable: R6 pairs DONE stories with their worklog rows,
  and story rows cannot be deleted, so the cardinality floor cannot fall.

**Tests**: an epic closed by the verb that later gains a PLANNED story, a
homed OPEN task, or an `met = 0` criterion appears in the census once per
divergence; an imported CLOSED epic with the same shapes does not; a
clean closed epic stays silent.

## Relationship to an accepted decision

`paused-epic-sprint-goal-is-intended` (accepted 2026-07-24) declined a
guard for an analogous orphan — a PAUSED epic holding an IN-PROGRESS
story — on the reasoning that "a pause-time guard relocates the hole
rather than closing it" and that no verify rule watches that state by
design. That reasoning does not transfer, and this amendment does not
supersede it:

- A PAUSED epic is not terminal: work legitimately continues inside it,
  and the sprint-goal entry is the window into that work. A CLOSED epic's
  whole meaning is that a gate passed.
- That amendment's objection was to a *single-path* guard while other
  paths stayed open. R16 is detection, which sees every path, and its
  companion amendment closes the verb paths as a set rather than one of
  them.

## What this change does NOT do

It does not block anything, does not reopen epics, does not repair
divergence, and does not judge imported history.
