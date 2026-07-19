# selftracked v0 — Specification

Status: **DRAFT, revision 3.15 — for owner review.** Revision history: rev 1 →
five-lens adversarial critic round + paper-migration fit analysis + two
research passes (see `docs/research/`) → rev 2 → second critic round with
empirical schema testing + delta fit analysis → rev 3 → third (convergence)
critic round + final fit analysis → rev 3.1 → owner decisions closing D3/D6
(rev 3.2) → five follow-up research passes folded in with owner approval
(Go stack, migrations, pilot/testing flow, scaffold taxonomy, long-horizon
scale — D11–D15, see `docs/research/`; rev 3.3) → fourth critic round (six
lenses, including the new stack/migrations/scale topics) + delta fit
analysis (rev 3.4) → fifth (control) critic round, same six lenses on
cheaper models, + delta fit analysis (rev 3.5) → five doc-drift incident
classes observed in the parent project (narrative-sourced dates, prose
duplicating enumerable state, date-only epoch anchors, variant-blind
textual matchers, unwritten conventions and decisions living only in chat)
mapped onto the spec (rev 3.6) → sixth critic round (same six lenses +
fit) on that mapping (rev 3.7) → seventh round, same stack, controlling
the fixes (rev 3.8) → eighth (convergence) round, same stack (rev 3.9) →
rev 3.9 → implementation of S1a found §5's nullable-column claim
contradicting the schema §5 itself defines, while §8.1 had it right
(amendment `nullable-columns-preamble`) → rev 3.10 → import date bounds,
after deriving task dates from git was tried against a real corpus and failed
(amendment `import-date-bounds`) → rev 3.11 → the S1b stage-open re-read
found §5.7's worklog.story comment naming R5 as its guard where the rule
is R4 (amendment `worklog-story-guard-rule-pointer`) → rev 3.12 → the
same re-read found §2's ≥2-stories definition enforced nowhere, and the
owner chose enforcement at the close boundary (amendment
`epic-close-story-cardinality`) → rev 3.13 → S5a implementation hit the
three-way contradiction between the link tables' no-delete triggers, the
verbs that must delete link rows, and R7 — the link tables lose their
triggers, entities keep theirs (amendment
`link-tables-are-relations-not-history`) → rev 3.14 → S5b found the
spec obliging `paths`/`config` to write events R8 must then flag — no §4
form exists for their subjects; instance-scoped events are carved out of
R8 by event type (amendment `instance-scoped-events-and-r8`) → this
revision. Implementation has
started; §5's schema and §3.1's connection posture are built.

Markers: **[DECIDED]** — settled by the owner. **[RESOLVED-BY-EVIDENCE]** —
adopts the verdict of documented research (cited); owner can veto.
**[BLOCKED: PO decision]** — only the owner can close (collected in §15).

Provenance: the design derives from (a) documented failure/success modes of a
private parent project that ran a file-based self-documenting system under AI
agents with machine-checked gates (qualitative observations only), and
(b) public research with sources, preserved as dated documents in
`docs/research/`. Empirical claims below cite those documents; every claim
that depends on the Go driver is additionally re-proven in the implementation
phase (§16) before being relied on.

---

## 1. What this is and why

**selftracked** is a local-first, git-native self-tracking and
self-documentation core for repositories developed by a small crew (one or
two people) through AI agents: a strictly local Jira + Confluence whose
primary *user is the agent*. The agent must always be able to find the full
state of the project, and must update that state as a side effect of the work
itself.

1. **A relational model deserves a relational store.** File-based
   self-tracking converges on Markdown tables modeling tasks, epics, typed
   links, status enums and counters — plus an ever-growing lint layer that
   polices what a database enforces natively: statuses drifting between file
   and index, multi-file status edits, colliding id namespaces, cross-reference
   confabulations surviving until an audit. (Each of these is a documented
   occurrence in the parent project, not a hypothetical.)
2. **Agents must not write SQL.** Agents hallucinate schema details; they get
   a closed verb set with `--json` everywhere.
3. **Single writer at a time; git is the only sync channel.** A violated
   axiom surfaces loudly (§8.4) — never as a silent merge. The niche's most
   popular tool abandoned single-writer for a multi-writer backend and its own
   changelog and issue tracker document what that cost its small-crew users;
   an ecosystem of successor tools built by departing users followed —
   including a widely-adopted fork frozen at the single-writer architecture
   (sources: `docs/research/2026-07-17-market-landscape.md`).
4. **Executable gates over prose rules.** Every convention ships as
   template/verb + machine check; a prose rule without a failing gate is an
   anti-pattern. (Scope: conventions about DB state and tracked structure.
   Durable-doc *authoring* guidance is prose by nature and is labeled as
   such where it appears — §9.)
5. **Closed vocabularies; one home per status.** "Unknown" is a legal status.
6. **History is moved, never deleted** — append-only audit; archive semantics.
   (Privacy consequences in a public repo: §14.)
7. **The path dictionary**: artifact classes (optionally scoped) map to
   filesystem roots; references are `class[@scope]:relpath`; moving a
   directory is a one-row update. No comparable tool has this.
8. **Harness-friendly core, Claude Code-first integration.** **[DECIDED]**

### 1.1 The integrity model — stated honestly

Two layers, with different strength, and the spec never conflates them:

- **Schema-enforced** (binds *any* process that opens the DB): CHECK
  constraints, the WIP unique index, STRICT types, NOT NULL. (Foreign keys sit
  one tier lower: enforcement is per-connection and off by default in SQLite —
  our DSN turns it on for every driver connection, and `verify` runs
  `foreign_key_check` to catch what a foreign process wrote with FKs off.)
- **Trigger-enforced** (binds the driver's connections and any casual/
  accidental raw write; a *deliberate* raw-SQL writer can bypass triggers via
  INSERT paths — which import legitimately uses — or by unsetting session
  pragmas): append-only protection, transition matrices, verb-gating flags.

Against a deliberate raw-SQL writer, prevention is impossible in an embedded
database; **detection** is the contract instead: the events-trail cross-check
(R12), the deterministic dump as a git-reviewed surface, and git history make
forged state *visible*, not impossible. The no-raw-SQL rule (§11) is therefore
prose backed by: schema teeth for accidents, detection for adversaries, and
the fact that every write verb is cheaper than the SQL it replaces.

Non-goals for v0: web UI; multi-writer merge (never, by thesis); doc-lint
for *prose* files (explicit boundary — §10/§17); MCP (v0.1+, §15); editing
prose documents.

---

## 2. Terminology

| Term | Meaning |
|---|---|
| **Task** | Atomic tracked unit, id `#NN`; lives in the backlog (single status home). |
| **Epic** | A goal decomposing into ≥2 stories; carries goal, machine-checkable acceptance criteria, stories, worklog, retro/close. |
| **Story** | One increment of an epic (`S1`…); DoD is a command/invariant, never prose. |
| **Worklog** | Append-only ledger of execution episodes; done is proven by a commit range a fresh agent can `git show`. The worklog is *history*; `stories.status` is the one current-state surface. |
| **Retro/close** | Atomic close-out: executable criteria check + status sweep + dated stamp, one transaction. |
| **Artifact** | A file/directory referenced as `class[@scope]:relpath`. |
| **DoR** | A story starts only with an empty `blocked` field. |
| **WIP=1 per epic** | At most one story IN-PROGRESS per epic — a schema index, not a rule. The limit is deliberately per-epic; the *cross-epic* discipline (ideally one active goal at a time) is the skill's job and `prime` surfaces every sprint goal so nothing hides. |
| **Backlog refinement** | The recurring triage loop (`prime` → triage queue → statuses/parking updated) — the skill names and drives it. |
| **PO** | The human. Questions to the PO are tasks in `IN-REVIEW` (a lifecycle state, not a kind). |

---

## 3. Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Claude Code integration (thin): SessionStart prime hook ·   │
│ git pre/post-commit hooks · rule (verbs-only) · skill       │
├─────────────────────────────────────────────────────────────┤
│ CLI: one Go binary, fixed verbs, --json everywhere          │
├─────────────────────────────────────────────────────────────┤
│ Store: SQLite via modernc.org/sqlite ≥ v1.48.2 (pure Go,    │
│ CGO_ENABLED=0); schema carries the gates (§1.1, §5)         │
│   .selftracked/db.sqlite    (local, gitignored)             │
│   .selftracked/dump.hash    (local, gitignored — §8.4)      │
├─────────────────────────────────────────────────────────────┤
│ Dump: canonical SQL text, committed to git — review surface │
│ and the ONLY sync channel: .selftracked/dump.sql (tracked)  │
└─────────────────────────────────────────────────────────────┘
```

### 3.1 SQLite posture (normative)

- Driver `modernc.org/sqlite`, minimum **v1.48.2** (earlier versions lost
  `RETURNING` rows through `Exec` — see
  `docs/research/2026-07-18-sqlite-advanced-features.md`; we use
  `Query`/`QueryRow` for all RETURNING statements regardless).
  `modernc.org/libc` pinned to the driver's go.mod version.
- DSN pragmas on every pooled connection: `foreign_keys(1)`,
  `recursive_triggers(1)` (without it `INSERT OR REPLACE` bypasses
  delete-triggers — research doc, ibid.), `trusted_schema(0)`,
  `busy_timeout(5000)`, `_dqs=0`.
- Write connections add `locking_mode(EXCLUSIVE)` + `synchronous(FULL)`.
  Semantics stated precisely: the exclusive OS lock is acquired at the
  connection's **first write** and held until the connection **closes**; it
  excludes all other processes from that point. Verbs are short-lived, but a
  concurrent read verb can receive `SQLITE_BUSY` for the duration of a write
  verb (including its dump-regeneration tail) — both sides retry within
  `busy_timeout`, and a final BUSY is exit 2 `{"error":{"code":"busy"}}`.
- Journal: default rollback journal + FULL sync; **WAL explicitly not used**
  (persistent mode flag, `-wal`/`-shm` litter, no network-FS support). Crash
  recovery = SQLite hot-journal rollback + §8.3/§8.4.
- Read verbs: `query_only(1)`. Write verbs end with `PRAGMA optimize`.
- `application_id` = project magic; `user_version` = schema version
  (mirrored in `meta`).

### 3.2 Implementation stack (normative) **[DECIDED]** (D11)

Exact pins and evidence: `docs/research/2026-07-18-go-stack.md` (every
version verified against official sources; pinned exact, never `latest`).

- Toolchain: go.mod names the exact current stable in its `toolchain`
  directive (go1.26.x at spec date), and — because under `GOTOOLCHAIN=auto`
  that directive only ever upgrades, never downgrades — CI and the Makefile
  export `GOTOOLCHAIN=go<exact>` for the true pin. The `go` directive
  (current−1) is the language *and* minimum-toolchain floor. Modern-idiom
  gate: `go fix -diff ./...` must produce **empty output and exit 0** (the
  Go 1.26 in-toolchain modernizers). Empirically the exit code does track
  diff emptiness (1 on pending fixes — verified on go1.26.x) but that
  behavior is undocumented, so the gate checks both and must not swallow the
  exit status (a parse error exits non-zero with an *empty* stdout).
- **No CLI framework**: stdlib `flag` + a hand-rolled verb registry. With a
  closed ~24-verb set, JSON-only errors, and the strict 0/1/2 exit contract
  (defined in §6.1), any framework (cobra/urfave/kong evaluated) costs more
  in suppressing its own usage/error behavior than dispatch costs to write.
  Three parsing obligations the stdlib choice creates (the honest price of
  "no framework", each cheap once, in the shared dispatcher): (a) verb
  signatures put fixed positionals BEFORE flags (§6.2), and `flag.Parse`
  stops at the first non-flag token — so the dispatcher checks the token
  count against the (verb, subverb) declared positional arity **in both
  directions first** (too few = usage refusal, never an index panic), pops
  the positionals, parses the remainder, and **refuses (usage error) when
  `fs.Args()` is non-empty afterward** — leftover tokens are never silently
  dropped (stdlib parses them into `Args()` with no error — empirically
  verified; a literal `--` in the remainder is honored by stdlib as the
  flags terminator); (b) FlagSets run `flag.ContinueOnError` with the
  FlagSet's output discarded (`SetOutput(io.Discard)`) **and** a suppressed
  `Usage` — overriding `Usage` alone does NOT silence the package's own
  one-line error prints (`failf` writes to `Output()` before `usage()` runs
  — empirically verified), and parse errors are wrapped into the standard
  JSON error object (usage errors exit 2; `ExitOnError`'s built-in path
  would print plain text and exit 2 outside the JSON contract); (c)
  `-h`/`--help` (`flag.ErrHelp`) is NOT a JSON error: the dispatcher detects
  `errors.Is(err, flag.ErrHelp)` and prints the verb's usage line to stdout
  itself, exit 0 — the suppressed `Usage` hook cannot be reused for help
  (the same no-op that silences errors silences the help path — empirically
  verified). One more shape rule closes the exact-arity hole: a token
  starting with `-` is never accepted AS a positional (no positional domain
  in the catalog — ids, slugs, S-ids, statuses, refs, roots — admits a
  leading `-`), so `verb --flag value` with the flags mistakenly first
  refuses as a usage error instead of popping flags as positionals. **No config library** either — configuration is `meta`
  rows (§6) plus flags.
- **DB layer: hand-written repositories over `database/sql`.** sqlc evaluated
  and rejected on fit, not maturity: the hard SQL of this project (serializer,
  whitelist load parser, PRAGMA choreography, exit-code error mapper) sits
  outside what a query generator covers, and the remaining queries are
  trivial.
- Lint: golangci-lint v2, `default: all` posture with per-linter reasoned
  disables. Security/supply chain: govulncheck via the go.mod `tool`
  directive; `-trimpath` reproducible builds.
- **Makefile (GNU make)** is the canonical command catalog — build, test,
  lint, fix-check, fuzz — so agents run `make <target>` instead of
  hallucinating toolchain invocations.
- Driver pair bumped only together: `modernc.org/sqlite` +
  `modernc.org/libc` at exactly the versions the driver's go.mod names
  (§3.1) — the pin is load-bearing, paired reviewed updates only.

---

## 4. Identifiers and reference grammar

| Form | Refers to | Example |
|---|---|---|
| `#NN` (CLI also accepts bare `NN`) | task | `#14` / `14` |
| `epic:SLUG` | epic | `epic:token-rotation` |
| `epic:SLUG/SID` | story | `epic:token-rotation/S2` |
| `CLASS:RELPATH` | artifact, default scope | `research:2026-03-05-options.md` |
| `CLASS@SCOPE:RELPATH` | artifact, named scope | `research@backend:2026-03-01-cache.md` |

