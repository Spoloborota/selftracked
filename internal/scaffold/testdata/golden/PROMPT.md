# selftracked — agent instructions

This project tracks its own work in a local SQLite database, dumped
deterministically to `.selftracked/dump.sql` (the one tracked, reviewed,
synced surface) and projected to `STATE.md`. You interact with it through a
fixed set of verbs — never by touching the database or the dump directly.

## Running the tool

The binary is `selftracked`; `strk` is the same binary under its short
name, and every command in this file works under either. It is expected
on `PATH`, installed once per machine and from outside this repository.

When it is not on `PATH`, two invocations still work. They are given
here **as shapes, never as literal paths**, because this file is written
by a verb — `init` — and selftracked's own verbs never write hostnames,
usernames, or absolute paths into a file the repository tracks:

- **A repo-local build output** — a checkout of selftracked built in
  place and called by its path relative to the repository root
  (`./bin/selftracked` when the build writes to `bin/`).
- **An absolute install path** — the binary wherever it was installed,
  called in full. Type it at the call site: it names one machine, so it
  belongs in neither this file, nor a commit message, nor a tracker row.

One line settles which of the three cases you are in, and it replaces
probing the environment:

```sh
selftracked prime
```

`prime` either **answers** — it is the session-start read — or **names
its refusal**: a binary that is not on `PATH` is the shell saying
`command not found`, a tracker that is not in the working directory is
`not-found` naming the root it did find instead, and a schema this
binary cannot serve is the version gate saying so. Read what came back
and act on it; do not go looking for the database.

## The working loop

**Start every session with `prime`** — the session-start read: active
epics with their unmet criteria, sprint goals (every IN-PROGRESS story),
and the ready / triage / in-review / stale / parked queues. If it reports
dump divergence, stop and reconcile before any write — the subsection
below says how, and which recovery is right is a **decision, not a
command**. Work then goes through verbs; commits reference tasks by
`#NN` and/or the epic slug in the message.

**End every session with a bookkeeping commit**, so the dump refreshed by
your last write reaches git — and stage the pair explicitly:

```sh
git add .selftracked/dump.sql STATE.md && git commit -m "Bookkeeping: ..."
```

The explicit `git add` matters: the pre-commit hook refreshes and stages
the pair itself, but when the index started empty, git refuses the commit
anyway — hook-staged content alone does not count.

### Reconciling dump divergence (§8.4)

Divergence means the tracked `.selftracked/dump.sql` and the local
database no longer agree. It has **two directions**, and they call for
opposite, irreversible moves: **taking the wrong branch is total loss in
exactly one of them**, because `load --force` on a good database destroys
every local write that never reached a commit. Decide first; write
nothing until you have.

**The test** needs no new surface — it is git, plus a read verb that is
safe while diverged:

```sh
git log -1 --stat -- .selftracked/dump.sql   # did another commit move it?
git status --short .selftracked/dump.sql     # or did this working tree?
selftracked prime                            # what does the database hold?
```

The rule you apply to those facts: **the good side is the one holding
work that exists nowhere else.**

**The tracked dump is the good side** — a `git pull` brought another
machine's newer state and the local database is stale. `load --force`
replaces the local database with the tracked dump (it prints what it
discards first); then re-apply any unsynced local writes through verbs
and re-run `prime`:

```sh
selftracked load --force
```

**The local database is the good side** — the working-tree dump was
clobbered by a checkout, a merge resolution or a hand edit, while the
database holds writes that never reached git. Discard nothing; re-derive
the tracked dump from the database:

```sh
selftracked dump
selftracked verify   # R1 red? run the `state` it names, verify again
```

`verify` may report `FAIL R1: STATE.md does not match the database (stale
projection); run selftracked state`. That is part of this procedure, not
a new failure — but it fires only when the clobber took `STATE.md` with
it, as a whole-tree checkout or a merge resolution does. When only
`.selftracked/dump.sql` was restored, `STATE.md` is still in step with
the database: `dump` alone clears the divergence and `verify` exits 0
with no R1. When R1 does fire, run the `state` the message names — that
message, not a scripted call, is the authority for the repair — then
`verify` again. Commit only once `verify` is green, so the commit
carries a current `STATE.md`. The commit is not optional: until it
lands, the divergence is unresolved on the git side.

```sh
git add .selftracked/dump.sql STATE.md && git commit -m "Bookkeeping: ..."
```

