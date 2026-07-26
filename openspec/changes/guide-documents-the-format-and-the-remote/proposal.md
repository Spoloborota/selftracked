# Change: the migration guide documents the format it offers and the risks it does not name

Target: `docs/v0-spec.md` §10 (the migration posture — the md-table
format's stated scope, the rehearsal-copy remote, and the closed-epic
import seam) and §7's R16 row (one clause naming where an operator meets
the seam); revision 3.34 → 3.35. `docs/migration-guide.md` §4, §6, §8 and
a new subsection. No code change, no schema change, no verb change.
Status: **accepted** · raised 2026-07-26 by tasks #41, #52 and #53 under
epic `adoption-contract`, story S1 · review tier **FULL** (plan §5,
D-EP7) · ratified by the coordinating agent 2026-07-26 under the owner's explicit 2026-07-26 grant of autonomy for this session · applied to the spec and the
migration guide the same day

## Why

The guide offers three things it does not describe: a second import
format, a rehearsal posture, and an importer seam.

**The md-table format is documented nowhere (#41).** `import --file F
--format md-table` is in the §6.2 catalog and in the guide's §4 as a
five-word parenthetical — "(md-table is the other reader)". Its section
headings and column names exist only in `internal/verb/import_mdtable.go`.
Read there 2026-07-26, `knownColumns` (`import_mdtable.go:264-271`) is
the whole format:

| `## <section>` | columns |
|---|---|
| `paths` | class, scope, root, ephemeral, note |
| `epics` | slug, goal, status, status_note, close_sweep, created_at |
| `stories` | epic, id, title, status, dod, consumes, produces |
| `tasks` | title, status, note, epic, date, dup_of, future_increment, pointer_note, owner_steer |
| `worklog` | epic, story, state, commits, gate, review, date, note, legacy_reason |

Headings are lower-case and exact — `## Tasks` is refused
`unknown section "Tasks"` (reproduced 2026-07-26, exit 1). A fresh
adopter needed ten trial-and-error iterations against error messages to
reach a green import.

Two properties of that table matter more than the table: md-table has
**no `criteria` section** — epic acceptance criteria are JSON-only, and
an adopter authoring in md-table loses them silently by choosing the
format — while it **does** carry `worklog` and the tasks column
`future_increment`, which the guide's §2 describes as migrating without
saying by which reader. The guide's one line about md-table says neither,
so the format's real trade-off (criteria versus authoring convenience) is
invisible at the moment the reader picks.

**The rehearsal recipe never severs the inherited remote (#52).** The
guide's §6 tells the reader to rehearse in a disposable clone
(`git clone --no-hardlinks /path/to/project work/local/pilot-client`,
`migration-guide.md:152`) and §8 to take a copy-on-write copy of the host
per attempt (`migration-guide.md:194-196`). The two carry **different**
risks, and the guide states neither:

- a local `git clone` sets the new `origin` to the **source path**, so a
  stray push out of a §6 clone lands in the local repository it was
  cloned from and reaches no live project directly;
- a copy-on-write copy duplicates `.git` wholesale and therefore inherits
  the source's configured remote **verbatim** — on a machine with cached
  git credentials that is one stray push away from publishing rehearsal
  state to whatever that remote addresses.

Reproduced side by side in a scratch directory (#52, 2026-07-25): after
`git clone` the new `origin` is the source path; after `cp -a` it is the
source's remote URL. The first wording of that task claimed both halves
inherited the remote and was wrong on the mechanism; the correction is
why this change gives the two halves different sentences rather than one.

**The closed-epic import seam is undocumented (#53).** A corpus whose
`epics` list omits an epic and merely references its slug from a story or
a task is accepted against an epic a verb has closed: exit 0, `imported 0
epic(s), 1 story(ies), 1 task(s)`. A corpus that re-declares the epic is
refused `epic-exists`, so the seam is exactly the children-only shape.
Re-read 2026-07-25 against the governing amendment, this is **not** an
oversight: `terminal-epic-conditions-stay-true` states it as the reason
R16 exists — "Detection is still needed, because import inserts rows
directly and never evaluates close conditions" — and R16 scopes itself to
verb-closed epics precisely so this case is the one it catches
(`internal/verify/rules_fs.go:439,451,456,461` — the shared
`rules.ClosedByVerbSQL` predicate on all three clauses; #53's note cites
`rules_fs.go:334`, which the `r16-reports-only-what-a-verb-can-clear`
implementation has since moved — that line is now R10's idle clock, and
the pointer is corrected here). The defect that remains is
documentary: an operator running a repeat or a delta import against a
tracker that already closed epics is told nowhere that it can re-open the
conditions those epics were closed on, nor that an **advisory** R16 line
— which never fails a commit and which `verify --fast --quiet` prints as
a single rule-naming line — is the only place they will learn of it.

## What changes

**The guide's §4 gains an md-table subsection**, at the point where the
reader chooses a format: the five headings, their exact casing, their
columns, the one-table-per-heading rule, and — stated first, because it
is the decision — **md-table carries no `criteria` section; epic
acceptance criteria are JSON-only.** The subsection also states the two
authoring constraints §3 already lists as md-table-specific (a bundled
increment cannot be expressed; split per increment or author in JSON) by
pointing at them rather than repeating them.

**The guide's §6 and §8 get their two different sentences.**

- §6 (local clone): states what the clone's `origin` actually is — the
  source path — so a stray push is a local accident, not a publication;
  and that this is why the rehearsal clone lives under a gitignored path.
- §8 (copy-on-write copy): **severing the inherited remote is the first
  act on the copy**, before `init`, before the corpus, with the reason
  stated in the same breath: `cp`-class copies duplicate `.git`, so the
  copy's `origin` is the *source's* remote and cached credentials make it
  pushable. The recipe's handoff shape gains the step and its
  expectation, in the form the section already uses for every other step
  (a command and the exit it must produce).

**The guide gains a subsection on importing into a tracker that already
has closed epics**, stating: a repeat or delta import can insert children
under a verb-closed epic; the importer accepts it by design (it inserts
rows and evaluates no close condition); the result is an **advisory** R16
finding, not a refusal and not a red gate; and the repairs R16 now names
(`edit <id> --detach`, `story dissolve <slug> <sid> --why …`) are how it
is cleared. It also states the operator-facing consequence plainly: an
advisory does not stop a commit, so the person who must notice is the one
who runs `verify` after the import, and the guide's existing
"advisory census" expectation in §8 is where that check belongs.

**Spec §10** states the md-table format's scope (which sections it
carries and that criteria are JSON-only) as an importer property rather
than a guide detail, and states the rehearsal-copy remote hazard among
its adoption-posture bullets. **Spec §7's R16 row** gains one clause
naming the repeat/delta import as the condition's ordinary producer —
the row already says R16 exists for state that arrives by `import`;
what it does not say is that the operator's only notice is advisory.

## Alternative considered and rejected

**Make the importer refuse a children-only corpus against a verb-closed
epic.** It is the reading #53's original title invited, and it is
rejected: `terminal-epic-conditions-stay-true` accepted the seam
deliberately and built R16 to detect it, and
`r16-reports-only-what-a-verb-can-clear` then gave every R16 finding a
repair. Closing the seam now would supersede an amendment accepted the
previous day, on no new evidence, and would break the legitimate case the
seam exists for — a delta import that adds history to an epic whose close
the operator intends to re-establish. The remaining defect really is that
nobody is told. Recorded because this is the branch a reader of #53's
title will reach for.

**One sentence covering both rehearsal shapes.** Rejected on the measured
mechanism above: a single "sever the remote" instruction attached to both
would be false for the local clone (whose `origin` is a local path) and
would teach the reader a wrong model of what `git clone` does — the exact
error #52's first wording made.

## Consequences accepted with this change

The md-table table in the guide is a second copy of `knownColumns`, and
copies drift. It is accepted for one reason and bounded by it: the
alternative is a format an adopter can only learn by provoking error
messages, which is the measured status quo and cost ten iterations. The
drift is bounded by the guide's own stated contract — "the field names
and behaviors below are the ones the code accepts, not an aspiration"
(`migration-guide.md:6-7`) — and by the same content-test discipline the
JSON shape already lives under. No gate is proposed for it here; a gate
over guide prose is the deferred doc-lint core (§17).

The closed-epic subsection documents a behaviour rather than changing it.
An operator who reads it and still imports gets the same advisory. That
is the accepted posture (`terminal-epic-conditions-stay-true`), and
saying so is the whole of this half.