JSON output always renders tasks as `"#14"`. The CLI accepts the bare integer
because unquoted `#` starts a shell comment; examples in this spec use the
bare form in commands and `#NN` in prose.

Task ids: `AUTOINCREMENT` (without it SQLite reuses a deleted max id —
research doc; never-reuse is a correctness requirement). Epic slugs:
kebab-case, permanent. Story ids `S<number>` per epic; `V-<number>` reserved
for post-close validation worklog rows. Scopes exist for multi-root
repositories (e.g. a monorepo whose subprojects each keep a `research/`);
single-root classes use the default scope `''`.

---

## 5. Data model

Schema v1. All tables `STRICT`; text columns `NOT NULL DEFAULT ''` unless
stated. No column ever **stores** NULL except `tasks.dup_of`, `tasks.epic`
and `worklog.corrects`, where absence is the meaning: no duplicate, no epic,
no row being corrected. (Columns declared `INTEGER PRIMARY KEY` accept NULL
on the way in — SQLite fills the rowid — but no stored row carries one, so a
check written against stored values needs no exception list.) Dates
are ISO-8601 UTC generated by Go only — **no verb accepts a caller-supplied
event date** (the one backfill path is `import --legacy`, whose backfilled
timestamps — git-derived or explicit-field, §6.2 — are events-marked).
Agent-supplied dates are a documented
parent-project drift class: a session that crosses midnight propagates its
own wrong narrative date into every record it touches, while the primary
source (the clock) is one call away.

```sql
-- 5.1 meta: instance facts + tool configuration. User-editable keys go ONLY
-- via `config`; system keys (schema_version, events_archived_through,
-- active_verb) are written solely by their owning verbs/machinery.
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL) STRICT, WITHOUT ROWID;
-- Configuration keys (production_globs 'internal/** cmd/**', idle_days '14',
-- prime_cap '20') are edited only via `config`. 'active_verb' is
-- transaction-internal: written
-- and deleted by `epic close` INSIDE its own transaction (crash ⇒ rollback
-- removes it), and the serializer additionally excludes the key defensively,
-- so it can never reach a dump even if a bug leaks it.

-- 5.2 path_dictionary: (class, scope) → repo-relative root
-- [RESOLVED-BY-EVIDENCE: docs/research/2026-07-18-design-forks-evidence.md D1].
CREATE TABLE path_dictionary (
  class TEXT NOT NULL, scope TEXT NOT NULL DEFAULT '',
  root  TEXT NOT NULL,
  ephemeral INTEGER NOT NULL DEFAULT 0 CHECK (ephemeral IN (0,1)),
  note  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (class, scope)
) STRICT, WITHOUT ROWID;
-- init seed (default scope; init CREATES every root so a fresh repo verifies
-- green): research→docs/research · adr→docs/decisions · workdir→work (ephemeral)
-- · run→work/runs (ephemeral) · report→work/reports. Additional classes/scopes
-- (e.g. src→ a code root, enabling `stale` over source files) are added by the
-- adopter via `paths set` — documented in PROMPT.md. Scope roots should be
-- disjoint per class: `paths set` warns when two scopes of the SAME class
-- overlap (ownership ambiguity). Different classes nesting (runs under a
-- workdir root) is normal and silent — the seed itself does it.

-- 5.3 epics
CREATE TABLE epics (
  slug TEXT PRIMARY KEY,
  goal TEXT NOT NULL CHECK (goal <> ''),
  status TEXT NOT NULL DEFAULT 'BACKLOG'
    CHECK (status IN ('BACKLOG','ACTIVE','PAUSED','CLOSED','DISSOLVED')),
  status_note TEXT NOT NULL DEFAULT '',
  close_sweep TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  CHECK ((close_sweep <> '') = (status = 'CLOSED')),
  CHECK (status NOT IN ('PAUSED','DISSOLVED') OR status_note <> '')
) STRICT, WITHOUT ROWID;
-- The pause/dissolve why lives in a CHECK (binds every process), not a trigger.

-- 5.4 epic_criteria: executable acceptance. criterion starting with '$ ' is
-- RUNNABLE (a shell command; `criteria check` executes it and records the
-- result); otherwise it is owner-attested (met flipped by `criteria met` with
-- evidence). The runnable/attested split is explicit — recorded prose is never
-- presented as a machine-verified pass.
CREATE TABLE epic_criteria (
  epic TEXT NOT NULL REFERENCES epics(slug),
  seq INTEGER NOT NULL,
  criterion TEXT NOT NULL CHECK (criterion <> ''),
  met INTEGER NOT NULL DEFAULT 0 CHECK (met IN (0,1)),
  evidence TEXT NOT NULL DEFAULT '',
  CHECK (met = 0 OR evidence <> ''),
  PRIMARY KEY (epic, seq)
) STRICT, WITHOUT ROWID;

-- 5.5 tasks: the backlog.
CREATE TABLE tasks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL CHECK (title <> ''),
  status TEXT NOT NULL DEFAULT 'NEEDS-TRIAGE'
    CHECK (status IN ('OPEN','IN-REVIEW','NEEDS-TRIAGE','DONE','WONT-DO','DUPLICATE','LABEL')),
  status_note TEXT NOT NULL DEFAULT '',
  parked TEXT NOT NULL DEFAULT '',    -- non-empty = deferred with this reason;
                                      -- excluded from ready; set via park/unpark
  dup_of INTEGER REFERENCES tasks(id),
  epic TEXT REFERENCES epics(slug),
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
  CHECK ((status = 'DUPLICATE') = (dup_of IS NOT NULL)),
  CHECK (status NOT IN ('DONE','WONT-DO') OR status_note <> ''),
  CHECK (dup_of IS NULL OR dup_of <> id),
  CHECK (parked = '' OR status IN ('OPEN','NEEDS-TRIAGE'))
) STRICT;
-- dup_of must point at a task that is NOT itself DUPLICATE (no chains/cycles):
-- refused by set-status and re-checked by R7. Any status transition clears
-- `parked` automatically (events-logged) — a parked task never trips the
-- parked-CHECK on close-out, and parking never outlives triage/completion.
-- OPEN (workable; parked≠'' = deferred-but-open) · IN-REVIEW (awaiting the PO,
-- incl. tasks that ARE questions) · NEEDS-TRIAGE (unknown is data; prime
-- surfaces a triage queue) · DONE (note = what closed it) · WONT-DO (note =
-- reopen trigger) · DUPLICATE (dup_of = canonical) · LABEL (reserved marker id;
-- no lifecycle; hidden from list/ready). No 'question' kind
-- [RESOLVED-BY-EVIDENCE: forks doc D8].

-- 5.6 task_links (created by `rel`; the 'duplicates' type is written ONLY by
-- set-status --dup-of and removed ONLY by reopen — rel does not offer it).
CREATE TABLE task_links (
  from_task INTEGER NOT NULL REFERENCES tasks(id),
  to_task   INTEGER NOT NULL REFERENCES tasks(id),
  type TEXT NOT NULL CHECK (type IN ('depends','relates','supersedes','duplicates')),
  note TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (from_task, to_task, type),
  CHECK (from_task <> to_task)
) STRICT, WITHOUT ROWID;
-- 'relates' is symmetric: stored once, queried in both directions by every verb.
-- `rel add` refuses: cycles (depends/supersedes) and depends/supersedes targets
-- in status LABEL or DUPLICATE (a LABEL can never become terminal; a DUPLICATE
-- masks its canonical).

-- 5.7 stories & worklog
CREATE TABLE stories (
  epic TEXT NOT NULL REFERENCES epics(slug),
  id TEXT NOT NULL CHECK (id GLOB 'S[0-9]*'),
  title TEXT NOT NULL CHECK (title <> ''),
  status TEXT NOT NULL DEFAULT 'PLANNED'
    CHECK (status IN ('PLANNED','READY','BLOCKED','IN-PROGRESS','DONE','DISSOLVED')),
  dod TEXT NOT NULL DEFAULT '', consumes TEXT NOT NULL DEFAULT '',
  produces TEXT NOT NULL DEFAULT '',
  blocked TEXT NOT NULL DEFAULT '',
  CHECK (NOT (status = 'IN-PROGRESS' AND blocked <> '')),
  PRIMARY KEY (epic, id)
) STRICT, WITHOUT ROWID;
CREATE UNIQUE INDEX wip_one_per_epic ON stories(epic) WHERE status = 'IN-PROGRESS';

CREATE TABLE worklog (
  epic TEXT NOT NULL REFERENCES epics(slug),
  seq INTEGER NOT NULL,
  story TEXT NOT NULL,      -- story id, or 'V-<n>' (post-close rows; verb refuses
                            -- V-rows unless the epic is CLOSED). FK-free by
                            -- design (composite target + V-rows); guarded by R4.
  date TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('IN-PROGRESS','DONE','BLOCKED-ON-OWNER','DISSOLVED')),
  commits TEXT NOT NULL DEFAULT '',  -- '<sha>..<sha>' | '<sha>' | 'legacy: <why>'
  gate TEXT NOT NULL DEFAULT '', review TEXT NOT NULL DEFAULT '',
  corrects INTEGER,                  -- NULL normally; a correction row names the seq it corrects
  note TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (epic, seq)
) STRICT, WITHOUT ROWID;
-- Episode history, not a status home. Episode rows are written BY THE STORY
-- VERBS: `story start` appends an IN-PROGRESS row, `story block` a
-- BLOCKED-ON-OWNER row (reason in note), `story dissolve` a DISSOLVED row,
-- `story done` the DONE row with commits — the ledger seeds itself and a
-- story's last row is terminal exactly when the story is (close rule 2 is
-- satisfiable by construction). Manual `worklog add` is for two cases only:
-- V-rows (CLOSED epics) and correction rows (`--corrects N`) — a correction
-- may target any story incl. terminal ones, must name the corrected seq —
-- always the seq of an ORIGINAL (non-correction) row: correcting a
-- correction is a second correction of the same original, so chains cannot
-- form and "what a correction means" never recurses. Correction volume is
-- unbounded by design; the dump diff is where a correction flood is visible
-- (no rule treats N corrections of one row as anomalous). A correction row
-- is excluded from state-consistency accounting BY NAME: from R6, from R10's
-- idle-clock (an unrelated historical correction must not reset an ACTIVE
-- epic's idle timer), and from §6.4 close rule (2) (a correction mirroring a
-- non-terminal row appended after a story's DONE row does not reopen the
-- story — without this exclusion an append-only ledger could never close the
-- epic again). Rework of a DONE story = a NEW story; the old story's history
-- stays true.

-- 5.8 artifacts & links
CREATE TABLE artifacts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  class TEXT NOT NULL, scope TEXT NOT NULL DEFAULT '',
  relpath TEXT NOT NULL,
  archived INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0,1)),
  note TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (class, scope) REFERENCES path_dictionary(class, scope),
  UNIQUE (class, scope, relpath)
) STRICT;

CREATE TABLE task_artifacts (
  task INTEGER NOT NULL REFERENCES tasks(id),
  artifact INTEGER NOT NULL REFERENCES artifacts(id),
  role TEXT NOT NULL CHECK (role IN
    ('home','origin-research','evidence','grounding','decision-package','adr',
     'workdir','output','report','run')),   -- closed vocabulary [DECIDED —
                                            -- pilot-validated in the parent]
  note TEXT NOT NULL DEFAULT '',            -- free prose describing the link
  PRIMARY KEY (task, artifact)
) STRICT, WITHOUT ROWID;
CREATE TABLE epic_artifacts (
  epic TEXT NOT NULL REFERENCES epics(slug),
  artifact INTEGER NOT NULL REFERENCES artifacts(id),
  role TEXT NOT NULL CHECK (role IN
    ('home','origin-research','evidence','grounding','decision-package','adr',
     'workdir','output','report','run')),
  note TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (epic, artifact)
) STRICT, WITHOUT ROWID;
-- role='home': a task/epic's narrative home(s); several legal; a dangling home
-- is unrecordable (the artifact must exist to be linked). R13 warns about OPEN
-- tasks with no home. Story-level attribution rides in worklog/note text for
-- v0 (story_artifacts is additive later, §17). Pointers to non-file targets
-- (a worklog row, a prose section) are notes, not links — documented limitation.

-- 5.9 events: append-only audit; entity uses §4 grammar (verb-validated;
-- R8) — except the instance-scoped `paths` and `config` events, whose
-- entity carries the affected token verbatim (a dictionary row or a
-- configuration key has no §4 form); R8 skips those two by event type.
CREATE TABLE events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  at TEXT NOT NULL, entity TEXT NOT NULL,
  event TEXT NOT NULL CHECK (event IN
    ('create','edit','set-status','reopen','park','unpark','rel','link','unlink',
     'archive','unarchive','story','worklog','criteria','epic','paths','config',
     'import','gate-skip')),
  detail TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX events_entity ON events(entity);
-- R8/R12 and per-entity `log` filter by entity; without this index R12 goes
-- quadratic with history (measured: seconds at year-5 event volume vs
-- milliseconds indexed — long-horizon research doc). Part of the canonical
-- DDL, so it rides in every dump.
-- `detail` contract: the verb's human-readable payload, VERBATIM — for
-- status transitions the `--note` text, for `story` events the block reason
-- / unblock resolution. For `edit`: the changed field names with old→new
-- values, EACH bounded to a short stated prefix ('…' marked) — edit is
-- ungated and its fields are unbounded prose, so verbatim copies would
-- pour essay-sized text into events forever; the full NEW value lives in
-- the entity row itself (and every committed dump), the full OLD value in
-- the prior committed dump — events carry pointers, not archives. Stated
-- limitation: a value overwritten twice within one session never reaches
-- a commit and survives only as its bounded prefix (state trails git by
-- one commit — §8.3). Because events are
-- append-only while `tasks.status_note` is a single overwritten column,
-- the events trail is where superseded verdicts survive (a task cycling
-- IN-REVIEW twice keeps verdict A here when the column moves on to B) —
-- this is what makes the trail, not the column, the ratification record.
```

