# Migrating an existing project into selftracked

This is the generic guide for adopting selftracked on a repository that
already tracks its work in prose — backlog tables, epic files, campaign
documents. It is written against the `import` verb as shipped in v0 and
was walked against the importer's own fixture corpus; the field names and
behaviors below are the ones the code accepts, not an aspiration —
**except for the one named here**, which the specification carries at
revision 3.42 and the binary does not have yet:

- the criteria corpus field is spelled **`criterion`** by the shipped
  importer, not `text` as the JSON example in §4 shows. `text` becomes
  the name and `criterion` an accepted alias; until then a corpus using
  `text` is refused as an unknown field.

It is a story of the epic that produced this revision of the guide. This
notice goes away with it; if you are reading it, it has not landed.

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
chains, unknown statuses, out-of-bounds dates, malformed identifiers);
what it cannot detect becomes tracker state you will be correcting
through verbs later — reconciling first is cheaper.

Identifiers are the class worth renaming ahead of time. Every epic slug
must be kebab-case and every story id `S<number>` (`V-<number>` for a
post-close worklog row) — on the identifiers your corpus declares and on
the ones it only references from a story, a task or a worklog row. A
legacy tool's `Auth_2024` is refused by name and left alone: import
never rewrites an identifier for you, because a silently kebab-cased
slug is your data changed without your say. This is not one of the three
relaxations in section 5, and `--legacy` does not admit it.

Two authoring constraints found in real rehearsals:

- A `DUPLICATE` task's canonical must precede it in the corpus: `dup_of`
  is the canonical's assigned integer id, and on a fresh instance ids are
  assigned in corpus order (the canonical's 1-based position). It is
  resolved against already-inserted tasks, so a forward reference
  refuses.
- The md-table format cannot express a bundled increment; split such
  rows per increment, or author in JSON.

## 4. The corpus

`import --file corpus.json --format json [--legacy]`. There are two
readers and they do not carry the same corpus — **md-table has no
`criteria` section, so epic acceptance criteria are JSON-only** and an
adopter who authors in md-table loses them by choosing the format.
Section 4.1 states the md-table shape in full; the JSON shape, exactly
as the fixture corpus exercises it, is:

```json
{
  "paths": [{"class": "src", "scope": "web", "root": "web/src"}],
  "epics": [{
    "slug": "demo", "goal": "demonstrate the backfill",
    "status": "CLOSED", "close_sweep": "2026-01-05",
    "criteria": [{"text": "owner attests the migration",
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

**A corpus that inserts nothing at all is refused** — in either format,
because the check sits above both readers rather than inside one. The
refusal distinguishes its two reasons, since they call for different
actions: nothing was recognized in the file (an md-table whose headings
are mis-cased or absent, an empty JSON object), or every recognized
section was found and empty. The first is a syntax problem to hunt for;
the second is not.

Dates are **git-first**: a worklog row citing real commits gets its date
from git (author date of the newest cited commit), out-voting any
narrative date — a calendar-day disagreement is warned about and the
warning is corpus-audit material. An explicit `date` field is an
ordinary source for any import. Cite only strings that actually resolve
as commits in the repository: a sha-shaped string that does not resolve
imports silently and fails later at `verify` (R5).

### 4.1 The md-table corpus

`import --file corpus.md --format md-table [--legacy]`. Decide this
first: **md-table carries no `criteria` section.** Epic acceptance
criteria cannot be expressed in it at all, and nothing warns you — the
import simply lands an epic with no criteria rows. If your epics carry
criteria, author in JSON.

What md-table does carry is five sections, one markdown table each,
each introduced by a `## ` heading. The headings are **lower-case and
exact**: `## Tasks` is not a spelling variant, it is refused with
`unknown section "Tasks"`. A heading may appear at most once — a
repeated section is refused as a duplicate — and a known section that is
blank or omitted is fine **as long as another one carries rows**, under
section 4's empty-corpus refusal. The column names are the JSON field
names:

| Section heading | Columns |
|---|---|
| `## paths` | class, scope, root, ephemeral, note |
| `## epics` | slug, goal, status, status_note, close_sweep, created_at |
| `## stories` | epic, id, title, status, dod, consumes, produces |
| `## tasks` | title, status, note, epic, date, dup_of, future_increment, pointer_note, owner_steer |
| `## worklog` | epic, story, state, commits, gate, review, date, note, legacy_reason |

A column outside its section's list is refused, as is a data row whose
cell count does not match its header. Section 3's two authoring
constraints apply here rather than being restated — the md-table-specific
one of the pair is that a bundled increment cannot be expressed, so such
rows are split per increment or authored in JSON.

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

A local `git clone` sets the new repository's `origin` to the **source
path** — the directory you cloned from, not whatever remote that
directory pushes to. So a stray push out of a rehearsal clone lands in a
local repository and publishes nothing; the exposure it does have is
that the clone holds a full copy of your project's history, which is why
it lives under a gitignored path and is deleted when the rehearsal ends.
A copy made any other way does not have this property — see section 8.

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

### 7.1 Importing again, into a tracker that already has closed epics

A repeat import — a delta round, a second pass after the corpus moved —
can insert rows under an epic a verb has already closed. The importer
accepts it **by design**: it inserts rows and evaluates no close
condition, so a corpus that omits an epic from its `epics` list and
merely references the slug from a story, a task or a worklog row lands
those children under the closed epic and exits 0. (A corpus that
re-declares the epic is refused `epic-exists`; the seam is exactly the
children-only shape.)

The result is an **R16 advisory** — a CLOSED epic no longer satisfying
the conditions it was closed on. Two consequences worth stating
plainly:

- An advisory does not stop a commit and does not fail `verify`'s exit
  code. The person who must notice is whoever runs `verify` after the
  import, which is where the section 8 advisory census belongs.
- Every R16 finding names its own repair: a homed task in
  OPEN/IN-REVIEW/NEEDS-TRIAGE clears with `edit <id> --detach` or by
  going terminal, and a non-terminal story clears with
  `story dissolve <slug> <sid> --why …`.

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
- **Severing the copy's inherited remote is the first act on it** —
  before `init`, before the corpus. A `cp`-class copy duplicates `.git`
  wholesale, so unlike the section 6 clone it inherits the *source's*
  configured remote verbatim; on a machine with cached git credentials
  that is one stray push away from publishing rehearsal state to a live
  remote. The step and its expectation, in the same form as every other
  step here: `git -C <copy> remote remove origin` exits 0, and
  `git -C <copy> remote -v` then prints nothing.
- **Exact chaining lines with their positions and the why.** The host's
  own gates keep running first in pre-commit (they own the repo);
  selftracked's post-commit line goes at the top because a host
  post-commit may exit early. Quote the guarded lines `init` printed.
- **An expectation at every step**: exit codes, the `imported N path(s),
  N epic(s), …` counts line, and the advisory census `verify` should print — class
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