**Both sides hold unique work** — that is the two-writer accident §8.4
calls out below, and neither branch is safe: `load --force` discards this
machine's writes and `dump` overwrites the other machine's. Stop here and
reconcile the two histories deliberately.

Plain `load` only builds a missing database; with an existing one it
refuses.

## Where new work goes

Work that is not already in the backlog is classified **before the first
write**, never guessed at:

- Work that advances an **ACTIVE epic's goal** is a **story** under that
  epic. Opening one is a **scope change**: the owner authorizes it, and an
  implementing agent never authorizes its own.
- Work that does **not** advance an active epic's goal — a defect, a
  question, a follow-up, anything standalone — is a **task**
  (`create --title …`), homed to an epic with `--epic` only when it
  genuinely belongs to one.
- Work matching **no existing task, story or epic** is said out loud before
  the first write and the choice put to the owner, rather than worked
  off-book. `prime` names the epic-scoped case — an active epic with no
  story that can receive work — as a `no-workable-story` notice; the rest
  is your own judgement, unenforced, until the v0.1 interactive protocol.

"Advances the epic's goal" is a judgement, and a wrong call is cheap to
correct (`edit --epic` / `--detach`). The rule exists to stop the *silent*
guess, not to remove the judgement. Like the durable-doc rules below, it is
guidance in this generated file: no verb refuses on it. That is a statement
about the tool, not permission — unenforced is not optional, and where the
rule says the choice is the owner's, an agent's own classification is what
raises the question, never what settles it.

## Sync (§8.4)

Git is the only sync channel: `.selftracked/dump.sql` and the generated docs
travel through commits, and nothing else does. If this repo also lives under
a file-sync service (Dropbox, iCloud, a synced Drive), **exclude
`.selftracked/db.sqlite*` and `.selftracked/dump.hash` from it** — they are
per-machine, and syncing them races two machines' databases and manufactures
conflict copies. True two-writer accidents surface as textual conflicts in
`dump.sql` or duplicate-key `load` failures — loud by construction, never a
silent automatic merge (no merge driver ships).

## The rule (§11.2)

**State changes only through verbs.** Never run `sqlite3` against
`.selftracked/db.sqlite`, never hand-edit `dump.sql`. Write invariants are
enforced by the schema and detected by `verify`; raw *reads* are not
technically prevented, but every read verb is cheaper than the SQL it would
replace, so there is no reason to reach past them. The generated
`.claude/settings.json` carries a deny-list entry for `sqlite3` — a
convention backstop, not a hard boundary (a determined caller can still
read, which is why the honest mitigation is that the read verbs are the
easier path).

**PO (product-owner) decisions are never answered by an agent.** When a
choice belongs to the owner, raise it: move the question task to
`IN-REVIEW` and `story block --reason` the affected story. `story block
--reason` is the tool for *any* blocker, not only owner questions. One
waiting question then shows in three status surfaces the verbs keep in step
— the IN-REVIEW task, the story at `BLOCKED`, and its `BLOCKED-ON-OWNER`
worklog episode. That is one waiting state in three vocabularies by design,
not drift.

The literal token `PO:` (English, fixed for greppability) prefixes an owner
decision wherever one is recorded. It is deliberately locale-fixed: a
non-English crew adapts the literal in its prompt config once, never per
use, so a single `grep PO:` always finds every owner ruling.

## Verbs

Every verb accepts `--json` for machine-readable output; `--help` prints
its signature. Wherever a signature says `<ref>`, the grammar is: `NN` or
`#NN` for a task, `epic:SLUG` for an epic, `epic:SLUG/SID` for a story.
The catalog, with signatures:

- **Tasks:**
  - `create --title T [--status OPEN|IN-REVIEW|NEEDS-TRIAGE] [--note N]
    [--epic SLUG] [--label]` — default status is `NEEDS-TRIAGE`.
  - `edit <ref> [--title T] [--note N] [--goal G] [--dod D] [--consumes C]
    [--produces P] [--epic SLUG|--detach]`
  - `set-status <id> <STATUS> [--note N] [--dup-of ID]` — targets are the
    task statuses below; every `set-status` call REWRITES the status
    note, so pass `--note` (superseded notes survive in the events
    trail).
  - `reopen <id> --why TEXT` · `park <id> --why TEXT` · `unpark <id>`
  - `ready [--epic SLUG]` · `show <ref>` ·
    `list [--status S] [--epic SLUG] [--parked] [--labels]`