### Schema gates (triggers)

```sql
-- Append-only + no-delete. worklog & events: no UPDATE, no DELETE. Entity
-- tables (tasks, epics, stories, epic_criteria, artifacts,
-- path_dictionary): no DELETE ("history is moved, never deleted" is
-- schema-enforced; import only INSERTs, so no conflict). The LINK tables
-- (task_links, task_artifacts, epic_artifacts) carry no delete trigger:
-- link rows are current relations whose audit history is the events trail
-- (rel/link/unlink events), and their sanctioned deleters — `rel rm`,
-- `unlink`, `reopen` for the duplicates link — could not exist otherwise
-- (nor could R7's one-to-one rule survive a reopen).
CREATE TRIGGER worklog_no_update BEFORE UPDATE ON worklog
  BEGIN SELECT RAISE(ABORT,'worklog is append-only'); END;
CREATE TRIGGER worklog_no_delete BEFORE DELETE ON worklog
  BEGIN SELECT RAISE(ABORT,'worklog is append-only'); END;
-- (events: same pair; every other table: a BEFORE DELETE refusal trigger)

CREATE TRIGGER worklog_seq_contiguous BEFORE INSERT ON worklog
  WHEN NEW.seq <> (SELECT COALESCE(MAX(seq),0)+1 FROM worklog WHERE epic = NEW.epic)
  BEGIN SELECT RAISE(ABORT,'worklog seq must be contiguous per epic'); END;

-- Transition matrices (UPDATE paths). INSERT paths are deliberately open for
-- import (§1.1); R12 is the compensating detection control.
CREATE TRIGGER tasks_transition BEFORE UPDATE OF status ON tasks
  WHEN NOT (OLD.status = NEW.status
    OR (OLD.status='OPEN'         AND NEW.status IN ('IN-REVIEW','NEEDS-TRIAGE','DONE','WONT-DO','DUPLICATE'))
    OR (OLD.status='IN-REVIEW'    AND NEW.status IN ('OPEN','NEEDS-TRIAGE','DONE','WONT-DO','DUPLICATE'))
    OR (OLD.status='NEEDS-TRIAGE' AND NEW.status IN ('OPEN','IN-REVIEW','DONE','WONT-DO','DUPLICATE'))
    OR (OLD.status IN ('DONE','WONT-DO','DUPLICATE') AND NEW.status='OPEN'))
  BEGIN SELECT RAISE(ABORT,'illegal status transition '||OLD.status||' -> '||NEW.status); END;

-- Leaving IN-REVIEW consumes an owner verdict; the ratification note must be
-- present (§6.2 set-status). Schema tooth for non-emptiness only — verdict
-- freshness/truthfulness stays with the verb and the events trail. The
-- legal-target clause keeps this trigger silent on matrix-ILLEGAL
-- transitions so tasks_transition reports those regardless of firing order
-- (SQLite trigger order is not a documented contract; empirically the
-- last-created same-event trigger fires first, which would otherwise turn
-- "illegal transition" errors into misleading "supply --note" ones).
CREATE TRIGGER tasks_inreview_exit_note BEFORE UPDATE OF status ON tasks
  WHEN OLD.status='IN-REVIEW'
   AND NEW.status IN ('OPEN','NEEDS-TRIAGE','DONE','WONT-DO','DUPLICATE')
   AND NEW.status_note = ''
  BEGIN SELECT RAISE(ABORT,'leaving IN-REVIEW requires the owner verdict in --note'); END;

CREATE TRIGGER stories_transition BEFORE UPDATE OF status ON stories
  WHEN NOT (OLD.status = NEW.status
    OR (OLD.status='PLANNED' AND NEW.status IN ('READY','BLOCKED','DISSOLVED'))
    OR (OLD.status='READY'   AND NEW.status IN ('IN-PROGRESS','BLOCKED','DISSOLVED'))
    OR (OLD.status='BLOCKED' AND NEW.status IN ('READY','DISSOLVED'))
    OR (OLD.status='IN-PROGRESS' AND NEW.status IN ('DONE','BLOCKED','DISSOLVED')))
  BEGIN SELECT RAISE(ABORT,'illegal story transition '||OLD.status||' -> '||NEW.status); END;
-- DONE and DISSOLVED are terminal: no out-edges — story reopening is
-- schema-illegal on the UPDATE path, matching §5.7's rework-is-a-new-story rule.

CREATE TRIGGER epics_transition BEFORE UPDATE OF status ON epics
  WHEN NOT (OLD.status = NEW.status
    OR (OLD.status='BACKLOG' AND NEW.status IN ('ACTIVE','DISSOLVED'))
    OR (OLD.status='ACTIVE'  AND NEW.status IN ('PAUSED','CLOSED','DISSOLVED'))
    OR (OLD.status='PAUSED'  AND NEW.status IN ('ACTIVE','CLOSED','DISSOLVED')))
  BEGIN SELECT RAISE(ABORT,'illegal epic transition '||OLD.status||' -> '||NEW.status); END;
-- CLOSED and DISSOLVED are terminal; there is deliberately no epic reopen —
-- post-close work = V-rows; a revived goal = a new epic (§17).

-- Close stamping is verb-gated (accident guard — see §1.1 for its honest
-- strength: it stops mistakes, not a deliberate raw-SQL writer, whom R12 and
-- the dump review surface detect instead):
CREATE TRIGGER epics_close_gate BEFORE UPDATE OF status, close_sweep ON epics
  WHEN (NEW.status='CLOSED' OR NEW.close_sweep <> OLD.close_sweep)
   AND COALESCE((SELECT value FROM meta WHERE key='active_verb'),'') <> 'epic-close'
  BEGIN SELECT RAISE(ABORT,'CLOSED/close_sweep are written only by epic close'); END;
```

### Views

```sql
CREATE VIEW v_ready AS
  SELECT t.* FROM tasks t
  WHERE t.status = 'OPEN' AND t.parked = ''
    AND NOT EXISTS (SELECT 1 FROM task_links l JOIN tasks d ON d.id = l.to_task
                    WHERE l.from_task = t.id AND l.type = 'depends'
                      AND d.status NOT IN ('DONE','WONT-DO','DUPLICATE'))
  ORDER BY t.id;
CREATE VIEW v_backlog AS
  SELECT id, '#'||id AS ref, title, status, status_note, parked, epic
  FROM tasks WHERE status <> 'LABEL' ORDER BY id;
```

### Enforcement map (who is bound)

| Invariant | Mechanism | Binds |
|---|---|---|
| Closed vocabularies; DUPLICATE⇔dup_of; DONE/WONT-DO⇒note; PAUSED/DISSOLVED⇒note; CLOSED⇔stamp; met⇒evidence; non-empty titles/goals; parked-only-on-open | CHECK | any process |
| IN-REVIEW exit ⇒ non-empty note | trigger (`tasks_inreview_exit_note`) | driver path + accidental raw UPDATEs. Two honest gaps: weaker than its DONE/WONT-DO CHECK sibling, and non-emptiness only — a raw UPDATE that leaves the stale prior verdict in the column passes (the verb path always rewrites it) |
| `story unblock` resolution text | verb-only (schema cannot see flag text); the events row is the durable record | the verb path |
| Referential integrity (FKs incl. composite) | FK + DSN pragma | driver connections; any process with FKs on |
| WIP=1 per epic | partial UNIQUE index | any process |
| Types; id never reused | STRICT; AUTOINCREMENT | any process |
| Append-only, no-delete, transition matrices, close stamping | triggers | driver path + accidental raw writes (UPDATE/DELETE); NOT a deliberate INSERT-path writer — see R12 |
| Single writer | EXCLUSIVE locking (first write → connection close) | any process |
| Everything else (R-rules, DoR, cycles, V-row placement, entity grammar) | verbs + `verify` | the verb path; verify detects the rest |

