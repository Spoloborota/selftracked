# Change: `prime` names the condition instead of printing a bare counter

Target: `docs/v0-spec.md` §11.1 (the `prime` JSON contract and the digest
it describes); revision 3.31 → 3.32. `internal/verb/prime.go` — two
contract fields and the digest; `internal/rules/` — consumed, not
redefined. Tests. No schema change.
Status: **accepted** · raised 2026-07-25 by task #66 (the v0 half, split
from #61 on the owner's 2026-07-25 verdict) under epic
`tracking-integrity`, story S1 · review tier **FULL** (plan §5, D-EP7) ·
ratified by the owner 2026-07-25 · applied to the spec the same day

## Why

The owner's decision recorded in #61 (2026-07-25) scopes v0 to making
`prime` "say the condition out loud instead of printing a bare
`sprint_goals` count"; the interactive protocol that puts the choice to
the owner is v0.1.

Today the only live signal that an ACTIVE epic can receive no work is
`sprint goals: 0` — one zero among the digest's six counters
(`internal/verb/prime.go:483-484`), which is also what a perfectly
healthy epic between two stories prints, and what an epic with three
READY stories and none started prints. Nothing states the condition.

The JSON is where this matters most: the SessionStart hook injects
`prime --json` (§11.1), so the JSON — not the digest — is what a fresh
agent reads at the start of every session. A digest-only change would
never reach it.

And the JSON cannot express the condition today even implicitly.
`epicStoryTally` (`internal/verb/prime.go:196-224`) counts `done` and
`in_progress` and lists `ready[]` and `blocked[]` — there is **no
`PLANNED` branch**, and §11.1's contract has no `planned` field. So an
epic holding three PLANNED stories and an epic holding none render
identically (`done:0, in_progress:0, ready:[], blocked:[]`). A reader
cannot distinguish "work has a home, nobody made it ready" from "work has
no home at all", which is precisely the distinction this change exists to
publish.

## What changes

### 1. `notices[]` — the condition, stated

The JSON contract gains an array of typed objects, in deterministic order
(code ASC, then epic ASC), never capped:

```json
"notices": [{"code": "no-workable-story", "epic": "tracking-integrity"}]
```

v0 defines exactly one code. `no-workable-story` is emitted for each
ACTIVE epic with no story in a non-terminal status. The predicate is
**not restated here**: it lands once in `internal/rules` with the
companion amendment `r10-sees-the-window-it-was-meant-to-watch`, and both
`prime` and R10 call it. Two hand-written copies of the story-status
vocabulary is the drift this pair of amendments exists to avoid, not a
detail of it.

The field is a **typed enumeration, not prose**: a fixed code token plus
an identifier, so §11.1's rule that the epic `goal` and the story `title`
are the contract's only prose-class payload holds unchanged. The
mechanized guard for that rule — `TestPrimeTotalsAndNoProseScan`
(`internal/verb/prime_test.go:184-234`), which reflects over the whole
`primeOutput` type graph and asserts the exact set of string-bearing JSON
fields — must classify `code` explicitly in its identifier bucket
(`epic` is already there). That test failing until edited is the guard
working as designed, and the classification is a reviewable decision, not
an incidental fixture update.

`notices[]` is appended **after `sprint_goals[]` and before `totals{}`**
in `primeOutput`; the struct's field order is part of the stable contract
by its own statement (`prime.go:26-28`), so the position is specified
rather than left to the implementer. `TestPrimeEmptyTrackerShape`
(`prime_test.go:331-348`) gains `"notices":[]` to its required
empty-list renderings — the promise that the list marshals as `[]` and
never `null` is mechanically checked there for every other list.

### 2. `stories.planned` — the tally stops hiding a whole status

`epics_active[].stories` gains `planned`, a count, beside `done` and
`in_progress`. Without it the absence of a notice is the only way a
reader can tell a PLANNED-story epic from an empty one, which makes the
new field's *silence* load-bearing — a shape no contract should have.

### 3. The digest says it in a sentence

Each notice renders before the counter line:

> `notice: epic tracking-integrity has no story that can receive work —
> new work has no home; opening a story is the owner's call`

**Spec**: §11.1's contract paragraph adds `notices[]` with its shape, its
one v0 code, its ordering, its never-capped status and the
typed-not-prose rationale, and adds `planned` to the `stories{}` tally;
the digest sentence is described as a rendering of the contract, and the
digest itself stays explicitly outside the stable contract as it is
today.

## Why a new field and not a widened existing one

`totals{}` was the obvious host and is rejected: a count is exactly what
#61 says fails, and a reader who must compare two numbers to infer a
condition is doing the work the field exists to remove.
`epics_active[]` could carry a per-epic boolean, and that shape stays
available for v0.1; a separate list was chosen because the v0.1 protocol
will need to carry conditions that are not epic-scoped (work matching no
epic at all), and a list of typed notices extends to those without
another contract change.

## Consequences accepted with this change

`notices[]` and `stories.planned` are additions to a **stable contract**,
so every consumer that pins the shape sees new keys. In v0 the only
consumers are this repository's own tests, the SessionStart hook (which
passes the object through untouched — the hook's command is a fixed
`sh -c` string and `prime`'s output is never spliced into it) and the
skill; a JSON object gaining keys is additive for all three.

`notices[]` is bounded by the ACTIVE-epic count, like `epics_active[]`
itself, and is uncapped for the same reason (§11.1's "nothing hides"
invariant). A repository that keeps many epics ACTIVE simultaneously and
lets several sit in the dead zone therefore gets a proportionally long
`notices[]` on every session start. That is the intended behaviour of an
uncapped list and the stated cost of not capping it.

The v0 surface stops at stating the condition. It does not require the
agent to do anything, and no verb enforces the follow-up — that is
exactly the v0/v0.1 line the owner drew, and it is stated here so a later
reader does not read the field as an implemented protocol.

The `epic` value in a notice is whatever `epics.slug` holds. `import`
does not apply the kebab-case rule `epic create` enforces, so an imported
slug can carry arbitrary non-control characters onto this channel — a
pre-existing gap on a channel `epics_active[].slug` already rides, filed
as task #63 rather than widened into this change.
