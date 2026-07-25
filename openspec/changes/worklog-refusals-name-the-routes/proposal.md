# Change: the refusals met while recording work name the routes that apply

Target: `docs/v0-spec.md` §6.2 (`worklog` row); revision 3.28 → 3.29.
`internal/verb/worklog.go` — two refusal messages, both made state-aware.
Tests. No schema change, no new write route, no change to what is
accepted or refused — only to what a refusal says.
Status: **accepted** · raised 2026-07-25 by task #58 under
epic `tracking-integrity`, story S1 · review tier **FULL** (plan §5,
D-EP7) · revised the same day against a five-lens critic round (the
state-aware refusal replaces an unconditional sentence) · ratified by the
owner 2026-07-25 · applied to the spec the same day

## Why

Between an ACTIVE epic's last story going terminal and the epic being
closed there is a window in which the tracker accepts no record of work.
Reproduced in a clean instance (#58): a V-row is refused
(`"V-rows are post-close validation; epic %q is %s"`, code `not-closed`,
`internal/verb/worklog.go:121`); an episode row against an existing DONE
story is refused (`"worklog add writes only V-N rows (CLOSED epics) or
corrections (--corrects N); episode rows belong to the story verbs"`,
code `usage`, `worklog.go:51-53`); the only append that succeeds is a
correction row, which by definition mirrors an existing row's state and
is not new work. The window's measured extent on this repository is
recorded in task #58, where it can be re-derived from the trail rather
than copied into prose.

Both refusals are correct. Both are also silent about what the agent
should do instead, and the one remaining route — opening a story — is a
scope change the working contract forbids the implementing agent from
authorizing for itself. So the agent meets two accurate refusals, learns
nothing from either, and works off-book.

## What changes

Nothing about acceptance. Both refusals gain a second clause, and that
clause is **computed from the epic's actual story state**, not appended
unconditionally:

- the epic **has a non-terminal story** (`PLANNED`, `READY`, `BLOCKED`
  or `IN-PROGRESS`) → the refusal names that story and the transition
  verb that reaches it (`story ready`/`story start`/`story unblock`).
  No owner decision is involved: the home already exists.
- the epic has **no non-terminal story** — the dead zone — → the refusal
  names both sanctioned routes and their authority: a new story
  (`story add` → `story ready` → `story start`), which is a **scope
  change and the owner's call**, or a standalone task
  (`create --title …`) when the work does not advance the epic's goal.

The state-awareness is the point of this revision. A fixed sentence
appended to both refusals would tell an agent whose epic holds a BLOCKED
story that its only route is a new story requiring owner authorization,
when `story unblock` is the correct, agent-actionable remedy — the
refusal would be confidently wrong in exactly the state where a wrong
answer is cheapest to follow. Both refusals fire in states well outside
the dead zone (`usage` fires for any non-V `--story` without
`--corrects`; `not-closed` fires on ACTIVE, PAUSED and BACKLOG alike),
so a dead-zone-specific sentence must be gated on the dead-zone state.

Implementation consequence, stated because it changes where the check
lives: the `usage` branch is evaluated today before any database access
(`worklog.go:51`). Composing a state-aware message requires the epic's
story tally, so that branch moves inside the read/transaction that the
other refusal already performs.

**Spec**: §6.2's `worklog` row gains one sentence stating that both
refusals name the routes that apply in the epic's current state, and
that the story route is marked as an owner decision. The sentence is
normative about the *content* of those refusals, not their wording — the
tests assert which routes are named, not the prose.

## Why this and not a new write route

Criterion 2 of `tracking-integrity` admits two resolutions: a sanctioned
record, or a refusal that names the alternative and the owner's choice.
A new write route was considered and rejected on the owner's own decision
recorded in #61 (2026-07-25): the dead zone's resolution "is not another
write route, but a prompt", because an implementing agent authorizing its
own scope is the failure the working contract exists to prevent. A route
that let an agent record work under an epic no story covers would let it
do exactly that, silently. The refusal branch is therefore the whole v0
answer; the interactive prompt is v0.1.

## Relationship to the sibling amendments

The task-versus-story rule these refusals apply is stated once, in the
generated contract, by `contract-says-where-new-work-goes`; this change
is its refusal-surface expression and must not restate it differently.
The dead-zone predicate ("no story in a non-terminal status") is the same
one `r10-sees-the-window-it-was-meant-to-watch` and
`prime-names-an-epic-that-cannot-receive-work` evaluate; those two
amendments introduce the single shared definition, and this change
consumes it rather than hand-writing a fourth copy of the story-status
vocabulary.

## Consequences accepted with this change

The dead zone still admits no write. An agent that meets these refusals
must stop and put the choice to the owner; nothing in v0 forces it to.
The change makes the state *legible*, not impossible — that limit is
stated rather than papered over.

Refusal text is not a stable contract: no consumer parses it, and the
codes `not-closed` and `usage` are unchanged. The refusal surface's split
between domain messages and raw SQLite text (task #28) is untouched.
