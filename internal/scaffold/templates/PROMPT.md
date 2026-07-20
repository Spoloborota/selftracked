# selftracked — agent instructions

This project tracks its own work in a local SQLite database, dumped
deterministically to `.selftracked/dump.sql` (the one tracked, reviewed,
synced surface) and projected to `STATE.md`. You interact with it through a
fixed set of verbs — never by touching the database or the dump directly.

## The rule (§11.2)

**State changes only through verbs.** Never run `sqlite3` against
`.selftracked/db.sqlite`, never hand-edit `dump.sql`. Write invariants are
enforced by the schema and detected by `verify`; raw *reads* are not
technically prevented, but every read verb is cheaper than the SQL it would
replace, so there is no reason to reach past them.

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

Run any verb with `--json` for machine-readable output; `--help` prints its
signature. The catalog:

- **Tasks:** `create`, `edit`, `set-status`, `reopen`, `park`, `unpark`,
  `ready`, `show`, `list`.
- **Relations & artifacts:** `rel`, `link`, `unlink`.
- **Epics & stories:** `epic`, `story`, `worklog`, `criteria`.
- **Paths & config:** `paths`, `config`.
- **Maintenance & state:** `log`, `stale`, `dump`, `load`, `verify`,
  `state`, `prime`, `init`.

Configuration lives in `meta` rows edited only through `config`; there is
no config file.

## Durable-doc authoring rules

`STATE.md` and the database are generated surfaces. When you write durable
prose (READMEs, ADRs, research docs, reports), three rules keep it from
drifting. Honest status first: these are **authoring guidance in this
generated file, not gated conventions**. Prose files are a stated v0
non-goal for machine checking; rule 2's validator is deferred; the
project's executable-gates principle governs database state, not doc
authoring. Guidance is what a disciplined crew follows anyway:

1. **Prose never duplicates DB-enumerable state** — counts, id or status
   lists. Cite the verb (`list`, `show`, `log`) or `STATE.md` instead.
   Every prose copy of an enumerable list drifts silently the day the
   enumeration changes.
2. **A DB-derived number that must live in a durable doc is anchored** `as
   of dump <sha12>` — the first 12 hex of the SHA-256 of the last
   **committed** dump:
   `git show HEAD:.selftracked/dump.sql | shasum -a 256`, truncated to 12
   hex. Anchor the committed blob, not the working-tree file: mid-session
   the tree holds a dump state that may never reach a commit. A number
   derived from this session's writes has no committed epoch yet — commit
   first, anchor after. The gitignored divergence sidecar
   (`.selftracked/dump.hash`) is NOT a shortcut for this digest. A bare
   date is too coarse the day the DB changes twice.
3. **Event dates and date-bearing filenames come from the system clock**
   (`date`), never from the session narrative. The verbs enforce this for
   database rows; you also *name* dated files, and a wrong date baked into
   a filename is permanent — filenames are identifiers, and renaming breaks
   every recorded reference.

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