- **Relations & artifacts:**
  - `rel add <id> <depends|relates|supersedes> <id> [--note N]` ·
    `rel rm <id> <type> <id>` · `rel tree <id>` · `rel cycles`
  - `link <id|epic:SLUG> <class[@scope]:relpath> --role R` ·
    `link archive|unarchive <artifact-ref> [--force]` ·
    `unlink <id|epic:SLUG> <class[@scope]:relpath>`
- **Epics & stories:**
  - `epic create SLUG --goal G` · `epic activate SLUG` ·
    `epic pause SLUG --why TEXT` · `epic dissolve SLUG --why TEXT` ·
    `epic show SLUG` · `epic list [--active]` · `epic close SLUG`
  - `story add SLUG --title T` · `story ready SLUG SID` ·
    `story start SLUG SID` · `story block SLUG SID --reason TEXT` ·
    `story unblock SLUG SID --resolution TEXT` ·
    `story done SLUG SID --commits RANGE --gate G [--review R]` ·
    `story dissolve SLUG SID --why TEXT`
  - `worklog add SLUG --story SID|V-N --state ST [--corrects N]
    [--commits] [--gate] [--review] [--note]`
  - `criteria add SLUG --text T` · `criteria met SLUG SEQ --evidence E` ·
    `criteria check SLUG`
- **Paths & config:**
  - `paths ls` · `paths set CLASS[@SCOPE] ROOT [--ephemeral] [--note N]` ·
    `paths move CLASS[@SCOPE] NEWROOT [--with-files]`
  - `config ls` · `config set <production_globs|idle_days|prime_cap> VALUE`
- **Maintenance & state:** `prime` (the session-start read) ·
  `verify [--fast] [--quiet]` · `log <ref> [--limit N]` ·
  `stale [--since REF]` · `dump [--stdout]` · `load [--force]` · `state` ·
  `gate skip-mark` · `import --file F [--format md-table|json] [--legacy]`
  · `init`.

Configuration lives in `meta` rows edited only through `config`; there is
no config file.

## Status vocabulary

- **Tasks:** `OPEN` (workable; `parked` marks deferred-but-open) ·
  `IN-REVIEW` (awaiting the product owner, including tasks that ARE
  questions) · `NEEDS-TRIAGE` (the default on create; `prime` surfaces
  the triage queue) · `DONE` (note = what closed it) · `WONT-DO` (note =
  the reopen trigger) · `DUPLICATE` (requires `--dup-of` naming the
  canonical) · `LABEL` (reserved marker; no lifecycle). Transitions are
  matrix-checked: terminal→OPEN is `reopen`'s job, any transition clears
  `parked`, and any exit from `IN-REVIEW` requires `--note` carrying the
  owner's verdict.
- **Stories:** `PLANNED` → `READY` → `IN-PROGRESS` → `DONE`, plus
  `BLOCKED` (`story block`/`unblock`) and `DISSOLVED`. One `IN-PROGRESS`
  story per epic.
- **Epics:** `BACKLOG` → `ACTIVE` → `CLOSED`, plus `PAUSED` and
  `DISSOLVED`.

## When a verb refuses because the epic is closed

An epic that is `CLOSED` or `DISSOLVED` is **terminal**, and a terminal
epic refuses every write that would re-open one of the conditions it was
closed on: `criteria add`, `criteria met`, `story add`, every story
transition, `create --epic`, `edit --epic`, `reopen` of a task homed
there, and `edit` of the epic's goal or of a story's fields. Each of
them refuses with `{"code":"terminal"}`, exit 1. That refusal is the
design and not an obstacle to route around; you meet it most often right
after an `import`, when terminal epics arrive in bulk.

What stays open after a close is a **closed list** — deliberately
unnumbered, because it has grown twice already:

- **`V-n` rows** — `worklog add SLUG --story V-N …`, post-close
  validation on a `CLOSED` epic. This is where post-close work goes.
- **`--corrects` correction rows** — `worklog add … --corrects N`, the
  append-only correction of an earlier row, on any story.
- **Artifact links** — `link` on an `epic:` target: a retrospective
  attached to a closed epic breaks no condition.
- **`edit --detach`** — un-homing a task. It removes a violation rather
  than creating one, and it is the repair the `reopen` refusal names.
- **`story dissolve SLUG SID --why …` of a non-terminal story, on a
  `CLOSED` epic only.** `DISSOLVED` is the one story target that
  *satisfies* a close condition instead of breaking it, so like `edit
  --detach` it removes a violation. The restriction is the asymmetry
  worth remembering: a `DISSOLVED` epic never passed a close gate, so it
  holds no claim to restore and this carve-out does not reach it. By the
  same reasoning it does not reach an epic that arrived `CLOSED` through
  `import` either — the gate it never passed is this tracker's own.

