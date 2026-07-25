# Change: a terminal epic refuses the writes that re-open its close conditions

Target: `docs/v0-spec.md` §6.2 (criteria/story/edit/reopen rows) and §6.4
(post-close paragraph); revision 3.27 → 3.28. `internal/verb/` — a shared
guard plus its call sites; `internal/verb/close_conditions.go` — the
report-only branch for a terminal epic's `criteria check`. Tests. No
schema change: the guard is verb-level by design (see below).
Status: **accepted** · raised 2026-07-25 by tasks #35/#48/#49 (S5
campaign block D, widened by coordinator drills and a five-lens critic
round) · ratified by the owner 2026-07-25 on
`docs/research/2026-07-25-terminal-epic-and-archived-home-semantics.md`
revision 2, option B1 with the repair carve-outs

## Why

§6.4 evaluates six conditions and then closes the epic; §5.4 states there
is deliberately no reopen. Yet every verb that could re-create a blocked
condition still accepts a terminal epic. Drilled and independently
reproduced 2026-07-25: `story add` plants a PLANNED story; the whole
story lifecycle (`ready`/`start`/`done`/`block`/`unblock`/`dissolve`)
runs and appends **non-V worklog rows** to a closed epic — new episode
history outside the one surface §6.4 sanctioned; `create --epic` and
`reopen` put a workable task back under a closed epic; `criteria add`
adds an obligation that can never gate anything; `criteria met` rewrites
a ratified criterion's evidence; `edit` rewrites a closed epic's goal or
its stories' DoD.

The sharpest case is `criteria check` (task #49): on any epic status it
re-executes runnables and writes the outcome over `met` and `evidence`
(`close_conditions.go:176-194`). A criterion this repository closed
`v0-bootstrap` on — `$ selftracked verify` — is one stray check away from
having its close-time evidence replaced, with no copy in the events trail
(only `check: N line(s), failed=…`) and no verb able to restore it.

## What changes

**Refused on a CLOSED or DISSOLVED epic** (`{"code":"terminal"}`, exit 1):

- `criteria add`, `criteria met`
- `story add`; `story ready|start|done|block|unblock|dissolve` on a story
  whose epic is terminal
- `create --epic <terminal>`; `edit <task> --epic <terminal>`
- `reopen <task>` while the task is homed to a terminal epic — the
  refusal names the repair: detach first
- `edit epic:<terminal> --goal`; `edit epic:<terminal>/<SID>`
  (`--dod`/`--consumes`/`--produces`)

**`criteria check` on a terminal epic becomes report-only**: it executes
the runnables and prints each result exactly as today, prints one line
saying the results were not recorded because the epic is terminal, exits
1 on failure as today, and **writes nothing** — no `met` flip, no
`evidence` overwrite. Refusing it outright was the first draft; it was
rejected because the diagnostic question ("does this closed epic's
criterion still hold?") is legitimate, and because a refusal removes the
only way to interrogate a criterion whose stored `met` a rule may be
flagging.

**Deliberately NOT guarded**, each for a stated reason:

- `worklog add --story V-N` — the sanctioned post-close surface (§6.4);
  it already *requires* CLOSED.
- `worklog add --corrects N` — the sanctioned correction surface; the
  schema states a correction may target any story including terminal
  ones (§5.7), and correction rows are append-only, so the original
  record survives.
- `edit <task> --detach` — it *removes* a close-condition violation, and
  it is the repair path the `reopen` refusal names.
- `link` / `unlink` / `link archive` on an `epic:` target — attaching a
  retrospective or a post-mortem to a closed epic is ordinary
  documentation and violates no close condition.

**Spec**: §6.4's post-close paragraph gains the write-lock and its two
sanctioned exceptions; §6.2's `criteria` row gains the report-only
branch; the `edit`/`reopen`/`story` rows gain the refusal.

## Why verb-level and not a schema trigger

A status-aware trigger on `epic_criteria` (or `stories`) INSERT would
fire on `import`, which inserts a CLOSED epic and then its criteria
(`import_insert.go:227` then `:297`), and on every `load`, which replays
the dump through the same schema — including the `load` that `verify`'s
R1 performs on every full run. The trigger would therefore break `verify`
continuously in any repository holding a closed epic with criteria, this
one included. The transaction-scoped `active_verb` marker could scope a
trigger around that, but it is written today only by `epic close`;
extending it across import's multi-table sequence is a separate change.
§1.1's own honesty applies: schema teeth stop accidents, detection
handles adversaries — and the adversary case is the companion
amendment's R16, not this one.

## Relationship to an accepted decision

`paused-epic-sprint-goal-is-intended` (accepted 2026-07-24) refused a
`pause`-time guard because "a pause-time guard relocates the hole rather
than closing it" — the story verbs were epic-status-blind and `import`
could insert the same state anyway. This amendment does not supersede
that decision; it answers the objection instead:

- The objection was to guarding ONE path while the others stayed open.
  This change closes the verb paths as a set — including the very
  story-verb blindness that amendment cited as the reason a single guard
  would not hold.
- The `import` path stays open by design and is covered by detection
  (R16), which is the split §1.1 prescribes.
- A PAUSED epic is not terminal: work legitimately continues in it, and
  that is why its orphan was ruled intended. A terminal epic's meaning is
  that the gate passed.

## Side effect on record

The guard reads the target epic's status before the write, so a
*nonexistent* epic is now named by a domain refusal (`no epic "ghost"`)
where `edit --epic` previously let the foreign key speak in raw SQLite
text. No spec clause promised the FK message — it was asserted only by
`tasks-edit.txtar`, updated with this change — and the new message is of
the class task #28 tracks. Recorded rather than absorbed silently.

## Consequences accepted with this change

Workflows that today write to a closed epic must now either reopen the
work as a new epic (§5.4's stated path), record it as a V-row, or correct
it with a `--corrects` row. An imported repository whose closed epics
carry non-terminal stories cannot advance those stories through the verbs
any more; that state is visible through R16 and is not repaired by this
change.
