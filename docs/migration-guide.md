# Migrating an existing project into selftracked

This is the generic guide for adopting selftracked on a repository that
already tracks its work in prose — backlog tables, epic files, campaign
documents. It is written against the `import` verb as shipped in v0 and
was walked against the importer's own fixture corpus; the field names and
behaviors below are the ones the code accepts, not an aspiration.

The one-line summary: **derive a corpus from your prose, rehearse the
import against a disposable clone until `verify` is green, then install
and import for real.** Migration is partial by design; the boundary is
stated below so nothing is lost by surprise.

## 1. The adoption posture

selftracked colocates: it never replaces your project's existing gates or
layout. `init` scaffolds `.selftracked/` plus a handful of generated
documents, appends (never rewrites) `.gitignore` entries, and **never
overwrites a file that already exists**. If your repository already runs
git hooks — a set `core.hooksPath` or a live pre/post-commit — `init`
detects that and prints a chaining recipe instead of the takeover
command: your gates stay authoritative, selftracked's hook runs as a
subprocess call added to yours. Abandonment is deleting the
`.selftracked/` directory — plus, if you chained, removing the one line
you added to your own hook (a call to a deleted script would otherwise
fail your hook loudly).

## 2. What migrates, and what does not

**Migrates** (through `import`):

- Epics: slug, goal, status (including terminal states), status note,
  close sweep, acceptance criteria with met/evidence.
- Stories per epic: id (`S1`, `S2`, …), title, status, DoD,
  consumes/produces.
- Tasks: title, status (including `DONE`, `WONT-DO`, `DUPLICATE` with
  its canonical, `IN-REVIEW`, `NEEDS-TRIAGE`), a free-text note, an
  optional owning epic, an optional explicit date. A task slated as a
  future increment sets `future_increment: true` and MUST name its
  epic — the importer maps "planned for later inside this epic" to epic
  homing, never to `park`.
- Worklog episodes: per-epic rows with state, cited commits, gate and
  review evidence, notes.
- Path-dictionary rows (class/scope/root).

**Does not migrate** — this belongs to the prose layer and stays there:

- Prose registries and narrative planning documents themselves. The
  corpus is *derived from* them; the documents remain the historical
  record in your repository.
- Per-file status headers and their index-sync gates.
- Pointers to non-file targets — they degrade to free-text notes on
  import, never to typed references.
- Owner steers recorded only in campaign prose, where the unblocked work
  was never modeled as a blocked story: they import as notes; no unblock
  event is synthesized, because there is no block row to resolve.

## 3. Reconcile before you import

The tracker's close gates are stricter than hand-kept files, by design.
Source inconsistencies must be resolved in the corpus **before** import,
not imported as-is: a closed epic whose cards were never flipped, a
duplicate pointing at a task that is itself a duplicate, a story status
your prose contradicts. The importer refuses what it can detect (dup
chains, unknown statuses, out-of-bounds dates); what it cannot detect
becomes tracker state you will be correcting through verbs later —
reconciling first is cheaper.

Two authoring constraints found in real rehearsals:

- A `DUPLICATE` task's canonical must precede it in the corpus: `dup_of`
  is the canonical's assigned integer id, and on a fresh instance ids are
  assigned in corpus order (the canonical's 1-based position). It is
  resolved against already-inserted tasks, so a forward reference
  refuses.
- The md-table format cannot express a bundled increment; split such
  rows per increment, or author in JSON.

## 4. The corpus

`import --file corpus.json --format json [--legacy]` (md-table is the
other reader). The JSON shape, exactly as the fixture corpus exercises
it:

```json
{
  "paths": [{"class": "src", "scope": "web", "root": "web/src"}],
  "epics": [{
    "slug": "demo", "goal": "demonstrate the backfill",
    "status": "CLOSED", "close_sweep": "2026-01-05",
    "criteria": [{"criterion": "owner attests the migration",
                  "met": true, "evidence": "owner noted"}]
  }],
  "stories": [
    {"epic": "demo", "id": "S1", "title": "first story", "status": "DONE"}
  ],
  "tasks": [
    {"title": "a backfilled task", "status": "DONE",
     "note": "closed: shipped in the backfill", "epic": "demo"}
  ],
  "worklog": [
    {"epic": "demo", "story": "S1", "state": "DONE",
     "commits": "<sha>", "gate": "go test ./..."}
  ]
}
```

Dates are **git-first**: a worklog row citing real commits gets its date
from git (author date of the newest cited commit), out-voting any
narrative date — a calendar-day disagreement is warned about and the
warning is corpus-audit material. An explicit `date` field is an
ordinary source for any import. Cite only strings that actually resolve
as commits in the repository: a sha-shaped string that does not resolve
imports silently and fails later at `verify` (R5).

## 5. `--legacy` — the three relaxations

Historical records cannot satisfy everything the schema demands of new
ones. `--legacy` relaxes exactly three things, nothing else:

1. Timestamps for entities with no git anchor are synthesized (and
   marked in the events trail).
2. Done work without a recoverable commit range is recorded as
   `commits="legacy: <why>"` — visible and accepted rather than blocking
   closure forever. A worklog row's `legacy_reason` field supplies the
   why; without it the row records "imported without commit range".
3. Terminal states (epics: `CLOSED`, `DISSOLVED`; tasks: `DONE`,
   `WONT-DO`, `DUPLICATE`) may be inserted directly; the transition
   matrices gate only the UPDATE path.

Every imported entity still gets its events row, so the audit trail
stays whole. Synthesized timestamps carry the import's wall clock:
re-importing from scratch produces equal state but not byte-identical
dumps — only git-anchored rows are stable across rounds.

## 6. Rehearse against a disposable clone

Never rehearse against the live repository. Keep a disposable local
clone under a gitignored path, refresh it by pull, and import from
scratch each round:

```sh
git clone --no-hardlinks /path/to/project work/local/pilot-client
cd work/local/pilot-client
selftracked init
selftracked import --file ../corpus.json --format json --legacy
selftracked verify        # the round's gate: 0 violations
```

Repeat — delete `.selftracked/`, re-init, re-import — until `verify` is
green and the advisories are ones you have read and accepted. Importer
refusals during rehearsal are findings: file them, fix the corpus (or
the importer), re-run. Real git objects in the clone make the git-first
date engine behave exactly as it will on the live install.

## 7. The live install

Only after the importer round-trips the full corpus with `verify` green:

```sh
cd /path/to/project
selftracked init          # detects incumbent hooks, prints the recipe
selftracked import --file corpus.json --format json --legacy
selftracked verify
```

Then chain the hooks per the printed recipe (subprocess calls added to
your existing hooks — never `source`), and confirm your own gate still
passes end to end. Everything `init` and the chaining touched is left
for your review as ordinary working-tree changes; commit them when
satisfied. From here on, state changes go through verbs only — `prime`
is the session-start read, and the generated `PROMPT.md` carries the
working contract.

## 8. The adaptation handoff — agent-executed onboarding

`init` is deliberately mechanical; everything after it — chaining into
the host's gates, deriving the corpus, deciding what the dictionary
should describe — is judgment work. On an agent-driven project the
natural executor is the project's own agent, and the deliverable that
makes the adoption repeatable is not a script but a **handoff
instruction** the agent executes. Rehearsed handoffs share a shape:

- **One fresh copy per attempt.** Each rehearsal starts from a pristine
  copy of the host (a copy-on-write clone takes the whole tree, uncommitted
  prose included, at near-zero cost where the filesystem supports it) and is thrown away, never repaired.
- **Exact chaining lines with their positions and the why.** The host's
  own gates keep running first in pre-commit (they own the repo);
  selftracked's post-commit line goes at the top because a host
  post-commit may exit early. Quote the guarded lines `init` printed.
- **An expectation at every step**: exit codes, the `imported N epic(s),
  …` counts line, and the advisory census `verify` should print — class
  and count ("N × R13 and nothing else"). A silent-on-success host gate is
  stated as such, or the executing agent will report its silence as a
  deviation. Any output outside the stated expectation is a finding, not
  something to recover from.
- **A fidelity table**: expected per-status task counts and per-table row
  counts derived from the source prose by independent counting, plus the
  exact commands to reproduce them against the imported dump.
- **The bookkeeping-commit command with explicit staging** (`git add
  .selftracked/dump.sql STATE.md && git commit …`): git refuses a commit
  whose only content was staged by the pre-commit hook when the index
  started empty.
- **A corpus-freshness rule.** A corpus is a derivative of a snapshot;
  when the live prose has moved since, re-derive or delta-check before
  the live install.
- **A deferred-to-owner list**, so the agent does not improvise
  decisions: dictionary scoping, artifact-link sweeps, and the live
  install itself stay with the owner.

**Sufficiency criterion:** a fresh-context agent given ONLY the handoff
reaches `verify` green with the fidelity table matching, no improvised
recovery and no questions asked. Validate by actually running one — twice,
on two fresh copies — before calling the handoff done.

**Multi-subproject hosts.** One repository still means one tracker; the
partition lives in the data: scoped dictionary rows (`paths set
class@scope root`) describe each subproject's real doc roots, epics carry
a subproject prefix in their slugs, and a subproject that is no longer
active imports as terminal records (`park --why` for what remains). Imported
backlog rows reference their prose homes as text, which `verify` surfaces
as R13 advisories; `link <id> class@scope:relpath --role home` retires
them row by row. Both are safely deferred until after the import lands.