And the route that is not a route: **there is no `epic reopen`, ever.**
A goal that revives becomes a new epic; the closed one keeps its record.

## The language of what you write

Tracker content — titles, notes, epic goals, story DoD, `consumes` /
`produces`, worklog notes, resolutions, criteria text — is written in
**one language, chosen once for the repository**, and **English is the
default this contract ships with**. The reasons are properties of this
artifact, not general advocacy:

- `dump.sql` is published and permanent: it carries every title, note
  and verdict into git, where append-only history makes them permanent
  and redaction tooling is deferred.
- The `PO:` token above is already locale-fixed for greppability, so a
  half-and-half tracker splits its own greppability — the marker stays
  findable while the ruling beside it stops being so.
- A reader of a published dump should not need a second language to
  follow one repository end to end.

This is a **default, not a prohibition**. A crew that chooses another
language records that choice in its own project memory and applies it
uniformly; what a mixed tracker costs is what the three reasons
describe, and it is paid whether the mixture was chosen or drifted into.
It is ungated by construction: no verb refuses a non-English title and
none is proposed, because a text-language check is not mechanically
sound — identifiers, quoted output and proper nouns all defeat it. As
with the durable-doc rules below, unenforced is not optional.

## Durable-doc authoring rules

`STATE.md` and the database are generated surfaces. When you write durable
prose (READMEs, ADRs, research docs, reports), three rules keep it from
drifting. Honest status first: these are **authoring guidance in this
generated file, not gated conventions**. Prose files are a stated v0
non-goal for machine checking; rule 2's validator is deferred; the
project's executable-gates principle governs database state, not doc
authoring. Guidance is what a disciplined crew follows anyway:

1. **Prose never duplicates DB-enumerable state** (counts, id or status
   lists) — cite the verb (`list`, `show`, `log`) or `STATE.md` instead.
   Every prose copy of an enumerable list drifts silently the day the
   enumeration changes.
2. **A DB-derived number that must live in a durable doc is anchored** `as
   of dump <sha12>` — the first 12 hex of the SHA-256 of the last
   **committed** dump:
   `git show HEAD:.selftracked/dump.sql | shasum -a 256`, then truncate to
   12 hex. Anchor the committed blob, not the working-tree file:
   mid-session the tree holds a dump state that may never reach a commit,
   and an anchor to it is unverifiable forever (the deferred validator
   checks against committed history). Freshness corollary: a number derived
   from THIS session's writes has no committed epoch yet — state trails git
   by one commit — so commit first, anchor after; anchoring it to HEAD's
   dump cites an epoch that cannot contain it. The gitignored divergence
   sidecar (`.selftracked/dump.hash`) is NOT a shortcut for this digest: it
   tracks the working-tree dump and during any session with pending writes
   is routinely one commit ahead of HEAD (and fresh clones have none; a
   crash window can leave a stale one). A bare date is too coarse an anchor
   the day the DB changes twice. Honesty note: the anchor pins the *epoch*,
   not the arithmetic — a wrong number beside a fresh hash still needs
   review to catch.
3. **Event dates and date-bearing filenames come from the system clock**
   (`date`), never from the session narrative — the verbs already enforce
   this for database rows, but agents also *name* dated files (research
   docs, reports), and a wrong date baked into a filename is permanent:
   filenames are identifiers, and renaming breaks every recorded reference.

## Conventions

- **CHANGELOG:** keep a human-facing `CHANGELOG.md` in the
  [Keep a Changelog](https://keepachangelog.com/) format if the project
  ships releases — distinct from the tracker's own history.
- **Repo maintenance:** run `git repack` periodically. A rewritten,
  multi-MB `dump.sql` leaves one full blob per commit between repacks, so a
  busy tracker's `.git` grows until repacked.

## Privacy & publication

`dump.sql` and `STATE.md` publish task titles, notes, PO decisions, and
worklog text — including `events.detail` payloads (verdicts, resolutions,
and `edit` old/new-value prefixes that accumulate append-only across every
historical edit) and user-authored fields (criteria commands, DoD,
consumes/produces) that may contain local paths if written carelessly —
whenever the repo is public. Append-only history plus git make this
permanent; redaction tooling is deferred. Review `git diff --staged` before
pushing a sensitive repo. selftracked's own verbs never write hostnames,
usernames, or absolute paths.