---

## 6. CLI

### 6.1 Conventions

- Binary `selftracked`; alias `strk` **[DECIDED]** (D3: `st`/`stk` were
  disqualified by collision evidence — forks research doc; owner picked
  `strk` over `sdt`/none).
- `--json` on every verb; errors as `{"error":{"code","message"}}`.
- Exit codes: `0` success · `1` refusal (understood and correctly denied —
  includes all SQLITE_CONSTRAINT-family failures AND integrity refusals such
  as linking a nonexistent file) · `2` environment/infrastructure error
  (busy, corrupt, usage). The constraint-vs-infrastructure split relies on
  the driver exposing distinguishable result codes — a §16 verification item;
  fallback is matching on the error type/text, encapsulated in one mapper.
- Write verbs: version gate (§8.6 — every verb begins with it) → check
  dump-divergence sidecar (§8.4) → EXCLUSIVE connection →
  transaction (mutate + events) → commit → regenerate `dump.sql` + `STATE.md`
  → update `.selftracked/dump.hash` → `PRAGMA optimize`.
- Read verbs: the same §8.6 version gate runs first and may escalate into a
  migration (which rewrites DB and dump — §8.6); the verb's own body then
  runs `query_only`, no side effects on tracker state.

### 6.2 Verb catalog

Signature conventions: two-level verbs dispatch on a subverb token popped
before positionals, and every (verb, subverb) pair has a **statically known
positional count** (the §3.2 dispatcher checks it in both directions).
Sibling subverbs that elide positionals in the table take the same leading
positionals as their first-listed sibling (every `epic` subverb takes SLUG
except `list`, which takes none — it enumerates; every `story` subverb takes
SLUG SID except `add`, which takes SLUG; `criteria`/`paths`/`config`/`rel`
as printed; `link`'s sub-forms each print their full signature — base
`link`/`unlink` two positionals, `archive`/`unarchive` one). `worklog add`
deliberately breaks the story-verb pattern (SLUG positional + `--story`
flag): its story slot admits `V-N` forms and correction context, and the
flag marks that non-uniform domain. `story` verbs take the SPLIT
positional pair `SLUG SID` (§6.3) — the §4 combined form `epic:SLUG/SID` is
reference grammar for `show`/`log`/`link`, not the story verbs' argument
shape. `link archive|unarchive` keyword dispatch is unambiguous by
construction: the §4 ref grammar cannot produce the bare tokens
`archive`/`unarchive` (refs are `#`-prefixed, numeric, or class/`epic:`
prefixed) — stated as an invariant the grammar must preserve.

| Verb | Signature | Notes |
|---|---|---|
| `init` | `init [--force]` | §9 |
| `create` | `create --title T [--status OPEN\|IN-REVIEW\|NEEDS-TRIAGE] [--note N] [--epic SLUG] [--label]` | Prints `#NN` (RETURNING). Terminal statuses at creation exist only via `import`. `--label` creates the LABEL marker row. |
| `show` | `show <ref>` | Any §4 ref; artifacts list their linked tasks/epics (reverse lookup); stories show episodes. |
| `list` | `list [--status S] [--epic SLUG] [--parked] [--labels]` | |
| `ready` | `ready [--epic SLUG]` | `v_ready` (OPEN, unparked, deps terminal, id order). |
| `set-status` | `set-status <id> <STATUS> [--note] [--dup-of <id>]` | Matrix-checked. Refuses terminal→OPEN (that is `reopen`'s job). `DUPLICATE --dup-of` also writes the duplicates link. Any transition **out of IN-REVIEW** requires `--note` carrying the owner's verdict — the ratification record for the question the task existed to ask; non-emptiness is trigger-backed (`tasks_inreview_exit_note`), and when the exit goes straight to DONE/WONT-DO the one note serves both duties (verdict + closure reason) — write it as both. An IN-REVIEW flagged in error exits with a note SAYING that («flagged in error — no question was pending»): the note explains the exit, it never fabricates a verdict. The column holds the latest note; superseded verdicts survive verbatim in the events trail (§5.9). Story-side counterpart: `story unblock --resolution`. |
| `reopen` | `reopen <id> --why TEXT` | The sanctioned terminal→OPEN path; clears dup_of + link; logs `reopen`. |
| `park` / `unpark` | `park <id> --why TEXT` · `unpark <id>` | Deferral with reason; parked tasks leave `ready` AND the `prime` triage queue but keep their status (OPEN or NEEDS-TRIAGE). Any status transition auto-unparks (logged). |
| `edit` | `edit <ref> [--title] [--goal] [--note] [--dod] [--consumes] [--produces] [--epic SLUG\|--detach]` | Corrections/re-homing; old→new logged. Never statuses. |
| `rel` | `rel add <id> <depends\|relates\|supersedes> <id> [--note]` · `rel rm <id> <type> <id>` · `rel tree <id>` · `rel cycles` | No `duplicates` here (single writer of that fact is `set-status`). Refuses cycles and LABEL/DUPLICATE targets. `relates` queried undirected. |
| `link` | `link <id\|epic:SLUG> <class[@scope]:relpath> --role R` · `unlink …` · `link archive <artifact-ref> [--force]` · `link unarchive <artifact-ref>` | Role from the closed vocabulary. File must exist (exit 1 refusal otherwise) unless class is ephemeral — ephemeral links are existence-exempt by design and `stale` ignores them (stated limitation). `archive` warns and requires `--force` when the artifact is a live `home`. `link` refuses a relpath that does not resolve inside its class(+scope) registered root (no `..` escapes) — root registration is what retention (`gc`, §17) and R3 reason about, so an escaping path would dodge both. |
| `epic` | `epic create SLUG --goal G` · `activate` · `pause --why` · `dissolve --why` · `show` · `list [--active]` · `close` | Lifecycle per the §5.3 matrix; `close` works from ACTIVE or PAUSED (BACKLOG never ran — dissolve it instead). `dissolve` has close-grade preconditions: refuses while a story is IN-PROGRESS or a task in OPEN/IN-REVIEW/NEEDS-TRIAGE is homed to the epic; PLANNED/READY/BLOCKED stories are auto-DISSOLVED (each getting its DISSOLVED worklog row) in the same transaction. Blocker messages for parked tasks suggest unpark / `edit --detach`. |
| `criteria` | `criteria add SLUG --text T` · `criteria met SLUG SEQ --evidence E` · `criteria check SLUG` | `check` EXECUTES every `$ `-prefixed criterion (cwd = repo root, inherited env, per-command timeout, stop at first failure), records pass/fail + timestamp as evidence, **flips `met` 1→0 on a failing re-run** (regressions cannot stay green), and exits 1 on any failure. `met` is for owner-attested (non-runnable) criteria only. Threat model: runnable criteria are shell commands from repo state — executing them is the same trust decision as running the repo's build/tests or its tracked hooks; a hostile branch already owns those surfaces, so criteria add no new one (§14). |
| `story` | `add SLUG --title T · ready SLUG SID · start · block --reason · unblock --resolution TEXT · done --commits RANGE --gate G [--review R] · dissolve --why` | The state-writing lifecycle verbs — `start`, `block`, `done`, `dissolve` — each append their worklog episode row (§5.7); `add`, `ready`, and `unblock` do not (no worklog state exists for those transitions) — `unblock`'s durable record is its `story` events row. `start`: READY + DoR + WIP. `done`: `--commits` and `--gate` are required (`--review` optional) — status flip + DONE row are one transaction (no non-atomic path exists). `unblock` requires `--resolution` — **what cleared the block**, recorded verbatim in the same transaction's events row (`event='story'`, detail = the resolution text): when the block was an owner question (the `[BLOCKED: PO decision]` convention), the resolution starts with the literal greppable prefix `PO:` and quotes the verdict — a provisional instruction («try X, revisit if wrong») IS a verdict and is quoted like one; a decision living only in chat does not exist. When the block was external (a dependency, an outage — `block --reason` is not owner-only, §11.3 uses it for any blocker), the resolution states the observed fact that cleared it, no prefix. The `PO:` prefix is an authoring convention (prompt-enforced) and applies wherever an owner verdict is recorded — resolutions, IN-REVIEW exit notes, question-task titles (§12 uses all three): the schema cannot distinguish verdicts from other text, so the prefix is what makes owner ratifications machine-findable in the trail afterward. The literal token is deliberately locale-fixed for greppability; a non-English crew adapts the literal in its prompt config, not per-use. Presence-of-text is verb-enforced only (enforcement map). |
| `worklog` | `worklog add SLUG --story SID\|V-N --state ST [--corrects N] [--commits] [--gate] [--review] [--note]` | Manual appends are only: `V-N` rows (epic must be CLOSED) and `--corrects N` correction rows (any story; N names an original non-correction row — §5.7; state must match the corrected row; excluded from R6, R10-idle and close-rule-2 accounting — §5.7). Everything else is written by story verbs. No `--date`: rows are dated at append time by the binary — a correction happens when it happens, and a backdating flag would reopen the narrative-date drift class (§5) through a verb. The true historical date of a corrected FACT belongs in the row's `note` as content, sourced per the PROMPT.md provenance rules (git/mtime, never session narrative) — the row's own date stays the append date. |
| `paths` | `paths ls` · `paths set CLASS[@SCOPE] ROOT [--ephemeral] [--note]` · `paths move CLASS[@SCOPE] NEWROOT [--with-files]` | `--with-files` performs the move via `git mv` when in a git repo (renames AND stages), else plain rename — no red window either way. `set` warns on overlapping roots of the same class only. |
| `config` | `config ls` · `config set <production_globs\|idle_days\|prime_cap> VALUE` | The sanctioned editor for configuration keys (closed list in schema v1 — new keys arrive only with schema versions; values validated: `idle_days`/`prime_cap` positive integers, `production_globs` parseable globs; events-logged). System meta keys (§5.1) are not settable here. |
| `stale` | `stale [--since REF] [--quiet]` | git-changed files ∩ resolved non-ephemeral artifact links of non-terminal work; output ordered path ASC (deterministic). |
| `import` | `import --file F [--format md-table\|json] [--legacy]` | Batch creation (tasks, epics, stories, worklog). `--legacy`: synthesized timestamps (events-marked), `commits='legacy: …'` accepted, terminal states insertable. Timestamps are taken **per row from the best primary source, git first**: (1) the newest resolvable cited commit's git **author** date (author over committer — a rebase must not rewrite worklog chronology; "newest" because a multi-commit or `a..b` citation dates the increment by its finish); (2) else the row's explicit date field; (3) else the import time. Git outranks explicit dates deliberately — narrative-contaminated date fields are the §5 drift class, and imported narratives carry it too; when sources (1) and (2) both exist and disagree **on the calendar day (UTC)**, the importer warns and git wins, both values recorded (the modal contamination is exactly an adjacent-day midnight drift, which a "more than N days" threshold would sleep through; the recorded disagreement is the audit trail, not a correction). Best-effort honesty: plain rebases preserve author dates, squash-merges rewrite them — recovered dates are primary-source approximations. Non-sha placeholder tokens in a commits cell (prose like "see commit", template leftovers) resolve nothing and fall through, becoming `commits='legacy: …'`; an unresolvable sha-SHAPED token (a typo) is left verbatim instead — R5 flags it in full `verify`, and rewriting it to `legacy:` would mask the typo. The **per-epic** import events row (§10) carries a compact per-worklog-row source map (one short `seq:source` code per row, worklog seq order — deterministic, so two imports of the same file byte-agree), making date provenance auditable after the fact; a flat batch-time stamp is forbidden (real chronology recoverable from git must not be flattened into one synthetic epoch). Two stated scopes: import rows' `date` means "when the increment finished (best primary approximation)" while live rows mean "when recorded" — one column, two regimes, read uniformly downstream; and task rows have no commits cell at all, so task dates come from explicit fields or import time — task-level narrative contamination is out of git-first's reach in v0 — deriving those dates from the source file's git history was evaluated and rejected: a maintained markdown backlog gets renamed and reformatted, and each sweep resets the apparent age of the lines it touches. What the importer does instead is bound what it cannot derive: **a date later than the import moment is refused** (a future date is provably not an event date, and a session crossing midnight writing tomorrow's date all day is the documented failure this catches), and **a date earlier than the earliest commit touching the source file is reported, not refused** — an older record can legitimately be transcribed into a new file, but the bound it broke is stated rather than absorbed. Neither bound computes a date; they only reject impossible ones. The importer must append worklog rows in ascending per-epic order (the contiguity trigger fires on INSERT) and materialize stories for every referenced id. |
| `dump` / `load` | `dump [--stdout]` · `load [--force]` | §8. |
| `verify` | `verify [--quiet] [--fast]` | §7. |
| `prime` | `prime` | §11.1 contract. |
| `state` | `state` | Regenerates `STATE.md` (deterministic rendering: fixed sections, fixed ordering, last 10 events; R14 checks it matches the DB). |
| `log` | `log <ref> [--limit N]` | Events for one entity. |
| `gate` | `gate skip-mark` | Writes the gitignored `.selftracked/skip-pending` marker (called by the pre-commit skip path; no DB write mid-commit). The next write verb — or `load`, which is what runs after a divergence — converts the marker into a `gate-skip` events row; `verify` (incl. `--fast`, R15) reports a pending marker meanwhile. Honest limit: the marker is per-machine; a skip followed by *nothing* is visible only on the machine that skipped. |

