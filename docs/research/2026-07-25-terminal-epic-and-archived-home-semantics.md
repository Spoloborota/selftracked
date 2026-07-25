# Two unratified semantics: the terminal epic and the archived home

Revision 2, 2026-07-25 — rewritten after a five-lens critic round
(spec fidelity, code correctness, design judgement, evidence validity,
security-and-publication). Revision 1 carried two factual errors and an
infeasible implementation sketch; §7 lists what changed and why, because
a proposal that quietly repairs itself teaches the reader nothing.

Status: **analysis + proposed implementation, awaiting owner ratification.**
Nothing here is decided. Each proposal that changes specified behaviour
travels the amendment flow (`openspec/changes/<name>/`) before any code
lands; this document is the evidence the amendment would cite.

Raised by: tasks #33 and #35 (owner questions, IN-REVIEW), widened by
#48 and #49 (defects filed 2026-07-25). Observations come from drills in
throwaway scratch repositories outside this repository and from reading
the code at the cited lines; no state in this repository was mutated to
produce them. Counts quoted from this repository are as of dump
`c554c1c7aec2`.

Transcripts below are abridged: hook-chaining advisories (`R11`) and the
`N advisory` suffix that every scratch repository emits are elided, and
verb success lines are summarised after `->`. Elisions are marked; no
line was dropped that changes a verdict.

---

## Question A (#33) — does an archived `home` link satisfy R13?

### What R13 promises and what archiving means

R13 is the advisory "an OPEN task has somewhere its narrative lives":

```
-- internal/verify/rules_fs.go:303-305 (inside r13, lines 301-311)
SELECT t.id FROM tasks t
WHERE t.status = 'OPEN' AND NOT EXISTS (
    SELECT 1 FROM task_artifacts ta WHERE ta.task = t.id AND ta.role = 'home')
```

The query has no `archived` clause: any home row satisfies it, archived
or not. R3 — the rule that keeps links honest — is scoped to
*non-archived* artifacts: "Every **non-archived** artifact of a
non-ephemeral class resolves" (`docs/v0-spec.md:792`). That exemption is
correct and deliberate: archiving exists precisely to retire an artifact
from existence checking.

### The composition nobody chose

The two exemptions compose into a blind spot. Drill (abridged):

```
$ selftracked create --title "drill task" --status OPEN      -> #1
$ selftracked verify        -> warn R13: OPEN task #1 has no home link
                               verify full: 0 violation(s), 3 advisory
$ selftracked link 1 report:home.md --role home              -> linked
$ selftracked verify        -> verify full: 0 violation(s), 2 advisory  (R13 line gone)
$ selftracked link archive report:home.md --force            -> archived
$ rm work/reports/home.md
$ selftracked verify        -> verify full: 0 violation(s), 2 advisory  (R13 silent, R3 silent)
$ selftracked show 1
  #1 drill task [OPEN]
    artifacts: report:home.md (home, archived)
```

The two remaining advisories in each line are `R11` hook-chaining
warnings from the scratch fixture, unrelated to homes. An OPEN task whose
only home is an archived link to a file that no longer exists produces no
home-related signal anywhere, and `show` presents the dangling pointer as
the task's home. The schema comment states the invariant this breaks: "a
dangling home is unrecordable (the artifact must exist to be linked)"
(`docs/v0-spec.md:507`) — true at link time, not preserved afterwards.

Surfaces checked and also silent: `prime`, `list`, `show --json`,
`epic show`. `stale` is scoped to non-terminal work's resolved links
(`docs/v0-spec.md:716`) and does not cover this either.

### What already guards this

`link archive` refuses a live home without `--force` (`docs/v0-spec.md:709`;
`internal/verb/artifacts.go:292-303`), so archiving a home is already an
explicit, force-flagged act. The question is only whether the *advisory
census* should notice afterwards.

### Options

