# selftracked — agent instructions

This project tracks its own work in a local SQLite database, dumped
deterministically to `.selftracked/dump.sql` (the one tracked, reviewed,
synced surface) and projected to `STATE.md`. You interact with it through a
fixed set of verbs — never by touching the database or the dump directly.

## The working loop

**Start every session with `prime`** — the session-start read: active
epics with their unmet criteria, sprint goals (every IN-PROGRESS story),
and the ready / triage / in-review / stale / parked queues. If it reports
dump divergence, stop and reconcile (`load`, then re-run `prime`) before
any write. Work then goes through verbs; commits reference tasks by `#NN`
and/or the epic slug in the message.

**End every session with a bookkeeping commit**, so the dump refreshed by
your last write reaches git — and stage the pair explicitly:

```sh
git add .selftracked/dump.sql STATE.md && git commit -m "Bookkeeping: ..."
```

The explicit `git add` matters: the pre-commit hook refreshes and stages
the pair itself, but when the index started empty, git refuses the commit
anyway — hook-staged content alone does not count.

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