Deferred verbs (§17): `search` (FTS5), `stats`, `events archive`, `redact`,
`story move/split`, priority/rank ordering, MCP server.

### 6.3 Examples

```console
$ selftracked create --title "Extract dump writer into internal/dump" --epic v0-core
#14
$ selftracked story start v0-core S3
error: WIP limit: story S2 is IN-PROGRESS in epic 'v0-core' …        [exit 1 — unique index]
$ selftracked set-status 14 OPEN
#14 NEEDS-TRIAGE -> OPEN
$ selftracked set-status 14 DONE
error: DONE requires --note (what closed it)                          [exit 1 — CHECK]
$ selftracked set-status 14 DONE --note "3f1c40e..8a2b7c1; gate: go test ./internal/dump green"
#14 OPEN -> DONE
$ selftracked link 14 research:2026-03-05-dump-format.md --role origin-research
error: file not found: docs/research/2026-03-05-dump-format.md        [exit 1 — refusal]
$ selftracked park 9 --why "waiting on PO decision: auth scope"
#9 parked (leaves the ready frontier, stays OPEN)
$ selftracked set-status 14 OPEN
error: DONE -> OPEN goes through `reopen --why` (reopens are always explained)  [exit 1]
```

### 6.4 `epic close` — the atomic retro

Refuses with the complete blocker list unless: (1) every story terminal;
(2) the **last non-correction** worklog row of every story terminal
(correction rows mirror the state of the row they correct and are excluded —
§5.7 — otherwise a legitimate correction of a non-terminal row, appended
after a story's DONE row in this append-only ledger, would make the epic
permanently uncloseable); (3) `criteria check`
passes for every runnable criterion and every non-runnable criterion is
`met=1`; (4) no task in OPEN / IN-REVIEW / NEEDS-TRIAGE is homed to the epic;
(5) every DONE story has a DONE worklog row with non-empty commits
(`legacy:` passes, visibly); (6) the epic has at least two stories, any
status — §2's "decomposing into ≥2 stories" enforced at the boundary where
the epic's shape is adjudicated (all statuses count: the definition asks
that the goal decomposed, not that every path succeeded, and a DISSOLVED
second story must not hold the close hostage). On success, one
transaction: status→CLOSED, close_sweep→today, events row. Post-close
validation = `V-n` rows.

---

## 7. Integrity: `verify`