| | Behaviour | Cost | Consequence |
|---|---|---|---|
| **A1** | Keep as-is; add one spec sentence saying an archived home satisfies R13 | spec sentence + amendment; no code | The drill above stays silent forever: a class of dangling home survives with no signal anywhere |
| **A2** | R13 counts only non-archived homes | one SQL clause + tests + spec row edit + amendment, **plus an upgrade-time census jump** (below) | Archiving a home returns the OPEN task to the advisory census — a nudge to re-home it or accept the advisory. Never blocks anything (R13 is advisory) |
| **A3** | Leave R13; add a new advisory "OPEN task's only home is archived" | a new rule id, spec row, code, tests — more than A2 by one rule's worth of surface, not quantified further | Same signal as A2, and a second rule to explain |

**The upgrade-time cost A2 carries** (raised by the design critic, not
present in revision 1): any repository that already holds an OPEN task
whose only home was archived as routine hygiene sees new advisory lines
on the first `verify` after upgrading, with no state change of its own.
Advisories never block, so the cost is attention, not breakage — but it
is a real discontinuity and this repository cannot demonstrate it (it
holds no archived artifacts), which is exactly why it is stated rather
than measured.

### Proposed implementation (A2)

```go
// internal/verify/rules_fs.go — r13
SELECT t.id FROM tasks t
WHERE t.status = 'OPEN' AND NOT EXISTS (
    SELECT 1 FROM task_artifacts ta
    JOIN artifacts a ON a.id = ta.artifact
    WHERE ta.task = t.id AND ta.role = 'home' AND a.archived = 0)
```

Verified by a critic against a real `modernc.org/sqlite` connection over
this schema: the query parses, exempts a task with at least one live
home, and counts a task whose only homes are archived. The rule's scan
target and message format are unchanged.

- Spec §7 R13 row becomes "Advisory: OPEN tasks with no **live** `home`
  link (an archived home is history, not a home)".
- Tests: a task whose only home is archived appears in the census; a task
  with one archived and one live home does not.
- Amendment name: `r13-counts-live-homes`.
- Materiality **in this repository**: no archived artifacts as of the
  dump above, so the local census does not move. This says nothing about
  other repositories, which is where the upgrade cost lands.

Recommendation: **A2**. The `--force` gate protects the *act*; it says
nothing about the state that outlives it, and that state is a silent
dangling pointer.

---

## Question B (#35, widened by #48/#49) — what does a terminal epic mean?

### What the spec says a closed epic is

- Close is gated on six conditions in one transaction
  (`docs/v0-spec.md:750-766`).
- "On success, one transaction: status→CLOSED, close_sweep→today, events
  row. **Post-close validation = `V-n` rows.**" (`docs/v0-spec.md:765-766`)
- "CLOSED and DISSOLVED are terminal; there is deliberately no epic
  reopen" (`internal/schema/ddl.sql:302-303`).

There is a **second** sanctioned post-close write, which revision 1
missed: `worklog add --corrects N`. The schema says so outright — "a
correction may target any story incl. terminal ones"
(`internal/schema/ddl.sql:147-148`) — and `worklog.go`'s status guard
applies only to the `V-` branch (`internal/verb/worklog.go:98-124`).
Correction rows are append-only: the original row survives beside the
correction. That mechanism matters twice below.

### What the implementation actually allows

Every one of these succeeded `rc=0` against a CLOSED epic — the first six
in this author's drills, the rest reproduced independently by critics:

| Verb on a CLOSED epic | Result | Close condition it re-opens |
|---|---|---|
| `story add` | `drill/S3 [PLANNED]` | (1) every story terminal |
| `story ready` / `start` / `done` / `block` / `unblock` / `dissolve` | full lifecycle runs; **new non-V worklog rows land on a closed epic** | (1), (2), (5) |
| `create --epic <closed>` | new OPEN task homed there | (4) no open task homed |
| `reopen <task homed to a closed epic>` | task returns to OPEN | (4) |
| `criteria add` | accepted (also on DISSOLVED) | (3) criteria met |
| `criteria met` | rewrites a ratified criterion's evidence (non-runnable only) | (3) |
| `criteria check` | re-executes and flips `met 1→0` | (3) |
| `edit epic:<closed> --goal` | rewrites the closed goal | — |
| `edit epic:<closed>/S1 --dod` | rewrites a closed epic's story DoD | — |
| `link` / `unlink` / `link archive` on `epic:<closed>` | artifact links mutate freely | — |

