-- selftracked schema, version 1.
--
-- Verbatim from the specification's §5. This file is the single compiled-in
-- source: the serializer emits it into every dump, and the loader refuses a
-- dump whose DDL block does not match it byte for byte. Editing it is a
-- schema version change, never a patch.
--
-- Do not reformat. Byte equality is the contract.

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

-- 5.9 events: append-only audit; entity uses §4 grammar (verb-validated; R8).
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

-- Append-only + no-delete. worklog & events: no UPDATE, no DELETE. All other
-- entity tables (tasks, epics, stories, epic_criteria, artifacts, task_links,
-- task_artifacts, epic_artifacts, path_dictionary): no DELETE ("history is
-- moved, never deleted" is schema-enforced; import only INSERTs, so no conflict).
CREATE TRIGGER worklog_no_update BEFORE UPDATE ON worklog
  BEGIN SELECT RAISE(ABORT,'worklog is append-only'); END;
CREATE TRIGGER worklog_no_delete BEFORE DELETE ON worklog
  BEGIN SELECT RAISE(ABORT,'worklog is append-only'); END;
-- The specification's shorthand is written out here because this file is
-- the byte-compared artifact. Entity tables get a BEFORE DELETE refusal;
-- the LINK tables (task_links, task_artifacts, epic_artifacts) do not —
-- their rows are current relations, their audit is the events trail, and
-- their sanctioned deleters (rel rm, unlink, reopen) need the path open.
CREATE TRIGGER events_no_update BEFORE UPDATE ON events
  BEGIN SELECT RAISE(ABORT,'events is append-only'); END;
CREATE TRIGGER events_no_delete BEFORE DELETE ON events
  BEGIN SELECT RAISE(ABORT,'events is append-only'); END;
CREATE TRIGGER tasks_no_delete BEFORE DELETE ON tasks
  BEGIN SELECT RAISE(ABORT,'tasks: history is moved, never deleted'); END;
CREATE TRIGGER epics_no_delete BEFORE DELETE ON epics
  BEGIN SELECT RAISE(ABORT,'epics: history is moved, never deleted'); END;
CREATE TRIGGER stories_no_delete BEFORE DELETE ON stories
  BEGIN SELECT RAISE(ABORT,'stories: history is moved, never deleted'); END;
CREATE TRIGGER epic_criteria_no_delete BEFORE DELETE ON epic_criteria
  BEGIN SELECT RAISE(ABORT,'epic_criteria: history is moved, never deleted'); END;
CREATE TRIGGER artifacts_no_delete BEFORE DELETE ON artifacts
  BEGIN SELECT RAISE(ABORT,'artifacts: history is moved, never deleted'); END;
CREATE TRIGGER path_dictionary_no_delete BEFORE DELETE ON path_dictionary
  BEGIN SELECT RAISE(ABORT,'path_dictionary: history is moved, never deleted'); END;

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
