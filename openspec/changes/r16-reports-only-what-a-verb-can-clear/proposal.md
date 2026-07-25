# Change: every R16 finding names a repair, and the story half gains one

Target: `docs/v0-spec.md` §6.4 (the post-close write-lock paragraph — one
carve-out added) and §7 (the R16 row); revision 3.30 → 3.31.
`internal/verb/stories.go` — `moveStory`'s terminal-epic guard becomes
per-transition; `internal/verb/terminal_epic.go` — the guard's documented
carve-out list; `internal/verify/rules_fs.go` — R16's messages;
`internal/cli/testdata/terminal-epic.txtar` — its header and a missing
case. No schema change.
Status: **proposed** · raised 2026-07-25 by task #60 under
epic `tracking-integrity`, story S1 · review tier **FULL** (plan §5,
D-EP7) · **supersedes part of an accepted amendment
(`terminal-epics-refuse-reopening-writes`)** · awaiting owner review
before application (plan §9)

## Why

R16 reports three conditions on an epic this tracker closed. Only one of
them can be acted on. Reproduced in a clean instance (#60), and
re-reproduced independently in a sandboxed instance during this round's
critic pass: close an epic by verb, import a children-only corpus citing
it, then try every verb that could restore the close condition.

- **Task clause** — "task #N (OPEN) is homed here". Clears two ways:
  `set-status N DONE`, or `edit N --detach`. The `reopen` refusal already
  names the detach path (`internal/verb/terminal_epic.go:76-80`).
- **Story clause** — "story S1 is not terminal". Clears by no route at
  all. `story dissolve`, `story ready` and `story done` all refuse with
  code `terminal` (`internal/verb/stories.go:165`); stories cannot be
  deleted; re-importing the same id is refused `story-exists`; and there
  is deliberately no `epic reopen` (`docs/v0-spec.md:618`, in the
  "Schema gates (triggers)" subsection — note that `terminal_epic.go:14`
  and tasks #57/#60 all cite this as "§5.4", which is the
  `epic_criteria` block; the wrong pointer is corrected where this change
  touches that comment).
- **Criterion clause** — "criterion N is not met". Re-read 2026-07-25
  against the current code and confirmed empirically in the sandboxed
  repro: since `terminal-epics-refuse-reopening-writes` landed, no verb
  can produce this state on a verb-closed epic — `criteria add`/`criteria
  met` refuse, `criteria check` is report-only there, and `import`
  refuses to re-declare an existing epic (`epic-exists`), so a
  children-only corpus cannot carry criteria. Its only remaining producer
  is raw SQL. The clause has quietly become a tamper signature of R12's
  family while its message still reads like an ordinary bookkeeping
  lapse.

R16's text names neither the asymmetry nor any remedy. An advisory a
reader cannot act on is one they learn to scroll past — and R16 shares
`verify`'s advisory channel with R10, R11, R13 and R15, so the cost is
paid by all of them.

## What changes

**`story dissolve` is carved out of the terminal-epic guard, on a CLOSED
epic only.** For a **non-terminal** story on a **CLOSED** epic,
`story dissolve SLUG SID --why TEXT` is accepted. Every other story
transition (`ready`, `start`, `done`, `block`, `unblock`) keeps refusing
exactly as today, `dissolve` of an already-terminal story keeps meeting
the existing `wrong-state` refusal, and `dissolve` on a **DISSOLVED**
epic keeps refusing `terminal`.

The CLOSED-only boundary is deliberate and matches R16's own scope: R16
re-checks the conditions an epic was **closed on**, and its query filters
`e.status = 'CLOSED'` in all three branches
(`internal/verify/rules_fs.go:333-350`). A DISSOLVED epic never passed a
close gate, so there is no claim to re-check and nothing to repair — the
same reasoning by which R16 exempts an epic that arrived CLOSED through
`import`. Carving out both terminal states, as an earlier draft of this
proposal did, would have granted a new write capability for a state class
that the motivating detector never reports. (The state is reachable on a
DISSOLVED epic too — `import` checks a corpus's own declared statuses,
never an existing epic's — but it is out of R16's scope by design and
stays out of this change's.)

**R16's messages name the repair** for each clause:

- task → `edit <id> --detach`, or set it terminal
- story → `story dissolve <slug> <sid> --why …`
- criterion → no verb writes this state: it arrived by raw SQL

**Spec**: §6.4's post-close paragraph adds `story dissolve` of a
non-terminal story on a CLOSED epic to the list of surfaces that stay
open, with the same justification `edit --detach` carries; §7's R16 row
states that each finding names its repair and that the criterion clause
is a raw-SQL signature.

## What implementing this actually touches

Named because the change is not the local edit it looks like:
`refuseTerminalEpic` is called from **one** place for five of the six
story transitions — `moveStory` (`internal/verb/stories.go:160-167`),
shared by `ready`, `block`, `unblock`, `done` and `dissolve`; `start`
carries its own call (`stories.go:219`). Exempting `dissolve` means
making that shared call per-transition, so a mis-scoped change silently
unguards four other transitions. Three pieces of in-repo documentation
state the current, soon-false invariant and are part of the change, not
follow-up: `moveStory`'s comment that "each of them would otherwise
append episode history to a closed epic" (`stories.go:161-164`),
`refuseTerminalEpic`'s enumeration of what is deliberately NOT guarded
(`terminal_epic.go:18-25`), and `internal/cli/testdata/terminal-epic.txtar`'s
header, which says the guard "keeps its two sanctioned surfaces open" —
three after this change. That fixture exercises `story dissolve` only
before the epic closes; the post-close case it lacks is added with the
carve-out.

## Why this does not reopen what the guarding amendment settled

`terminal-epics-refuse-reopening-writes` (accepted 2026-07-25) guards
story transitions because each "would otherwise append episode history to
a closed epic outside the V-row surface §6.4 sanctioned". That reasoning
holds for five of the six transitions and inverts for the sixth: the same
amendment carved out `edit --detach` on the stated ground that it
**removes a close-condition violation rather than creating one, and is
the repair path a refusal names**. `story dissolve` of a non-terminal
story on a closed epic is that argument applied unchanged — it is the
only story transition whose target status (`DISSOLVED`) satisfies close
condition (1), and it cannot create any other violation: condition (2)
is satisfied by the DISSOLVED worklog row it appends, and conditions
(4)–(6) do not involve story status. R6 is untouched in both directions
(its biconditional is about DONE, not DISSOLVED). R12 does not cover
story terminal states at all (`docs/v0-spec.md:821` names epics and
tasks), so it neither blesses nor blocks this — recorded so a later
reader does not mistake R12's silence for corroboration.

The cost accepted is the one the amendment named: a DISSOLVED episode row
lands on a closed epic outside the V-row surface. It is honest history —
the story *was* dissolved, after the close, as a repair, and `--why` is
required so the row says why. The alternative is a permanent unclearable
advisory, which trains readers to ignore the channel that also carries
R13 and R15.

Stated plainly: there is **no schema-level backstop** behind this guard.
`internal/schema/ddl.sql`'s story trigger validates only the story's own
old→new status matrix and knows nothing of the parent epic, so the Go
check is the whole boundary — before this change and after it. Narrowing
it is therefore a change to the only guard that exists, which is why the
per-transition scoping above is part of the proposal rather than left to
the implementer.

## Alternative considered

**Message-only**: leave the guard alone and have R16 say "no verb clears
this". Cheaper, changes no behaviour, and supersedes nothing. Rejected
because it resolves #60's wording defect while leaving the state itself
permanent: a repository that imports a delta corpus once carries a
red-flagged advisory for the rest of its life with no action available,
and §1.1's honesty principle argues for making the state repairable
rather than better-documented. Recorded because the owner may prefer the
cheaper branch — this is the ratification fork.

## Consequences accepted with this change

An epic's closed record can gain one more kind of post-close row. Anyone
auditing a closed epic must read `dissolve` rows dated after
`close_sweep` as repairs of imported state, not as work. The `--why`
text is what makes that readable, and it is already required. `epic show`
gives no structural help telling a post-close row from a pre-close one —
true of V-rows today and unchanged here; filed as task #64 rather than
widened into this change.

The import seam itself stays open by design (task #53, and the governing
amendment `terminal-epic-conditions-stay-true` states detection as the
answer there); this change gives that detection a repair, it does not
close the seam.