No `verify` rule inspects a CLOSED epic's conditions, and there is no
reopen — so a closed epic can permanently display the exact states its
gate exists to forbid (#48). The story-lifecycle row is the sharpest of
the additions: it writes *new episode history* into a closed epic outside
the V-row mechanism the spec named as the only post-close surface.

### The record-destroying case (#49)

`criteria check` re-executes runnables regardless of epic status and
writes the outcome over the criterion's `met` and `evidence`
(`internal/verb/close_conditions.go:176-194`). Drill (abridged):

```
epic closed with criterion 1 = "$ test -f marker.txt" [met]
$ rm marker.txt
$ selftracked criteria check drill
  criterion 1: FAIL test -f marker.txt:  (exit status 1) @ ...
  {"error":{"code":"criteria","message":"a runnable criterion failed"}}   rc=1
$ selftracked epic show drill
  criteria: 1 [open] $ test -f marker.txt      <- a CLOSED epic with an unmet criterion
$ selftracked verify                            -> 0 violation(s)
$ selftracked epic close drill
  {"error":{"code":"blocked","message":"epic:drill is CLOSED; close works from ACTIVE or PAUSED"}}
```

The close-time evidence (`PASS ... @ <timestamp>`) is overwritten in the
database, and the events trail keeps only
`criteria check: 1 line(s), failed=true` — no copy of the text.

**Correction to revision 1**, which claimed the evidence was "gone for
good": a committed `dump.sql` preserves it, and `git show
<commit>:.selftracked/dump.sql` recovers it. Since this project mandates
an end-of-session bookkeeping commit, the common case is recoverable from
git. The accurate statement is narrower and still uncomfortable: the
live record is destroyed with no in-tracker copy, and recovery depends on
a commit having happened between the close and the overwrite. Task #49's
note has been corrected to match.

**Materiality here is not zero** — revision 1 said it was, and that was
wrong. This repository's own CLOSED epic carries a runnable criterion:

```
epic_criteria('v0-bootstrap', 2, '$ selftracked verify', met=1,
              'PASS selftracked verify @ 2026-07-24T23:27:19Z')
```

One `criteria check v0-bootstrap` executed while the gates are not green
overwrites that close-time evidence, on this repository, today.

### Options

**B1 — terminal means write-locked.** Refuse the mutating verbs on
CLOSED/DISSOLVED epics, preserving the two sanctioned exceptions: V-rows
(which already *require* CLOSED) and `--corrects` correction rows.

**B2 — terminal means a claim that must stay true.** Add a verify rule
that re-evaluates the close conditions on terminal epics from **stored
state only**. Detects damage from any source, including `import`, which
inserts rows directly (`internal/verb/import_insert.go:227,297,338`) and
never runs the close conditions.

**B3 — status quo, documented.**

**B4 — extend the correction-row idiom to criteria** (raised by the
design critic; absent from revision 1). `epic_criteria` has no
append-only correction shape: `criteria met` and `criteria check`
overwrite in place. The project already solved this exact problem for
worklog rows with `--corrects`. A criteria analogue would let a closed
epic's record be *amended with history preserved* instead of either
overwritten (today) or frozen (B1).

B1 and B2 cover different sources and are not alternatives. B4 is
orthogonal to both and addresses what neither does: legitimate post-close
correction.

### Why B1 alone is not enough — and can make things worse

Two failure modes the critics demonstrated, neither present in revision 1:

1. **B1 can make B2's findings unclearable.** If an imported CLOSED epic
   arrives with `met=0` on a criterion, B2 flags it forever — and B1 has
   refused the only two verbs that could ever clear it (`criteria met`,
   `criteria check`). An advisory that no verb can retire trains
   operators to ignore advisories.
2. **Condition 6 (at least two stories) becomes structurally
   unclearable** if `story add` is refused: an imported single-story
   CLOSED epic would trip B2 with no verb-path remedy.

So B1's refusal set cannot be "everything that writes"; it needs the
repair verbs carved out, or B4's correction shape to replace them.

### Proposed implementation (B1 + B2 + B4, with the carve-outs above)

**B1 — verb guards.** One shared helper refusing on terminal epics:

```
{"code":"terminal","message":"epic:<slug> is CLOSED; post-close work is V-rows (worklog add --story V-N) or a correction row"}
```

Exit code 1 (domain refusal) — matching the existing `not-closed`
register, and deliberately not repeating #32's exit-code slip.

Guarded (revision 1's list, plus everything the critics found unguarded):
`criteria add`; `story add`; the story lifecycle verbs `ready` / `start`
/ `done` / `block` / `unblock` / `dissolve`; task homing via `create
--epic` and `edit --epic`; `reopen` of a task homed to a terminal epic;
`edit epic:<slug> --goal`; `edit epic:<slug>/<SID>` story fields;
`link` / `unlink` / `link archive` on an `epic:` target.

**Not** guarded, by design: `worklog add --story V-N` and `worklog add
--corrects N` (both are the sanctioned post-close surfaces), and — if B4
ships — the criteria correction verb. Whether `criteria met` and
`criteria check` are refused outright or replaced by B4's correction path
is the substantive fork inside this option; refusing them without B4
creates failure mode 1 above.

**Schema-level guard is the wrong tool, and the reason is stronger than
revision 1 stated.** A status-aware trigger on `epic_criteria` INSERT
would break `import` (the importer inserts the epic first, CLOSED status
included, then its criteria: `import_insert.go:227` then `:297`) — and
also every `load`, because `load.Build` replays the dump through the same
live schema and triggers, and `verify`'s own R1 calls `load.Build` on the
tracked dump on every full run (`internal/verify/rules_fs.go:118-152`).
The trigger would therefore break `verify` continuously in any repository
with a closed epic carrying criteria — this one included. The
transaction-scoped `active_verb` marker (`ddl.sql:308-311`,
`epics.go:366,376`) could scope a trigger around that, but it is written
only by `epic close` today; extending it across import's multi-table
sequence is a change of its own, and §1.1's honesty about schema guards
("it stops mistakes, not a deliberate raw-SQL writer") applies.

**B2 — a detection rule (next free id: R16 — R1–R13 and R15 are taken,
R14 is folded into R1, `docs/v0-spec.md:790`).**

> R16 (advisory): a terminal (CLOSED or DISSOLVED) epic that no longer
> satisfies its close conditions — a non-terminal story, an
> OPEN/IN-REVIEW/NEEDS-TRIAGE task homed to it, or a criterion whose
> **stored** `met` is 0.

**The correction that matters most.** Revision 1 said R16 would "reuse
the existing condition queries (conditions 1, 3, 4)" and would be
"pure-SQL, so it can ride `--fast`". That was wrong in a way three
critics independently flagged, and the wrong reading is dangerous:
condition 3's only implementation is `runCriteria`/`runOne`, which
**executes a shell command** (`internal/verb/criteria.go:28-40`) and
**writes** the result back (`close_conditions.go:184-186`). Wiring it
into `verify` would (a) fail outright, because `verify` opens the
database with `PRAGMA query_only(1)` (`internal/schema/schema.go:64,77`),
and (b) if that were "fixed" by giving verify write access, turn every
`verify` — a command run freely for diagnostics and inside the pre-commit
hook — into a trigger for repository-controlled shell execution, and
mechanise the very overwrite #49 is about. It would also contradict B1's
own reason for refusing `criteria check` on terminal epics.

R16 therefore **re-executes nothing**. It reads `epic_criteria.met` as
stored, exactly as `epic show` does. Conditions 1 and 4 are genuinely
pure SQL and could be shared with `close_conditions.go`, but not for
free: those functions are unexported and typed against `*sql.Tx`, while
every verify rule takes `*sql.DB` (`internal/verify/rules_fs.go`) — an
export plus signature generalisation, which is real cost the amendment
must carry, not a free "one source of truth".

Advisory, not red: an imported history should not block commits. Whether
it rides `--fast` is a decision for the amendment — it is cheap enough,
but #46 shows the fast/full split has operator-visible consequences and
deserves an explicit call rather than a default.

**B4 — criteria corrections.** Mirror `worklog --corrects`: a new row
recording the amended judgement, the original preserved. Shape, storage
and whether `criteria met`/`check` become its only writers on terminal
epics are open design questions this document does not settle.

**What none of these fix.** Evidence already overwritten stays
overwritten in the database (recoverable from a committed dump only). A
separate, larger change would carry the criterion's evidence text into
the events row that `criteria check` and `epic close` already write.

---

## What the owner is being asked to decide

1. Question A: **A1**, **A2** (recommended), or **A3**.
2. Question B: which of **B1 / B2 / B4** ship, and in what order
   (recommended: B2 first — it is detection-only and cannot break a
   workflow; then B1 with the repair carve-outs; B4 if post-close
   correction should be possible at all).
3. Whether the evidence-preservation change (events rows carry the
   criterion's evidence text) is in scope now or deferred to a task.

**Security-class note, escalated per the critic protocol:** the R16
mis-specification in revision 1 would have introduced shell execution
into `verify`. It is corrected above, but the class deserves the owner's
eye: any future proposal that lets `verify` reuse the criteria engine
carries the same hazard.

Each acceptance produces an amendment under `openspec/changes/` before
any code merges; each rejection is recorded on the originating task with
the verdict, per the IN-REVIEW exit rule.

---

## §7 — what the critic round changed

| Revision 1 claim | Verdict | Revision 2 |
|---|---|---|
| "no runnable criteria" in this repository | **false** — `v0-bootstrap` criterion 2 is `$ selftracked verify` | materiality restated; the #49 scenario is live here |
| "the original evidence text exists nowhere else… gone for good" | **false** — a committed dump preserves it | narrowed; task #49's note corrected |
| R16 reuses close conditions 1, 3, 4 and is pure-SQL | **infeasible and hazardous** — condition 3 executes shell and writes; verify is `query_only` | R16 reads stored `met`; execution explicitly excluded |
| Guard list (6 verbs) | **incomplete** | 13 verb paths, incl. the whole story lifecycle, `reopen`, story-field edits, epic links |
| "the one sanctioned exception (V-rows)" | **incomplete** — `--corrects` is a second | both named; both excluded from the guard |
| Options exhausted | **no** — the correction-row idiom was missing | B4 added |
| B1 is uniformly safe | **no** — it can make B2's findings unclearable | failure modes stated; carve-outs required |
| Transcripts "verbatim" | **not literally** — advisories elided | elisions disclosed up front |
| Option labels were non-Latin letters | this repository is English-only by convention | relabelled A1–A3 / B1–B4 |
| "materiality is zero" | scoped to one repository, presented generally | scoped explicitly; upgrade cost stated |

Conflict of interest, stated plainly: the same author designed the test
campaign that produced #33/#35/#48/#49, adjudicated those findings, filed
the tasks, wrote both revisions of this document, and adjudicated the
critic round that corrected it. Revision 1 cited that campaign's earlier
findings (#22, #39, #40) as "the project's own evidence" for preferring
loud failure over silence; that phrasing overstated their independence
and has been removed. The owner is the only party outside this loop.
