# Change: R10 reports an ACTIVE epic that can receive no work, at once, and the commit boundary stops swallowing it

Target: `docs/v0-spec.md` §7 (the `--fast` partition sentence, the R10
row, and the `--quiet` behaviour the pre-commit path relies on); revision
3.29 → 3.30. `internal/rules/` — the shared predicate;
`internal/verify/rules_fs.go` — R10's query and message;
`internal/verify/verify.go` — the partition; `internal/verify/verb.go` —
the `--quiet` summary line. Fixtures. No schema change.
Status: **accepted** · raised 2026-07-25 by task #59 under
epic `tracking-integrity`, story S1 · review tier **FULL** (plan §5,
D-EP7) · revised the same day against a five-lens critic round (the
`--quiet` half, the shared predicate and the reversed fixture were added
after it showed the partition move alone was inert) · ratified by the
owner 2026-07-25, who chose the `--quiet`
summary line over the trigger-only alternative recorded below · applied
to the spec the same day

## Why

R10 is the only rule watching an ACTIVE epic drift out of its story
structure, and it cannot see the drift #58 describes. It fires only when
an ACTIVE epic has **no READY/IN-PROGRESS story AND** no non-correction
worklog append within `idle_days`, default 14
(`internal/verify/rules_fs.go:273-298`). In the dead zone the second
clause is false by construction: the last story's DONE row was appended
minutes ago. The behaviour is not accidental — `TestR10RecentAppendSuppresses`
(`internal/verify/verify_test.go:443-455`) seeds exactly the dead zone
(ACTIVE epic, one DONE story, one recent non-correction append) and
asserts R10 stays silent. So the epic can absorb up to `idle_days` of
unrecorded work before a single advisory line appears.

R10 is also full-only (`internal/verify/verify.go:144`), so the commit
gate never evaluates it — commits carrying off-book work pass silently at
the boundary where the work becomes permanent.

## What changes

### 1. A second, window-free trigger

An ACTIVE epic with **no story in a non-terminal status** — no `PLANNED`,
`READY`, `BLOCKED` or `IN-PROGRESS` story, i.e. every story terminal or
no story at all — is reported immediately, independent of `idle_days`:

> `epic X has no story that can receive work; new work has no home`

When the idle clause also holds, the same line states both facts rather
than emitting two findings — one epic, one line.

The trigger is deliberately **narrower than R10's existing story clause**
(`no READY/IN-PROGRESS`). A `PLANNED` story is a home: `story ready` →
`story start` reaches it with no scope change. A `BLOCKED` story is the
sanctioned PO-absent state (§11.3), where the correct action is to stop —
firing there would claim work is being done off-book at the exact moment
the contract says none should be. Both are excluded; the idle clause
keeps watching them on its 14-day horizon exactly as today.

### 2. One definition of the predicate, in `internal/rules`

"An ACTIVE epic with no story in a non-terminal status" is evaluated by
two packages: `internal/verify` (this rule) and `internal/verb`
(`prime`'s notice, the companion amendment). The two do not import each
other and the codebase has already drifted on exactly this shape — three
call sites spell out overlapping story-status subsets as independent SQL
literals (`internal/verb/stories.go` defines status constants that
`internal/verb/epics.go`'s auto-dissolve query and
`internal/verify/rules_fs.go`'s R10 query both ignore in favour of inline
strings).

So the predicate lands **once**, in `internal/rules` — a package
`internal/verify` already depends on and `internal/verb` can import
without a cycle — and both surfaces call it. Restating it in a second SQL
literal is what this change exists to prevent, not a detail of it.

### 3. R10 joins the `--fast` partition, and `--quiet` stops swallowing advisories

R10 is pure-SQL: two small queries (an `idle_days` lookup and the epic
scan) and a clock read, no filesystem and no git. Its place among §7's
"filesystem/git-bound" skips (R2, R3, R5, R10, R11, R13) has never
matched what it does.

Moving it alone, however, achieves nothing. The shipped pre-commit hook
runs `selftracked verify --fast --quiet`
(`internal/scaffold/templates/hooks/pre-commit:9`), and `--quiet`
suppresses the report entirely while advisory findings never affect the
exit code (`internal/verify/verb.go:38-54`). An advisory moved into the
fast partition today is computed and then discarded.

`--quiet` therefore gains exactly one line, on **stderr**, when the run
is otherwise clean but advisories were found:

> `verify: advisory R10, R13 (run 'selftracked verify' for detail)`

Bounded to one line, naming the rules and not their instances, so a
repository with dozens of advisory findings pays one line per commit, not
dozens. `--quiet` keeps suppressing the report body; the exit contract is
untouched (advisories still never fail a commit).

**Spec**: §7's `--fast` sentence moves R10 from the skipped list into the
pure-SQL set; the R10 row states both triggers, the non-terminal-story
definition and the `PLANNED`/`BLOCKED` exclusions with the reason above;
the `--quiet` summary line is stated where §7 describes the pre-commit
path.

## Alternative considered

**Trigger only, no partition move and no `--quiet` change.** `prime`
would still name the condition at session start (companion amendment),
and `verify` run by hand would report it; the commit boundary would stay
silent. Cheaper and touches no shared behaviour. Rejected because #59's
second half — "commits that carry off-book work pass silently" — would
remain wholly unaddressed, and closing #59 on half its content is the
accounting failure this epic exists to fix. Recorded because the
`--quiet` change alters what every adopter sees on every commit, which
makes it the owner's call.

## Consequences accepted with this change

**A one-line advisory summary appears on essentially every commit in this
repository.** Its `verify` currently reports advisories from R13 alone,
so the line will be present far more often than absent until those
findings are cleared. That visibility is the change's purpose and also
its main cost; it is stated rather than discovered later.

**A true positive that reads as noise, once per epic.** `epic activate`
and the first `story add` are two separate commands with no atomicity
between them, so every epic passes through the reported state briefly. A
commit landing in that gap draws the line. The state is genuinely the one
described — an ACTIVE epic no work can be recorded against — but it is
also routine, and a reader who sees it during epic setup should not read
it as a defect.

**A fixture whose assertion this reverses.**
`TestR10RecentAppendSuppresses` asserts silence in precisely the dead
zone; under this change that seed must report. It is rewritten to assert
the new finding and a second fixture keeps the *idle-clock* exclusion it
was really guarding (a recent append must still suppress the **idle**
half). The stale comment at `internal/verify/verify_test.go:523` ("R15 is
the one advisory cheap enough for `--fast`") is corrected with it.

R10 stays **advisory**: it never fails a commit or a `verify` exit.

Query cost at the commit boundary is not asserted here; measuring it is
an obligation of the implementing story's close, not a claim of this
proposal. For the record of what it is not: `epics.status` carries no
index and `worklog`'s primary key is `(epic, seq)`, so neither R10 query
is index-assisted beyond the `epic` prefix.