Stage 0 — container: `PRAGMA integrity_check` **plus** `PRAGMA
foreign_key_check` (the former does not cover FKs — research doc); `--fast`
(the pre-commit path) = `quick_check` + `foreign_key_check` + every pure-SQL
Stage-1 rule (R4, R6–R9, R12 — cheap queries needing no serialization and no
filesystem/git access; R4's correction-row structure check among them, so a
malformed correction is caught at the commit boundary) + R15. It skips only
the serialization-bound rules (R1, and R14 folded into it) and the
filesystem/git-bound ones (R2, R3, R5, R10, R11, R13). R1 is skipped deliberately: the hook regenerates dump + STATE.md
immediately after (§9), so the pair is fresh by construction and the whole
hook still costs **one** serialization pass (at multi-year event volume that
pass is the hook's dominant cost — long-horizon research doc) — while the
detection rules, R12's forgery signature among them, DO run at the commit
boundary. Full Stage 1 runs in `verify` proper and CI.

Stage 1:

| # | Rule |
|---|---|
| R1 | Dump regenerated from DB byte-equals tracked `dump.sql`; tracked dump loaded (in-memory, via driver Serialize/Deserialize) re-dumps byte-equally; STATE.md byte-equals its render (R14 folded here) |
| R2 | Every path root exists |
| R3 | Every non-archived artifact of a non-ephemeral class resolves |
| R4 | `worklog.story` ∈ stories(epic) or `V-[0-9]+` (V only on CLOSED epics); `corrects` targets an existing smaller seq of the same story, the target is itself a **non-correction** row (the no-chains backstop — same shape as R7's dup-chain rule), AND the correction's state equals the corrected row's state (the verb-level rules, §5.7/§6.2, re-checked here) |
| R5 | Non-`legacy:` commits resolve via `git cat-file` |
| R6 | DONE story ⇔ DONE worklog row with commits, both directions (correction rows excluded from the count) |
| R7 | duplicates links ⇔ dup_of, one-to-one; no dup_of target is itself DUPLICATE (no chains/cycles) |
| R8 | Every `events.entity` resolves (grammar + existence) — except `paths`/`config` events, instance-scoped by design: their entity is the affected token verbatim, skipped by event type |
| R9 | `sqlite_sequence` ≥ MAX(id) per AUTOINCREMENT table (an absent `sqlite_sequence` row counts as 0; the clause applies only when the table has rows); `meta.events_archived_through` present, a non-negative integer, and **= 0 in schema v1** — no verb can write it, so any other value is the raw-SQL tamper signature (a forged boundary would silently truncate the dump's audit trail with R1 green on both sides) — red. Reserved D7-era clause, inactive while the boundary is 0: `sqlite_sequence(events)` ≥ the boundary |
| R10 | Advisory: ACTIVE epics with no READY/IN-PROGRESS story and no **non-correction** worklog append in `idle_days` → idle report (PAUSED/BACKLOG are silent — intentional states; correction rows are excluded so an unrelated historical correction cannot reset the idle clock of a genuinely neglected epic — §5.7) |
| R11 | Advisory: per-machine gate inactive warning. For each of pre-commit and post-commit, the **effective hook location** (`core.hooksPath` when set, else `.git/hooks`) must either be `.selftracked/hooks` or contain a hook file referencing its `.selftracked/hooks/` counterpart (best-effort textual detection, §9; a reference counts as chained only when it appears OUTSIDE a comment — neither a line starting with `#` nor a reference sitting after an inline `#` on a live line qualifies, since a commented mention otherwise reads as a live chain, and false silence is this rule's fail-open direction); the warning names each unchained hook — covering unset, set-but-unchained, and pre-commit-chained-but-post-commit-not states |
| R12 | Terminal-state ⇔ events-trail cross-check: every CLOSED/DISSOLVED epic and DONE/WONT-DO/DUPLICATE task must have a matching events row (or an `import` events row). A terminal state with no trail = the raw-SQL forgery signature (§1.1) — red |
| R13 | Advisory: OPEN tasks with no `home` link |
| R15 | Advisory: pending `.selftracked/skip-pending` marker (unconverted gate skip) |

Every rule ships with a red fixture; a gate that cannot fail is decoration.
The one textual matcher among the rules (R11) additionally ships a **variant
table**: the canonical chaining line, common hand-edited spellings (quoted
path, `sh`-prefixed, variable-prefixed), a whole-line `#`-commented reference
(must NOT match), a live line whose only reference sits in a trailing
`# comment` (must NOT match — the case a naive line-starts-with-`#` filter
plus substring search gets wrong), and an unrelated hook (must warn). A
matcher blind to one accepted
spelling fails open silently in exactly the state it exists to warn about,
and an over-eager matcher false-positives on mere mentions — both failure
directions are documented parent-project incident classes, so both get
fixtures. Accepted best-effort residual, stated rather than fixtured: a
quoted `#` inside a live string BEFORE the real reference (e.g.
`echo "#x"; .selftracked/hooks/pre-commit`) can defeat naive after-`#`
stripping — R11 is advisory and "best-effort textual detection" is its
honest ceiling.

---

## 8. The deterministic dump

### 8.1 Serializer contract

Own serializer; `sqlite3 .dump` is disqualified (no column lists, shadow-table
blobs, format changed between adjacent releases — research doc). UTF-8, LF,
trailing newline; no banners beyond the **single header comment line**, which
carries `schema_version` plus the tasks/artifacts id high-water for human
readers (never events/worklog seq — those change on every verb and would put
the header into every diff). The header's `schema_version` is normative for
the loader: it selects
DDL(k)/whitelist(k) *before* parsing (the `meta` row sits after the full DDL
block and cannot steer the parser), and a mismatch between header and
`meta.schema_version` row is a hard refusal. `sqlite_sequence` is not serialized at all:
loading rows with explicit ids sets the AUTOINCREMENT high-water marks
automatically (empirically verified), so the dump needs no sequence data —
R9 sanity-checks the live DB instead. Full DDL verbatim from the single
compiled-in source; fixed table order with `events` last; rows ORDER BY full
PK (`events` bounded below by the archive boundary — §8.2); explicit column
lists; single-quote doubling as the only escape (control
characters refused at write time); NULL emitted only for `tasks.dup_of`,
`tasks.epic` and `worklog.corrects` — the three columns §5 permits to store
it; no PRAGMAs ever.

### 8.2 Review surface — honest claims

A status flip diffs as two lines (state row + events row in the tail).
Verbs that create entities also bump the seq header comment (one more line);
`story done` is three lines (story, worklog, events); `story unblock` is the
two-line shape with its resolution prose riding in the events line — same
size blessing as worklog notes below. Worklog `note` prose is
as large as its author writes it — teams that write essay-sized episode notes
get essay-sized diffs; that is the worklog carrying real content, not noise.
Events stay in-dump [RESOLVED-BY-EVIDENCE: forks doc D7 — a separate append
file does not in fact merge cleanly and union-merge corrupts seq]; growth is
bounded later by `events archive` (§17), whose **dump-format boundary is
reserved now** (D15): `meta.events_archived_through`, seeded by `init` as
`0`, treated as `0` when the row is absent (a missing row must never drop
events). The serializer emits only events with `seq >` the boundary, so
activating the verb later changes no dump grammar. Honest scope of the
reservation — what is *not* reserved and lands **with** D7, not before:
boundary-aware verify semantics (R12's terminal-trail check reads history
that archiving removes from the live dump), the archive segments' own
versioned grammar, reader, and naming (they carry no DDL block, so §8.5's
loader cannot read them), and live-DB reclamation (the no-delete trigger
stands; reclamation will ride the §8.6 rebuild path — a fresh DB built
without the archived rows). **In v0 the boundary must equal 0, enforced by
R9**: no verb can write the key, so any other value is the raw-SQL tamper
signature — without that rule a forged boundary would silently truncate the
dump's audit trail with R1 green (the serializer honors the forged value on
both sides of the comparison). Reserved D7-era load guard (inactive while
the boundary is 0, like R9's boundary clause): `load` seeds the events
AUTOINCREMENT high-water mark to `max(boundary, MAX(live seq))` — covering
both a dump with a gap above the boundary and one whose events were ALL
archived, where zero live rows give the explicit-id mechanism nothing to
work with and a fresh clone would otherwise reuse archived seq numbers
(empirically verified). Events are ~72% of dump bytes at multi-year scale (long-horizon
research doc); documented activation triggers: events > 50k, or dump > 10 MB, or a write
verb > 200 ms (a trigger starts D7 work; the research doc shows worklog
archiving may need to follow before a large dump drops back under 10 MB).

### 8.3 Crash-safety and the pair

DB commit, then dump+STATE render to temp + atomic rename, then sidecar hash
update. The DB is the local truth; a crash between steps leaves derived files
stale, and the next write (or pre-commit hook) regenerates them. `load
--force` is the only operation that discards local DB state; it prints the
divergence summary first. `load` builds the new DB in a temp file and
atomically renames (interrupted builds never land).

**State trails git by at most one commit**: the dump refreshed by the last
write of a session rides in the *next* commit; the skill therefore ends every
session with a bookkeeping commit (§11.3), and `verify` reports a dirty dump.

### 8.4 Sync, divergence, and conflict semantics

Git is the only sync channel, and the divergence hazard is handled
mechanically, not by hope:

- `.selftracked/dump.hash` (gitignored sidecar) stores the hash of the last
  dump this DB produced or loaded. **On mismatch, a write verb first
  regenerates the dump from the DB in memory**: if the regenerated bytes equal
  the tracked file, the mismatch is residue of a crash between dump write and
  sidecar update — the verb heals the sidecar and proceeds. If they differ,
  the tracked dump changed under us (a `git pull`, a checkout) and writing
  would clobber it with stale-DB-derived content — the verb **refuses**,
  naming the fix: `selftracked load` (fast-forward the DB to the pulled dump),
  or reconcile local unsynced writes by re-applying them through verbs after
  `load`. A missing sidecar is treated as divergent (safe default; fresh
  clones run `load` once). The sidecar (SHA-256) is written by `init`, by
  every write verb's dump-regeneration tail, and by `dump` and `load`. The
  §8.6 version gate runs **before** this comparison, so the
  regenerate-and-compare always runs the current serializer against a
  current-schema DB. The decision procedure is sidecar-anchored and total:
  tracked dump **matches** the sidecar → no external change — and if the
  DB's `user_version` is ahead of the tracked dump's header version, that is
  migration-crash residue (crash between the DB swap and its re-dump),
  healed by re-running the re-dump. Tracked dump **does not match** the
  sidecar → regenerate-and-compare as above: equal → crash residue, heal the
  sidecar; different → external change (covering crash-*plus*-pull too) →
  refuse. A migration triggered in the externally-changed state completes
  DB-side only and does **not** write the dump (§8.6) — nothing may
  overwrite a pulled dump before the divergence is reconciled.
- `prime` performs the same comparison read-only and reports
  `"dump_divergence": true` so a session starts knowing (§11.1).
- True two-writer accidents (the axiom violated) surface as textual merge
  conflicts in `dump.sql` or duplicate-PK `load` failures — loud by
  construction; no auto-merge driver is shipped, deliberately.
- Repos synced by file-sync services: the generated docs instruct that git
  is the only sync medium and that `.selftracked/db.sqlite*` and the sidecar
  must be **excluded from the file-sync tool** — they are per-machine.
  §8.6's automatic session-start migration makes a file-synced DB actively
  dangerous: two devices can race the EXCLUSIVE rebuild and the sync service
  manufactures conflict copies.

### 8.5 Loading untrusted dumps

`load` never pipes dump text into a general SQL interpreter:

1. The DDL block must **byte-equal the compiled-in canonical DDL** for the
   dump's schema version — not "look valid", byte-equal. A tampered trigger
   or CHECK body is thereby unloadable.
2. Data statements must match the serializer's exact grammar:
   `INSERT INTO <known-table> (<exact column list>) VALUES (...)` with
   literal tokens only (the serializer's integer/quoted-string/NULL forms —
   token-shape whitelist, no expression grammar at all). Anything else —
   PRAGMA, ATTACH, expressions, unknown tables — is a hard refusal (exit 2).
3. Loading runs with `trusted_schema=OFF`, `cell_size_check=ON`,
   `mmap_size=0`, capped `sqlite3_limit` values. `load` refuses a dump whose
   `meta` lacks the `schema_version` or `events_archived_through` rows (a
   *missing* row passes the grammar whitelist, so this is an explicit
   required-rows check) or whose `schema_version` row disagrees with the
   header (§8.1). The build then stamps `PRAGMA
   user_version`/`application_id` from the `meta.schema_version` row (the
   dump carries no PRAGMAs, and a freshly built DB defaults to 0 — without
   this stamp the next verb would spuriously re-migrate a current DB) and
   applies the §8.2 load guard; then Stage-0 verify **plus the DB-only rule
   set of §7 `--fast`** (R6–R9, R12 — a boundary forgery or trail-less
   terminal state never survives to the rename), then atomic rename. (`SQLITE_DBCONFIG_DEFENSIVE` is unreachable through the
   pure-Go driver — documented; the whitelist parser is the primary defense.)
4. §16 mandates fuzzing the parser — it is the security boundary.

### 8.6 Schema evolution — versioned rebuild **[DECIDED]** (D12)

Evidence: `docs/research/2026-07-18-db-migrations.md`. The fossil model
(auxiliary state recomputed from the durable record), not a migration tool:
goose et al. were evaluated and rejected — their version table enters
`sqlite_schema` and breaks §8.5's byte-equal DDL gate, they migrate only the
DB side (the dump side would still be hand-built), and their transaction
wrapping conflicts with `foreign_keys=OFF` exactly where rebuilds need it.

- The binary compiles in, per historical schema version k: canonical DDL(k),
  loader whitelist(k), and a pure-Go row transform `T_k: rows(k) → rows(k+1)`.
- **Every verb — read verbs included — begins with the version gate**, and it
  runs *before* the §8.4 divergence check. The gate is two ordered
  comparisons: (i) the tracked dump's header `schema_version` vs the
  binary's N — newer triggers the forward-only refusal below; then (ii) the
  DB's `user_version` vs N — older triggers migration (`load` on a fresh
  clone has no DB to gate: (i), then rebuild). A read verb finding
  `user_version` behind escalates for the migration only: it drops
  `query_only`, opens the write connection, takes the EXCLUSIVE lock,
  migrates, then reopens read-only — so the first post-upgrade `prime`
  (SessionStart) migrates, not just the first write. Migration: hydrate rows
  (from the live DB, or from an old dump via the versioned whitelist — same
  engine, two sources) → run the transform chain → build a fresh DB from
  DDL(N) in a temp file (the `load` path: INSERT is open by §1.1, gates
  arrive whole with the DDL — **and the INSERT-firing gates bind hydration
  too**: transforms must emit rows in serializer PK order, and any transform
  that renumbers or splits worklog rows must re-emit each epic's seq
  contiguously, or the rebuild aborts on `worklog_seq_contiguous` — today
  `worklog` carries the schema's only INSERT-firing gate; the obligation is
  stated table-generally so future INSERT gates inherit it) →
  Stage-0 + full verify → atomic rename → the verb reopens the renamed file
  on a fresh connection appropriate to itself (write verbs: EXCLUSIVE;
  escalated read verbs: back to `query_only`) — the swap gap is benign under
  the single-writer axiom, an intruding writer surfaces as §8.4 divergence →
  then, **only if the §8.4 check finds no external change**, re-serialize
  with the current serializer → sidecar update (in the externally-changed
  state the migration stays DB-side only — §8.4). Automatic, no user-facing
  `migrate` command; the notice line always goes to stderr (no
  output-limiting flag suppresses it), and `prime` additionally reports
  `"migrated": "vK→vN"` in its JSON. Two same-machine read verbs may race
  the escalation: the loser blocks on the lock, re-checks `user_version`
  after the wait (the winner migrated) and proceeds read-only; a BUSY that
  survives `busy_timeout` exits 2 with a hint that another process may be
  migrating — retry. A welcome simplification: fresh-rebuild sidesteps
  SQLite's 12-step ALTER recipe entirely.
- **Forward-only, permanently** (the pg_dump contract): a dump whose
  `schema_version` is newer than the binary is a hard exit-2 refusal naming
  the required upgrade (git's repository-format rule: an unknown version
  means the tool "MUST NOT operate on that repository"). `prime` surfaces
  the same condition non-fatally at session start: its divergence check also
  reads the tracked dump's header `schema_version`, and a newer-than-binary
  value is reported as `"dump_requires_newer_binary": true` in healthy JSON
  (§11.1) — without this the SessionStart fallback chain would reduce the
  refusal to an uninformative "run load" loop.
- CI: a golden corpus with one dump per historical schema version — load →
  migrate → full verify → byte-determinism; a commutativity check (migrating
  via the live DB ≡ via the dump; asserted at `events_archived_through = 0`,
  the only v0 state — D7 revisits); a refusal fixture for version N+1.
- v0 ships schema version 1; the machinery above is the *policy* — the first
  transform is written when version 2 exists.

---

## 9. Repository layout, init, hooks

```
<repo>/
├── .selftracked/
│   ├── dump.sql        # tracked — state, review surface, sync channel
│   ├── db.sqlite       # gitignored
│   ├── dump.hash       # gitignored — divergence sidecar (§8.4)
│   ├── skip-pending    # gitignored — gate-skip marker (transient)
│   └── hooks/          # tracked — generated git hooks
├── docs/decisions/  docs/research/  work/  work/runs/  work/reports/
├── STATE.md            # GENERATED (deterministic; R14)
└── PROMPT.md           # agent instruction generated by init
```

No config file: configuration is `meta` rows edited via `config`. `init`
creates every seeded root (fresh `init` ⇒ `verify` green), class-contract
READMEs, ADR `_template.md`, `PROMPT.md`, `STATE.md`, `AGENTS.md` (a short
harness-agnostic pointer: the verbs, the rule, `prime`; content specified in
the implementation plan), `.claude/` files (§11), hooks, and `.gitignore`
entries, and seeds the `meta` system rows (`schema_version`,
`events_archived_through` = `0` — §8.2). The class READMEs also name the
recommended **opt-in** classes with
conventional roots — `runbook`, `guide`, `rfc`, `src`, `external` —
registered via `paths set` when a project needs them (D14: seed kept minimal,
the rest documented, per the scaffold research doc), plus the
Keep-a-Changelog `CHANGELOG.md` file convention; the `work/` README states
the cleanup expectation for ephemeral classes (manual until `gc` ships —
§17), and the generated
docs recommend periodic `git repack` — a rewritten multi-MB dump leaves one
full blob per commit between repacks (long-horizon research doc).

Beyond the verb catalog and the §11.2 rule, PROMPT.md carries three
**durable-doc authoring rules**, each distilled from a documented
parent-project drift incident. Honest status: these are authoring guidance
in a generated instruction file, not gated conventions — prose files are a
stated v0 non-goal for machine checking (§1.1), rule 2's validator is
explicitly deferred (§17), and §1's executable-gates principle governs DB
state (its stated scope), not doc authoring. Guidance is what the parent
project had too when it drifted; the difference here: rule 1 removes the
duplication surface outright (the enumerable state lives behind verbs and a
generated STATE.md, so pointing beats copying), while rules 2–3 govern the
prose that legitimately remains — anchored numbers and dated filenames —
so following them costs less than violating them:

1. **Prose never duplicates DB-enumerable state** (counts, id/status lists)
   — cite the verb (`list`, `show`, `log`) or STATE.md instead. Every prose
   copy of an enumerable list drifts silently the day the enumeration
   changes.
2. **A DB-derived number that must live in a durable doc is anchored**
   `as of dump <sha12>` — the first 12 hex of the SHA-256 of the last
   **committed** dump: `git show HEAD:.selftracked/dump.sql | shasum -a 256`,
   then truncate to 12 hex. Anchor the committed blob, not the working-tree
   file: mid-session the tree holds a dump state that may never reach a
   commit, and an anchor to it is unverifiable forever (§17's validator
   checks against committed history). Freshness corollary: a number derived
   from THIS session's writes has no committed epoch yet (state trails git
   by one commit — §8.3) — commit first, anchor after; anchoring it to
   HEAD's dump cites an epoch that cannot contain it. The gitignored
   divergence sidecar is NOT a shortcut for this digest: it tracks the
   working-tree dump and during any session with pending writes is
   routinely one commit ahead of HEAD (and fresh clones have none; §8.3
   crash windows can leave a stale one). A bare date is too coarse an anchor
   the day the DB changes twice; honesty note: the anchor pins the *epoch*,
   not the arithmetic — a wrong number beside a fresh hash still needs
   review to catch.
3. **Event dates and date-bearing filenames come from the system clock**
   (`date`), never from the session narrative — the verbs already enforce
   this for DB rows (§5), but agents also *name* dated files (research docs,
   reports), and a wrong date baked into a filename is permanent: filenames
   are identifiers, and renaming breaks every recorded reference.

It prints the per-machine activation
(`git config core.hooksPath .selftracked/hooks`) with the trust note:
repo-tracked hooks execute on your machine — enable only on repos you trust.
**If `core.hooksPath` is already set, or the incumbent hook location is
non-empty, `init` does not print the takeover command** — it prints a
chaining recipe instead, covering **both hooks**: one line in the incumbent
pre-commit invoking `.selftracked/hooks/pre-commit` **with the exit status
propagated** (the printed line is `.selftracked/hooks/pre-commit || exit $?`
— without propagation a RED verify degrades to advisory), and one line **at
the top of** the incumbent post-commit invoking
`.selftracked/hooks/post-commit` (warn-only, safe to run first — appended at
the bottom it can be skipped by the incumbent's own early `exit` paths). The
recipe mandates
**subprocess execution, never `source`** — the generated scripts contain
internal `exit` statements that would otherwise terminate the incumbent hook
and skip its remaining gates (the exact hazard chaining exists to prevent).
`SELFTRACKED_SKIP=1` bypasses only selftracked's own gate (and still writes
the R15 marker); the incumbent's own skip conventions are untouched. Blindly
repointing hooksPath would silently disable the host project's existing
gates — a real hazard, observed in a pilot rehearsal against a host repo
whose incumbent pre-commit was load-bearing.

Generated `pre-commit`:

```sh
#!/bin/sh
command -v selftracked >/dev/null 2>&1 || {
  echo "selftracked: binary not installed — gate skipped (install to enable)" >&2; exit 0; }
if [ "$SELFTRACKED_SKIP" = "1" ]; then
  selftracked gate skip-mark >/dev/null 2>&1 \
    || echo "selftracked: WARNING could not record skip marker" >&2
  exit 0
fi
selftracked verify --fast --quiet; rc=$?
if [ "$rc" = 2 ]; then
  echo "selftracked: verify could not run (busy/corrupt/env) — fix the environment; not bypassable as RED" >&2; exit 1
elif [ "$rc" != 0 ]; then
  echo "selftracked: verify RED — 'selftracked verify' for details; SELFTRACKED_SKIP=1 bypasses ONCE (recorded)" >&2; exit 1
fi
selftracked dump  || { echo "selftracked: dump refresh FAILED — aborting commit" >&2; exit 1; }
selftracked state || { echo "selftracked: STATE.md refresh FAILED — aborting commit" >&2; exit 1; }
git add .selftracked/dump.sql STATE.md \
  || { echo "selftracked: staging dump/STATE.md FAILED — aborting commit" >&2; exit 1; }
echo "selftracked: staged refreshed dump.sql + STATE.md" >&2
selftracked stale --since HEAD || true
```

`post-commit` (warn-only): commit touches `meta.production_globs` paths with
neither `#NN` nor an epic slug in the message → yellow untraced-commit
warning. It also compares the just-committed `dump.sql` blob against the
sidecar hash: a mismatch right after a commit means the commit carried a
stale dump — the signature of a bypassed pre-commit (`git commit -n` skips
the whole hook chain and writes **no** R15 skip marker; this warn-only
backstop is the only in-repo trace such a bypass leaves). Hooks are POSIX sh; v0 targets POSIX environments (on Windows they
run under Git-for-Windows' sh; the *dump determinism* CI matrix includes
Windows, the hook scripts are exercised on POSIX runners — stated limitation).

---

## 10. Adopting on an existing project (migration posture)

Generic by design — the concrete first-client walkthrough lives in a separate
migration guide (a v0 deliverable, written generically per the charter):

- **Scopes** exist because real monorepos keep one artifact class in several
  roots; the importer registers one dictionary row per (class, scope).
- **`import --legacy`** exists because historical records cannot satisfy
  what the schema demands of new ones: timestamps are synthesized (marked in
  events), done-work without recoverable commit ranges is recorded as
  `commits='legacy: …'` — visible and accepted by R6/close rather than
  blocking closure forever. Import may insert terminal states directly
  (matrices gate the UPDATE path only); every import writes events rows, so
  R12 stays green.
- **Importer obligations** (enforced by the schema, so stated plainly):
  worklog rows append in ascending per-epic order; ledger rows that bundle
  several increments are split; a story row is materialized for every S-id
  the worklog references (`V-` rows are exempt by definition); **an `import`
  events row is written per terminal entity** (each DONE/WONT-DO/DUPLICATE
  task, each CLOSED/DISSOLVED epic) — R12 matches per entity, so a single
  batch summary row would leave every imported terminal state red; the
  per-epic import events row's detail carries the compact per-worklog-row
  date-source map (§6.2 — which of git/explicit/import-time dated each row),
  giving worklog rows date provenance without per-row events rows; rows
  "slated as a future increment" map to epic homing, not to `park`; source
  inconsistencies (e.g. a closed epic whose cards were never flipped) are
  resolved *before* import — the close gates here are stricter than hand-kept
  files, and that strictness is the point.
- **What does not migrate into the tracker**: prose registries, narrative
  planning documents, per-file status headers and their index-sync gates —
  the prose layer and its future lint core (§17). Pointers to non-file
  targets degrade to notes. So do owner steers recorded only in
  campaign-level prose that unblocked work never modeled as a BLOCKED story:
  they import as task/epic notes, not as unblock events (there is no block
  row to resolve). The migration is partial *by design*; the guide
  says exactly where the boundary is so nothing is lost by surprise.

---

## 11. Claude Code integration layer (v0)

### 11.1 SessionStart hook + `prime` contract

```json
{"hooks": {"SessionStart": [{"hooks": [{"type": "command",
  "command": "sh -c 'selftracked prime --json 2>/dev/null || { selftracked load >/dev/null 2>&1 && selftracked prime --json; } || echo \"{\\\"error\\\":\\\"selftracked state unavailable\\\",\\\"hint\\\":\\\"run selftracked load (if load refuses a newer dump, upgrade selftracked); see .selftracked/dump.sql\\\"}\"'"}]}]}}
```

Three branches, all emitting exactly one JSON object: healthy `prime` —
which itself reports `"dump_divergence": true` when the tracked dump moved
under a readable DB (the common post-pull case: surfaced by the *flag*, not
the error branch); `load` then `prime` (fresh clone / missing DB); the
explicit error object (unreadable or truly irreconcilable state). `load` here
is the no-`--force` form: it fast-forwards a missing/behind DB and refuses a
divergent one, which lands in the error branch by design. Stated limitation:
on a fresh clone whose pulled dump needs a newer binary, `load` refuses
inside the fallback chain and the static error JSON (whose hint names the
upgrade path) is all the session sees — the typed
`dump_requires_newer_binary` field exists only when a readable DB lets
`prime` run.

`prime` JSON (stable contract): `epics_active[]` (slug, goal, stories
{done, in_progress, ready[], blocked[]}, criteria_unmet — a **count**, never
criterion text), `epics_paused[]` (slugs),
`epics_backlog[]` (slugs), `ready[]`, `triage[]` (NEEDS-TRIAGE queue),
`in_review[]`, `stale[]`, `totals{}`, `dump_divergence` (bool),
`dump_requires_newer_binary` (bool — §8.6 forward-only surfaced at session
start), `migrated` (present only when this invocation migrated, §8.6),
`sprint_goals[]` (every IN-PROGRESS story; multiple entries = "finish or
choose explicitly", never a silent pick). The backlog-type lists —
`ready[]`, `triage[]`, `in_review[]`, `stale[]`, `epics_paused[]`,
`epics_backlog[]` — are **capped** at `prime_cap` entries (a validated
`config` key, default 20) in stated deterministic order (id ASC; `stale[]`
path ASC; slug ASC for epic lists); `totals{}` is part of the stable contract and carries the full
count of every list plus `parked` (no separate scalar — one authoritative
representation). **`sprint_goals[]` and `epics_active[]` are never capped**:
they are bounded by WIP=1 per epic, and capping them would break the
"nothing hides" invariant (§2). List entries carry identifiers, statuses,
and counts, plus exactly two naming fields: the epic `goal` line in
`epics_active[]` and the story `title` in `sprint_goals[]` entries
(`(epic, story, title)` tuples) — one-line naming prose by authoring role,
the only prose-class payload in the contract. Note/verdict/reason/DoD text
never rides in `prime` (fetched per entity via `show`/`log`); that exclusion
is part of what keeps `prime` output O(active), never O(history), at any
repo age (D15).

### 11.2 Rule

State only through verbs; never `sqlite3` against the DB, never hand-edit
`dump.sql`. Stated honestly: write invariants are backed by schema/detection
(§1.1); raw *reads* are unpreventable — the rule plus a deny-list entry are
convention, and the real mitigation is that every read verb is cheaper than
the SQL it replaces. PO decisions go to IN-REVIEW / `story block`; agents
never answer them. One PO question therefore shows up in **three
status-bearing columns kept in step by the verbs** (`story block` writes the
story-side pair; the task moves by its own verb — no transaction spans
both): the question task at
`tasks.status='IN-REVIEW'` (the queue item awaiting the PO), the story at
`stories.status='BLOCKED'` (the WIP slot freed; reason text in
`stories.blocked`), and the story's `worklog.state='BLOCKED-ON-OWNER'`
episode row (the history). One waiting state, three vocabularies by design —
stated here so an audit does not read the spread as drift (a parent-project
auditor filed exactly such a deliberate multi-surface pairing as a status
mismatch because it was written down nowhere). Stated limitation: the
task↔story correlation is conventional (shared epic + matching prose in
`--reason`/title), not schematic — with several blocked stories and several
IN-REVIEW tasks in one epic, nothing in the data model says which task
answers which story; an epicless question task correlates by prose alone.
A schematic link is a named deferral (§17).

### 11.3 Skill

The working loop: `prime` → if `dump_divergence`: stop, reconcile first → 
**backlog refinement** when `totals.triage > 0` (triage → OPEN /
IN-REVIEW / park / WONT-DO; when the queue exceeds the capped `triage[]`
slice, re-`prime` between passes until `totals.triage` reaches 0) → pick
from `ready` honoring `sprint_goals` →
`story start` → work → commit with `#NN`/slug → `story done --commits …
--gate …` → at epic end `epic close` → **end every session with a
bookkeeping commit** (the dump refreshed by the session's last write must
reach git — §8.3). Drift rule: new idea = `create` + park, one command.
**PO-absent branch**: every remaining story blocked and `in_review`
non-empty → stop; ensure each open question is an IN-REVIEW task (they
surface in every future `prime`); if an in-progress story is what blocks,
`story block --reason` it (freeing the WIP slot); do not pivot to
out-of-scope work; never answer PO questions.

---

## 12. End-to-end trace: a working day

Baseline: a generic file-based self-tracking setup (markdown backlog + epic
files + lint scripts). Fictional host `acme-api`; dates illustrative.

**T0 — adoption.** `selftracked init` → seeded roots created, hooks offered,
`verify` green by construction. The doc ecosystem starts with enforcement
attached instead of gates retrofitted after the first drift.

**T1 — a thought parks without derailing.** `create --title "Rotate auth
tokens on password change"` → `#7` NEEDS-TRIAGE. One command; "unknown" is
data; next `prime` shows it in `triage`.

**T2 — research links with write-time integrity.** `set-status 7 OPEN`; then
`link 7 research:2026-03-05-token-rotation-options.md --role origin-research`
refuses (exit 1) until the file exists. A confabulated reference cannot be
recorded *through any verb* (and R3 detects one planted past the verbs) — in
the file baseline it survives until an audit.

**T3 — the work grows into an epic with executable acceptance.**
`epic create token-rotation --goal …`; `criteria add token-rotation --text
"$ go test ./internal/rotate/..."` (runnable — `criteria check` will
execute it); `story add` ×3; `story block token-rotation S3 --reason
"[BLOCKED: PO decision] alerting channel?"`; `create --title "PO: alerting
channel" --status IN-REVIEW --epic token-rotation` → `#8`;
`epic activate token-rotation`. The question is a DoR blocker *and* an
IN-REVIEW queue item; the agent can neither start S3 nor close the epic
around it.

**T4 — WIP enforced by the storage engine.** `story ready` + `story start S1`
→ sprint goal. `story start S2` → exit 1, unique-index refusal naming S1.
Even a raw UPDATE hits the same index.

**T5 — work, commit, prove: honestly two steps.** Commit with
`(token-rotation S1, #7)` → pre-commit: verify green, dump+STATE regenerated
and staged (echoed). Then `story done token-rotation S1 --commits 4f9c2d1
--gate "go test ./internal/authdb/... green"`. The hash-after-commit step
cannot disappear (no tool knows a hash before its commit), but it is one
command, a small diff, pair-gated — and the session-end bookkeeping commit
(§11.3) guarantees it reaches git.

**T6 — the feared refactor.** `paths move research docs/notes --with-files`
→ `git mv` + one dictionary row, atomically verified. Zero references change
because references were never paths.

**T7 — staleness against code reality.** The adopter registered a code class
at setup (`paths set src internal`), and S2's output was linked:
`link epic:token-rotation src:rotate/service.go --role output`. Weeks later
a commit touches that file → pre-commit advisory: "stale: linked to ACTIVE
epic token-rotation (role: output) — update the epic if behavior changed."
Every competitor's `stale` means "issue untouched lately"; this one means
"reality moved under documented work."

**T8 — compaction survival.** New session → `prime`: sprint goal, blocked
story, PO queue, `dump_divergence:false` — nothing from memory; worklog
anchors let the agent `git show 4f9c2d1` and *verify* what done meant.

**T9 — the PO answers; the epic closes atomically.** `epic close` refuses
listing every blocker (S2 in progress, S3 blocked, criteria unmet, #8
IN-REVIEW homed). The verdict lands twice, once per surface: `story unblock
token-rotation S3 --resolution "PO: 'use the webhook channel' (chat,
2026-03-12)"` and `set-status 8
DONE --note "PO: 'use the webhook channel'; S3 (9e1f0a2..b3c4d5e)"` (the IN-REVIEW exit
refuses without the note — trigger-backed). After S2/S3 complete with
commits and `criteria check` runs the `$`-criteria green, close succeeds in
one transaction. A decoy
stamp is excluded on every verb and UPDATE path (verb-gated trigger); an
INSERT-path forgery is the §1.1 adversary, caught by R12 and the dump diff.

**T10 — archaeology.** `log 8` → the full IN-REVIEW→DONE trail with the PO's
answer; append-only, entity-queryable, and one `git log -p
.selftracked/dump.sql` away on any clone.

| File-baseline friction | Mechanism here |
|---|---|
| Status change = multi-file edit policed by lint | One verb; indexes are views |
| Post-commit hash bookkeeping = multi-file sweep | One command, pair-gated (collapsed, not eliminated) |
| Hand-kept counters; prose rename tables | AUTOINCREMENT; DUPLICATE⇔dup_of⇔link |
| Confabulated references until audit | Refused at write time |
| WIP as prose, then lint | Unique index |
| Close-checklist decoy arms race | Verb-gated stamp + R12 detection |
| Directory move = dead-link hunt | `paths move --with-files` |
| Cold start = long reading route | `prime` |
| Rules read but violated | Violation is an error code |

---

## 13. Naming

§5 names are **[DECIDED]** (D6, owner sign-off). Machine token `WONT-DO` (no
apostrophe).

## 14. Privacy & publication

- `dump.sql`/`STATE.md` publish task titles, notes, PO decisions, worklog
  text — including `events.detail` payloads (verdicts, resolutions, `edit`
  old/new-value prefixes accumulating append-only across every historical
  edit, §5.9) — and user-authored structured fields (criteria
  commands, DoD,
  consumes/produces) that may contain local paths if written carelessly —
  when the repo is public; append-only + git history make it permanent.
  `init` and PROMPT.md carry this warning; `redact` tooling is deferred
  (§17). The pre-commit hook *visibly* stages the refreshed dump (echoed
  line) — review `git diff --staged` before pushing sensitive repos.
- Commit SHAs recorded in worklogs resolve, in any clone, to git objects
  carrying author name/email/timestamps — inherent to git provenance, stated
  so nobody assumes otherwise. selftracked's own verbs never write hostnames,
  usernames, or absolute paths.
- Repo-tracked hooks execute on the committer's machine once activated —
  informed per-machine step (§9).
- `load` treats all dump content as untrusted (§8.5).

## 15. Decision log

Resolved (owner can veto; evidence in `docs/research/`):

| # | Decision | Evidence doc |
|---|---|---|
| D1 | Repo-relative roots; `external` indirection later | forks-evidence §D1 |
| D2 | Canonical SQL dump, own serializer | forks-evidence §D2 (merge experiments; format-neutral merges; SQL wins on DDL/load/tooling) |
| D4 | DB gitignored, dump-only tracked | forks-evidence §D4 (byte-nondeterminism vs pair gate) |
| D5 | MCP at v0.1+ as a thin wrapper | forks-evidence §D5 |
| D7 | Events in-dump; `events archive` later | forks-evidence §D7 |
| D8 | PO questions = IN-REVIEW convention | forks-evidence §D8 |
| D9 | Scopes solve multi-root classes | first-client fit analyses (private archive; the multi-root requirement and importer obligations they established are summarized in §10) |
| D10 | Ratchet register deferred; named in §17 | forks-evidence §D10 |
| D11 | Go stack: stdlib CLI + hand-written DB layer; no framework / sqlc / config lib; pinned toolchain (§3.2) | go-stack |
| D12 | Schema evolution = versioned rebuild, forward-only dumps; no migration tool (§8.6) | db-migrations |
| D13 | Pilot ladder: testscript synthetics → self-host → gitignored-clone import rehearsal → colocated live install (§16) | pilot-and-testing-flow |
| D14 | Default scaffold kept; runbook/guide/rfc/src/external documented as opt-ins (§9) | repo-scaffold-taxonomy |
| D15 | Events-archive boundary reserved in v0 (§8.2); `events(entity)` index; `prime` caps; activation triggers documented | long-horizon-scale |

Decided by owner: **D3** alias = `strk` (`st`/`stk` disqualified by collision
evidence; `sdt`/none passed over) · **D6** §5 naming confirmed as specified ·
**D11–D15** reviewed and approved (no longer veto-pending).

Open: none.

## 16. Quality gates for selftracked itself

- CI: build/vet/test `CGO_ENABLED=0`; `init && verify` green in a temp repo;
  dump byte-determinism across OS runners (incl. Windows); red fixture per
  V-rule and per schema gate, plus the R11 variant table (§7 — including the
  must-not-match commented-out reference); serializer mutation tests;
  **whitelist-parser fuzzing** (it is the security boundary, §8.5); golangci-lint v2 +
  `go fix -diff` clean + govulncheck (§3.2); a **testscript e2e suite** —
  each scenario a txtar script running the real binary in a fresh temp dir,
  git fixtures generated in-script (real objects for hook/divergence/R5
  tests), golden `cmp` on dumps and JSON, exit codes asserted; from schema
  version 2 on, the golden old-dump migration corpus (§8.6).
- Implementation-phase re-verification of research-pass findings before
  reliance (each currently documented, with method, in
  `docs/research/2026-07-18-sqlite-advanced-features.md`): driver
  Serialize/Deserialize roundtrip; VACUUM-INTO/rename flow; extended vs
  primary result codes; recursive_triggers/REPLACE regression; RETURNING via
  Query; cross-OS serializer byte-equality; `go fix -diff` exit-code
  behavior on the pinned toolchain (empirically 1-on-diff, undocumented —
  §3.2).
- Licensing: modernc stack is BSD-3-licensed — compatibility with
  Apache-2.0 recorded in NOTICE at first dependency intake (deliverable).
- Deliverables alongside this spec: README with positioning + public
  references (the `docs/research/` documents), CONTRIBUTING (DCO +
  AI-contribution clause), the generic migration guide, CI workflow.
- Dogfooding & pilot ladder (D13): the moment `init` works, this repo's
  backlog moves into `.selftracked/` via `import` from the implementation
  plan derived from this spec (tracked as the first epic — the fossil
  precedent: the first project hosted is the tool itself). External adoption
  is staged: testscript synthetics → self-hosting → import rehearsals against
  a **disposable local clone of the client kept under a gitignored path**
  (refreshed by pull, imported from scratch each round; real git objects,
  zero risk to the source) → live install only after the importer round-trips
  the full corpus with `verify` green — and then in colocation posture: the
  host's existing gates stay authoritative, selftracked chains from them
  (§9), abandonment = delete a directory.

## 17. Explicit deferrals

MCP server (v0.1) · `search`/FTS5 (serializer is shadow-table-safe from day
one) · `stats` · `events archive` (its dump boundary is already reserved —
§8.2) · `gc` (removes ephemeral-class dirs — workdirs/runs — of terminal
entities older than a cutoff, **always excluding paths under a registered
non-ephemeral root nested inside an ephemeral one** — the seed nests
`work/reports` under `work/`, and a plain path sweep would delete report
artifacts into R3 red — **and excluding any artifact whose link role marks
it durable** (`report`/`output`/`evidence`/`decision-package`/`adr`),
whatever its class: role and class are independent axes, and class alone
must not decide deletion; uncleaned `work/` outgrows the DB by orders of
magnitude — long-horizon research doc) · release packaging (goreleaser) ·
doc-lint core for prose files (index-sync
and status-header gates for research/ADR files; registry-field validation;
content counts; homoglyph scanning; snapshot-anchor validation — checking a
doc's cited `as of dump <sha12>` (§9) equals the SHA-256, recomputed, of
some historical committed content of `dump.sql` (walk `git log` and hash
each revision's content — git's own blob object ids are SHA-1 over a
prefixed payload and never comparable), NOT equality with the live digest —
historical anchors legitimately differ from the live dump one commit after
they are written, that is what an epoch anchor is; the check is honest only
against a full clone (shallow or history-pruned clones weaken it to
"not found here"); until it ships the §9 anchor convention is unchecked,
and a host that needs the gate today wires its own doc-lint to the same
digest; dated-filename check — a date-prefixed filename vs its first-commit
date, §9 rule 3's mechanical counterpart) · tech-debt/ratchet register (D10) ·
structured multi-round review records · `redact` · `external` path class ·
preupdate-hook auto-events · story-level artifact links · a schematic
task↔story blocked-question link (§11.2's correlation stays conventional in
v0) · `story move`/
`story split` · priority/rank ordering of the ready frontier (v0 order is id
ASC) · epic reopen (never — post-close work is V-rows; a revived goal is a
new epic) · a curated human roadmap/changelog view beyond STATE.md ·
first-class references to external trackers or suffixed id schemes (they live
in notes) · multi-writer anything (never, by thesis).
