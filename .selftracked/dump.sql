-- selftracked dump schema_version=1 tasks=41 artifacts=1
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
INSERT INTO meta (key, value) VALUES ('events_archived_through', '0');
INSERT INTO meta (key, value) VALUES ('idle_days', '14');
INSERT INTO meta (key, value) VALUES ('prime_cap', '20');
INSERT INTO meta (key, value) VALUES ('production_globs', 'internal/** cmd/**');
INSERT INTO meta (key, value) VALUES ('schema_version', '1');
INSERT INTO path_dictionary (class, scope, root, ephemeral, note) VALUES ('adr', '', 'docs/decisions', 0, '');
INSERT INTO path_dictionary (class, scope, root, ephemeral, note) VALUES ('report', '', 'work/reports', 0, '');
INSERT INTO path_dictionary (class, scope, root, ephemeral, note) VALUES ('research', '', 'docs/research', 0, '');
INSERT INTO path_dictionary (class, scope, root, ephemeral, note) VALUES ('run', '', 'work/runs', 1, '');
INSERT INTO path_dictionary (class, scope, root, ephemeral, note) VALUES ('workdir', '', 'work', 1, '');
INSERT INTO epics (slug, goal, status, status_note, close_sweep, created_at) VALUES ('pilot-adaptation', 'Adapt selftracked to deployment on an external pilot repository: debug the onboarding recipe (init, hook colocation, legacy import) on local sandbox copies until it deploys cleanly and repeatably; file every generic defect or missing feature it surfaces as ordinary public tasks. Pilot specifics never enter tracked content - records here are worded purely in terms of what selftracked does.', 'ACTIVE', '', '', '2026-07-25T01:04:29Z');
INSERT INTO epics (slug, goal, status, status_note, close_sweep, created_at) VALUES ('v0-bootstrap', 'Build selftracked v0 per docs/v0-spec.md and switch this repository to self-hosting', 'CLOSED', '', '2026-07-24', '2026-07-18T23:39:52Z');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('pilot-adaptation', 1, 'The documented onboarding recipe deploys selftracked end-to-end (init, hooks, import) on two consecutive fresh sandbox copies with selftracked verify green and no manual forks', 1, 'runs two and three: fresh-context agents executing only the handoff reached verify green (0 violations) end-to-end with no manual forks; third run deviation-free');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('pilot-adaptation', 2, 'Import fidelity on the sandbox copy is verified by recorded checks: no source records lost or altered', 1, 'per-status task counts, story/worklog/criteria row counts equal across source prose (independent greps), derivation manifest and imported dump on every rehearsal run');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('pilot-adaptation', 3, 'Every generic defect surfaced by the experiments is filed as a public task; every fix applied carries tests and green gates', 1, 'one generic defect surfaced (#19, bookkeeping commit vs empty index) - fixed in the scaffold templates with content tests, go test ./... 12/12 ok (be1669e); no other generic defect found by the rehearsals');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 1, 'Verb catalog, integrity engine (R1-R15), init, hooks, reader half, and import built; make gates green locally through S9', 1, 'git history: docs/v0-progress.md ledger @ 489f4c5; local gates run @ acdd980 (interim, D-EP8)');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 2, '$ selftracked verify', 1, 'PASS selftracked verify @ 2026-07-24T23:27:19Z');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 3, 'Schema version gate and migration branches ship (plan S11)', 1, 'S11 landed at commits ab10c78..32dcb18: the 8.6 gate before every verb (dispatcher + pipeline re-check), the migration engine on the load path with two hydration sources, escalation with the sentinel protocol, 8.4 branches (5)-(6), prime migrated:vK->vN; make gates exit 0 on a cleared cache 2026-07-24; FULL three-critic close review; spec revs 3.22-3.23');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 4, 'Pilot ladder rungs 3-4: gitignored disposable-clone import rehearsal, then colocated live install (plan S12)', 1, 'Rung 3: repeated from-scratch imports of the pilot corpus, verify full 0 violations each archived round, determinism by timestamp-normalized byte-identity; rung 4: colocated live install, incumbent hooks chained per recipe, host gate authoritative, verify 0 violations; docs/research/2026-07-24-pilot-import-rehearsal.md');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 5, 'README, CONTRIBUTING (DCO + AI clause), generic migration guide, docs link-check (plan S12)', 1, 'README final, CONTRIBUTING (DCO + AI clause), docs/migration-guide.md (boundary, reconciliation, both rehearsal-found authoring constraints; example round-trips through import+verify), scripts/check-doc-links.sh exit 0 - commits ab10c78..92d5fe3, reviewed by the S12 fidelity critic');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 6, 'First push: gates re-run against the squashed tree, verified rows re-stamped, CI green (D-EP11)', 1, 'CI green on the published tree (origin/main 42a3e5b): GitHub run 30132992219 success on all jobs — policy gates, macOS full suite, Ubuntu full suite, Windows dump-determinism. Published WITHOUT squash (owner decision); the tree was history-rewritten pre-push only to re-stamp evidence (ef1d4f7) and scrub the pre-publication audit findings, not squashed. Verified-by-command worklog rows S1-S19 cite story shas <=19a608f, untouched by the scrub.');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 7, 'Spec governed proposal-first end to end: every deviation an archived amendment (26 amendments through spec rev 3.21 / plan rev 17)', 1, 'openspec/changes/ directory; amendments log in git history (docs/v0-progress.md @ d4cddcc)');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (1, 'Align dump.sql file mode with the other tracked files (0600 from CreateTemp+rename vs 0644)', 'OPEN', 'Parked at the S8a close (code critic): more restrictive, not a leak; git tracks content, not mode. internal/dump owns it if it ever matters.', '', NULL, NULL, '2026-07-23T22:09:23Z', '2026-07-24T23:27:19Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (2, 'Read-transaction for prime/state snapshot consistency, if the owner ever wants it', 'OPEN', 'Refuted as a defect at the S8c close under the single-writer axiom (spec section 1); a momentary off-by-a-few in advisory totals that self-heals on the next prime. Remedy on record: wrap prime/state reads in one read transaction.', '', NULL, NULL, '2026-07-23T22:09:23Z', '2026-07-24T23:27:19Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (3, 'dump_divergence is a boolean and cannot distinguish a behind DB from a truly divergent one', 'OPEN', 'Post-v0 note from the 2026-07-24 open-questions round (fork analysis F2, architecture critic): both cases route to load --force + re-apply; the flag carries no distinguishing detail.', '', NULL, NULL, '2026-07-23T22:09:23Z', '2026-07-24T23:27:19Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (4, 'Revisit the OpenSpec proposal+delta+tasks.md convention once the tool is installed', 'OPEN', 'Parked at the first-commit review: change directories hold proposal.md only; the tool is adopted but not installed, so the convention has nothing to run against yet.', '', NULL, NULL, '2026-07-23T22:09:23Z', '2026-07-24T23:27:19Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (5, 'First push: re-run gates on the squashed tree and re-stamp every verified row (D-EP11)', 'DONE', 'First push done 2026-07-24: origin/main at 42a3e5b, GitHub CI run 30132992219 green on all jobs. Evidence re-stamped (ef1d4f7) and criterion 6 met. Published without squash (owner decision) — verified worklog rows S1-S19 cite story shas <=19a608f, untouched by the pre-push leak scrub.', '', NULL, 'v0-bootstrap', '2026-07-23T22:09:23Z', '2026-07-24T23:26:23Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (6, 'epic show --json emits a JSON header line plus plain-text sections, not one JSON document', 'DONE', 'epic show --json emits one JSON document; fixture in epic.txtar; spec-conformant fix (6.1 --json contract)', '', NULL, 'v0-bootstrap', '2026-07-23T22:28:38Z', '2026-07-24T01:57:31Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (7, 'criteria check misdiagnoses unmet owner-attested criteria as ''a runnable criterion failed''', 'DONE', 'standalone criteria check exits 1 only for failed runnables; close still blocks on unmet attested (condition 3); fixture in criteria.txtar', '', NULL, 'v0-bootstrap', '2026-07-23T22:29:35Z', '2026-07-24T01:57:31Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (8, 'import: DUPLICATE with a later-positioned canonical refuses (forward dup_of)', 'DONE', 'Closed as a documented limitation on owner ruling 2026-07-24: docs/migration-guide.md section 3 already states the authoring constraint from the pilot rehearsal — a DUPLICATE''s canonical precedes it in the corpus, dup_of is the canonical''s corpus-order id, and a forward reference refuses. No code change ordered.', '', NULL, NULL, '2026-07-24T01:27:12Z', '2026-07-24T21:41:08Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (9, 'import --legacy: an unresolvable cited commit is accepted silently; only R5 catches it', 'DONE', 'The stored value stays verbatim — spec 6.2 mandates it so R5 can flag the typo rather than legacy: masking it. What was missing was the import-time signal: the importer now names each sha-shaped token that resolves to no commit, using the same warnf channel as the date-disagreement warning. Asserted end to end in import-typo.txtar.', '', NULL, NULL, '2026-07-24T01:28:05Z', '2026-07-24T20:30:31Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (10, 'chaining recipe: a guarded chained line would survive directory-deletion abandonment', 'DONE', 'Fixed via amendment chaining-recipe-guards-abandonment (spec rev 3.24, commit ab20d2f): both printed lines are existence-guarded, so deleting .selftracked/ no longer blocks the host repository''s commits with 127. All three cases (absent, RED, green) proven by hand before the change; activate_test.go pins the guarded lines; make gates green. (Commit sha re-stamped to the published value after the 2026-07-24 pre-push leak scrub, per D-EP11.)', '', NULL, NULL, '2026-07-24T01:49:31Z', '2026-07-24T23:29:26Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (11, 'Pre-release review pass', 'DONE', 'Fulfilled by the 2026-07-24 pre-publication work, closed on owner ruling: audit round 1 (e5ad831), the red-team self-audit dossier and privacy research (private archive, 2026-07-24), the hygiene pass recorded as task #14 (0c55ceb), and the owner-sanctioned history rewrite with evidence re-stamp (ef1d4f7). Publication to the public remote happened the same day; the go-public gate this task guarded has been passed.', '', NULL, NULL, '2026-07-24T12:56:09Z', '2026-07-24T21:05:23Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (12, 'PROMPT.md verb catalog omits gate and import; verify the deny-list claim''s wording against the live instance', 'DONE', 'Fixed in the generator: PROMPT.md''s catalog now lists gate and import (Maintenance & state), so the correction reaches every adopter, and cmd/selftracked/catalog_doc_test.go compares the generated file against the registry the binary installs, so the next omission fails the build rather than shipping. Golden regenerated. Part (b) — the settings.json deny entry — was already corrected on 2026-07-24.', '', NULL, NULL, '2026-07-24T13:01:13Z', '2026-07-24T20:27:37Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (13, 'Harden the agent push barrier (this repo)', 'DONE', 'Closed on owner ruling 2026-07-24: the control is in place — git push denied at the permission layer (.claude/settings.json deny rules, e4aa352) and the this-repo-only, no-template-change decision recorded (dd0693a). No further hardening ordered.', '', NULL, NULL, '2026-07-24T13:26:03Z', '2026-07-24T21:32:34Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (14, 'Pre-release repository hygiene pass', 'DONE', 'Completed; detail in the private research archive', '', NULL, NULL, '2026-07-24T14:21:47Z', '2026-07-24T15:11:20Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (15, 'Full Windows support (post-v0): hooks, symlinks, sqlite locking, path separators', 'OPEN', 'v0 targets POSIX (spec 16); Windows CI verifies only dump byte-determinism. Full support is future work. First cross-platform CI run surfaced: pre-commit hook tests fail on Windows (generated hook is POSIX sh, run directly without a shell); symlink-containment tests need Windows symlink privileges; TestGateRaceOneWinner sees different sqlite file-locking semantics; TestScripts path-separator assumptions. Scope a Windows-portability pass if/when Windows becomes a target.', 'Post-v0; v0 explicitly targets POSIX per spec 16', NULL, NULL, '2026-07-24T16:39:08Z', '2026-07-25T01:14:52Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (16, 'Argument-shape guard for dump-supplied tokens reaching git argv (verify/import-dates)', 'DONE', 'Fixed: R5 refuses a leading-dash citation before git cat-file; the import-dates git show sink was already shaShape-gated (internal/verb/import_dates.go:35, pre-existing). stale --since refuses a leading-dash revision. Red fixtures proven red without the guards.', '', NULL, NULL, '2026-07-24T20:05:41Z', '2026-07-24T20:22:36Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (17, 'git mv in paths move receives dictionary-supplied roots with no -- separator (CWE-88 shape)', 'DONE', 'Fixed: git mv now takes a -- separator, and paths move validates the new root like paths set does (one column, one validation). Covered by dict_root_test.go plus the existing git-transport case in paths-move.txtar.', '', NULL, NULL, '2026-07-24T20:05:41Z', '2026-07-24T20:22:36Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (18, 'epic show omits the linked-artifacts section (ADRs and evidence are invisible to a reader)', 'DONE', 'Fixed via amendment show-verbs-print-linked-artifacts (spec rev 3.25, commit b653b15): show <id> and epic show list linked artifacts as class[@scope]:relpath (role), archived marked, text and --json ([] when empty). link.txtar asserts it; live check: ADR 0001 now prints on this task (grounding) and on epic:v0-bootstrap (adr). (Commit sha re-stamped to the published value after the 2026-07-24 pre-push leak scrub, per D-EP11.)', '', NULL, NULL, '2026-07-24T20:44:34Z', '2026-07-24T23:29:26Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (19, 'Bookkeeping commit with an empty index refuses despite the hook staging dump/STATE', 'DONE', 'Fixed in the generated guidance: claude_rule.md and claude_skill.md templates (+ this repo''s instances and goldens) now spell the explicit staging command for the bookkeeping commit, with the why (git refuses a commit whose only content was hook-staged when the index started empty; reproduced in a scratch repo). Asserted by content tests; go test ./... 12/12 ok.', '', NULL, 'pilot-adaptation', '2026-07-25T01:25:43Z', '2026-07-25T01:28:51Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (20, 'Probe: fresh-agent working loop on an adapted host', 'DONE', 'Probe executed on a sandbox host copy: a fresh-context agent, given only the generated PROMPT.md, ran the full loop - prime, create, a file change committed through the host''s chained hook stack (host gate silent on success; selftracked printed its state-refresh and staged-refresh lines), set-status DONE with --note, explicit-staging bookkeeping commit (plain commit refused on empty index exactly as #19 documents), verify 0 violations with the advisory census unchanged, clean tree. Coordinator independently re-verified commits, task state, verify output and file content. Four PROMPT.md usability gaps surfaced; filed as a follow-up task.', '', NULL, 'pilot-adaptation', '2026-07-25T07:56:21Z', '2026-07-25T08:01:28Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (21, 'PROMPT.md: close the usability gaps a fresh-agent probe surfaced', 'DONE', 'Template rewritten: working-loop section (prime named as the session-start read; bookkeeping commit with explicit staging and the empty-index rationale), full verb catalog with signatures, and the task/story/epic status vocabularies with transition rules (set-status note-rewrite trap documented). Repo''s own PROMPT.md instance refreshed from the template (it predated even the gate/import fix). Evidence: go test ./... 12/12 ok incl. 8 new content assertions and the catalog honesty test; commit 4e546e2. CLI usage strings left untouched: the spec''s verb table pins signatures, so enriching them is amendment territory - PROMPT.md is where the spec says the catalog lives.', '', NULL, NULL, '2026-07-25T08:01:28Z', '2026-07-25T08:12:46Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (22, 'list --epic accepts any slug silently: unknown or epic:-prefixed slugs return 0 rows, exit 0', 'OPEN', 'A nonexistent epic slug (or the epic:SLUG form that epic list itself prints) filters to an empty result with no diagnostic - indistinguishable from an epic with no tasks. Found by the S5 read-surface sweep; epic show refuses the same input loudly.', '', NULL, NULL, '2026-07-25T08:23:47Z', '2026-07-25T08:23:47Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (23, 'criteria check prints nothing on success', 'OPEN', 'Zero bytes on stdout (plain and --json), exit 0, for an epic whose criteria are all met - a caller cannot distinguish checked-and-green from did-nothing. verify prints a summary even when clean; criteria check should say N/N met. Found by the S5 read-surface sweep.', '', NULL, NULL, '2026-07-25T08:23:47Z', '2026-07-25T08:23:47Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (24, 'Verbs never say which tracker they operate on', 'OPEN', 'A verb resolves .selftracked/ from the working directory and gives no cue in its output; with two trackers on one machine (a coordinating repo plus a sandbox host) a wrong-cwd write lands silently in the wrong database - happened once during the S5 sweep. Possibly a one-line repo identifier in write-verb output or prime; design call.', '', NULL, NULL, '2026-07-25T08:23:47Z', '2026-07-25T08:23:47Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (25, 'Scaffolded divergence recipe names a verb form that always refuses', 'DONE', 'Templates and instances corrected to load --force with the discard warning and the re-apply-local-writes step; plain load''s refusal documented. Evidence: go test ./... 12/12 incl. 3 new content assertions; commit recorded in git history. P5 docs-only exception: no verb behavior touched.', '', NULL, NULL, '2026-07-25T08:42:33Z', '2026-07-25T08:43:39Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (26, 'create --help does not state the default status', 'OPEN', 'Bare create lands as NEEDS-TRIAGE (deliberate: unknown is data), but the usage line''s [--status OPEN|IN-REVIEW|NEEDS-TRIAGE] never says which one is the default; two S5 block agents in a row assumed OPEN and had to correct course. PROMPT.md documents it since #21; --help should too.', '', NULL, NULL, '2026-07-25T08:53:40Z', '2026-07-25T08:53:40Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (27, '--dup-of rejects the #N id form every other argument accepts', 'OPEN', 'set-status <id> accepts #N or N; --dup-of "#74" dies with a flag parse error (exit 2), only bare integers pass. Inconsistent ref grammar within a single command. S5 block B.', '', NULL, NULL, '2026-07-25T08:53:40Z', '2026-07-25T08:53:40Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (28, 'Refusal surface is split between domain messages and raw SQLite text', 'OPEN', 'Some refusals are clean domain envelopes (use-reopen, dup-chain, not-terminal); others leak raw CHECK/trigger text with SQLite error codes appended - park on a DONE task, DONE without note, LABEL transition (blocks B/C), and md-table import status-enum violations, which additionally fail to name the offending row while the importer''s own format and dup-target errors do (block G). Same surface, two message registers. S5.', '', NULL, NULL, '2026-07-25T08:53:40Z', '2026-07-25T09:12:11Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (29, '--json silently ignored by rel add/rm/tree/cycles and paths ls', 'OPEN', 'The spec promises --json on every verb; rel add/rm/tree/cycles, paths ls/set, the epic family (list, show), and import''s success line all accept the flag and emit plain text anyway - only list and verify were observed genuinely honoring --json on the success path. Error envelopes are JSON everywhere (import errors are JSON even without the flag). Breaks scripted consumption of the documented contract. S5 blocks B, C, D, E, G.', '', NULL, NULL, '2026-07-25T08:53:40Z', '2026-07-25T09:12:11Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (30, 'show does not surface a DUPLICATE task''s canonical', 'OPEN', 'show #N on a DUPLICATE prints only [DUPLICATE]; neither plain nor --json output carries dup_of. The one fact that status exists to record is only reachable via rel tree. S5 block B.', '', NULL, NULL, '2026-07-25T08:53:40Z', '2026-07-25T08:53:40Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (31, 'list --status LABEL returns 0 rows unless --labels is also passed', 'OPEN', 'The default label-exclusion is applied before the status filter, so explicitly asking for LABEL rows yields nothing without the second flag; nothing in --help says the flags interact. S5 blocks B and C.', '', NULL, NULL, '2026-07-25T08:53:40Z', '2026-07-25T08:53:40Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (32, 'rel''s usage-class refusal exits 1, not 2', 'OPEN', 'rel add X duplicates Y returns code:usage in its envelope but process exit 1 (the domain-refusal code); the spec''s three-way exit split says usage belongs to 2. Either the envelope code or the exit code is wrong. S5 block C.', '', NULL, NULL, '2026-07-25T08:53:40Z', '2026-07-25T08:53:40Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (33, 'Does an archived home link satisfy R13?', 'IN-REVIEW', 'PO: link archive --force on an artifact that is a task''s home leaves the task OUT of the R13 no-home-link census - an archived home still counts as a home. Observed in S5 block C. Is that the intended semantics (history preserved = homed), or should archiving revive the advisory?', '', NULL, NULL, '2026-07-25T08:53:40Z', '2026-07-25T08:53:40Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (34, 'Restarting the story that holds the WIP slot names itself as the blocker', 'OPEN', 'story start on the story that is ALREADY IN-PROGRESS returns the generic WIP envelope - story S1 is IN-PROGRESS, finish or block it first - about itself, instead of a distinct already-in-progress message. Misleading self-reference. S5 block D.', '', NULL, NULL, '2026-07-25T09:01:16Z', '2026-07-25T09:01:16Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (35, 'Does a CLOSED epic accept new criteria - and should verify notice an unmet one there?', 'IN-REVIEW', 'PO: criteria add on a CLOSED epic is accepted (spec''s criteria row states no status restriction); a runnable criterion added post-close can then FAIL and stay open on the closed epic, and no verify rule flags the inconsistency - the close gate is transition-time only, not an invariant. Intended post-close validation surface (like V-rows), or a gap? S5 block D.', '', NULL, NULL, '2026-07-25T09:01:16Z', '2026-07-25T09:01:16Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (36, 'Path dictionary: a mistyped class is permanent and unvalidated', 'OPEN', 'paths has no unset/remove verb, class names take any string including spaces, and re-registering a class silently drops --ephemeral/--note instead of merging. One exploratory sweep left four irremovable scratch rows. S5 block E.', '', NULL, NULL, '2026-07-25T09:12:11Z', '2026-07-25T09:12:11Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (37, 'paths set accepts a nonexistent root; the mistake surfaces later as RED R2', 'OPEN', 'No existence check at set time; the typo turns up as FAIL R2 on the next verify, which also bricks the pre-commit gate until fixed. Cheap to catch at the verb. S5 block E.', '', NULL, NULL, '2026-07-25T09:12:11Z', '2026-07-25T09:12:11Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (38, 'paths move --with-files cannot see untracked files and says the directory is empty', 'OPEN', 'The move shells out to git mv, so files not yet git add-ed are invisible and the refusal reads git mv failed: source directory is empty while the directory plainly holds files. Also asymmetric preconditions: the dictionary-only move tolerates an existing destination, --with-files demands a fresh one. S5 block E.', '', NULL, NULL, '2026-07-25T09:12:11Z', '2026-07-25T09:12:11Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (39, 'md-table import: a corpus with no recognized section heading imports as a silent no-op', 'OPEN', 'A file whose tables lack the exact lowercase ## tasks / ## epics headings yields imported 0 epic(s), 0 story(ies), 0 task(s), 0 worklog row(s), exit 0 - an entire corpus ignored with a green exit. Two of ten discovery iterations hit this. S5 block G.', '', NULL, NULL, '2026-07-25T09:12:11Z', '2026-07-25T09:12:11Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (40, 'md-table import: an unknown extra column is silently discarded', 'OPEN', 'If the five required task columns are present, any additional column parses fine and its data vanishes - not folded into note, not warned about. Silent data loss on import. Column validation only fires when a REQUIRED column goes unmatched. S5 block G.', '', NULL, NULL, '2026-07-25T09:12:11Z', '2026-07-25T09:12:11Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (41, 'The md-table import format is documented nowhere', 'OPEN', 'Not in import --help, not in the migration guide (which shows only the JSON shape), not in the spec''s import row, not in anything the scaffold ships - the section headings and column names exist only in import_mdtable.go. A fresh adopter needed 10 trial-and-error iterations against error messages to reach a green import. S5 block G.', '', NULL, NULL, '2026-07-25T09:12:11Z', '2026-07-25T09:12:11Z');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('pilot-adaptation', 'S1', 'Relocation baseline: a fresh sandbox copy passes its own native checks at the new location; breakages recorded locally', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('pilot-adaptation', 'S2', 'Deploy recipe: init + hook colocation with the host''s existing hook chain on a fresh copy, selftracked verify green', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('pilot-adaptation', 'S3', 'Legacy import: the host''s existing backlog imports on the sandbox copy with recorded fidelity checks', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('pilot-adaptation', 'S4', 'Repeatability: the full documented recipe passes end-to-end on a fresh copy with no manual forks, twice consecutively', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('pilot-adaptation', 'S5', 'Exercise the full verb surface on an adapted sandbox host', 'IN-PROGRESS', 'Coverage checklist reaches zero uncovered verbs/subcommands, each with a recorded exit code; every refusal case refuses; dump-load-dump byte-identical; final verify 0 violations; findings filed as tasks', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S1', 'G0 - traceability inventory', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S10', 'S5b - relation, artifact and dictionary verbs', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S11', 'S6 - epic, story, worklog and criteria verbs', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S12', 'S7 - verify', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S13', 'S8a - init scaffold and generated docs', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S14', 'S8b - hooks and sidecar matrix', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S15', 'S8c - state, prime, SessionStart', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S16', 'S9 - import, the backfill door', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S17', 'S10 - dogfood switchover', 'DONE', 'corpus imported through the S9 door; selftracked verify green on this repo''s live tracker; bootstrap ledger deleted; inventory file retired (git history keeps both)', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S18', 'S11 - version gate and migration branches', 'DONE', 'schema version gate + migration branches per plan section 4 S11; INV-464 migrated field rides here', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S19', 'S12 - pilot ladder and remaining deliverables', 'DONE', 'pilot rungs 3-4; README; CONTRIBUTING (DCO + AI clause); generic migration guide (INV-449/450 ride here); docs link-check', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S2', 'S0 - repo bootstrap', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S3', 'S1a - schema as text', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S4', 'S1b - schema gates', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S5', 'S1c - driver behaviour', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S6', 'S2 - CLI dispatcher', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S7', 'S3 - serializer and dump', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S8', 'S4 - load and parser fuzzing', 'DONE', '', '', '', '');
INSERT INTO stories (epic, id, title, status, dod, consumes, produces, blocked) VALUES ('v0-bootstrap', 'S9', 'S5a - task-lifecycle verbs', 'DONE', '', '', '', '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('pilot-adaptation', 1, 'S1', '2026-07-25T01:12:15Z', 'IN-PROGRESS', '', '', '', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('pilot-adaptation', 2, 'S1', '2026-07-25T01:20:02Z', 'DONE', '39f94ac', 'native checks of the sandbox copy at the relocated path: 8/9 green; the one red is content-determined and reproduced by identical bytes+history at origin (location-independent, pre-existing) - detail in the local run log', '', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('pilot-adaptation', 3, 'S2', '2026-07-25T01:20:10Z', 'IN-PROGRESS', '', '', '', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('pilot-adaptation', 4, 'S2', '2026-07-25T01:22:03Z', 'DONE', '39f94ac', 'init on a fresh copy: incumbent-hooks detection correct, guarded chaining recipe applied, selftracked verify 0/0, end-to-end host commit through both hook chains green', '', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('pilot-adaptation', 5, 'S3', '2026-07-25T01:22:03Z', 'IN-PROGRESS', '', '', '', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('pilot-adaptation', 6, 'S3', '2026-07-25T01:45:15Z', 'DONE', 'be1669e', 'corpus derived from the host''s prose by a fresh subagent under a written mapping spec; import --legacy exit 0 first try; verify full 0 violations (advisories read and accepted); fidelity cross-checked by independent counts on source, manifest and dump - all classes equal; detail in the local run log', '', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('pilot-adaptation', 7, 'S4', '2026-07-25T01:45:15Z', 'IN-PROGRESS', '', '', '', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('pilot-adaptation', 8, 'S4', '2026-07-25T01:50:45Z', 'DONE', 'be1669e', 'two consecutive fresh sandbox copies adopted by fresh-context agents from the handoff instruction alone: init + chaining + import, verify 0 violations, fidelity table exact on both; final run reported zero deviations', 'the second run''s report states every step''s observed output matched the stated expectation; run logs local', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('pilot-adaptation', 9, 'S5', '2026-07-25T08:15:06Z', 'IN-PROGRESS', '', '', '', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 1, 'S1', '2026-07-18T23:39:52Z', 'DONE', '3f560d8', 'python3 scripts/check-inventory.py exit 0', '', NULL, '545 obligations extracted from the spec and ratified as the control artifact (D-EP4); three review passes; governance amendments plan-accounting-scope and stage-open-plan-crosscheck filed at close (plan revs 5-6).');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 2, 'S2', '2026-07-18T23:59:39Z', 'DONE', 'dbf9328', 'make gates exit 0 (local, D-EP8)', '', NULL, 'Repo bootstrap: Makefile gates chain, check-inventory, probe scripts. Close review found two fail-open probes (reported success without go on PATH) - both refuse now. Amendments review-proportionality-tiers, local-commits-and-interim-evidence, s0-minimal-package, split-s1 (plan revs 7-10).');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 3, 'S3', '2026-07-19T07:21:29Z', 'DONE', '3ae73e3', 'make gates exit 0 (local, D-EP8)', '', NULL, 'Schema as text + connection posture. Close found the DSN setting the one journal mode the spec rules out, and proved the DDL tests could not fail by deleting a trigger. Amendment nullable-columns-preamble - the first to the specification (spec rev 3.10).');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 4, 'S4', '2026-07-19T10:30:12Z', 'DONE', '1b9ca8d', 'make gates exit 0, 114 subtests, -race, fresh cache', '', NULL, 'Schema gates; first stage under the D-EP13 opening record. Five mutation probes shown red. Amendments worklog-story-guard-rule-pointer (spec rev 3.12) and epic-close-story-cardinality (3.13); the governance trio stage-open-record, evidence-across-a-squash, pre-authorized-amendment-cadence landed this window (plan revs 11-13).');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 5, 'S5', '2026-07-19T10:45:04Z', 'DONE', '7d12e31', 'make gates exit 0 (local, D-EP8)', '', NULL, 'Driver behaviour: Serialize byte-identity, pragma posture, the _dqs=0 probe. One accepted fix: an unchecked type assertion made uniform with its siblings.');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 6, 'S6', '2026-07-19T11:10:50Z', 'DONE', 'f55097c', 'make gates exit 0 (local, D-EP8)', '', NULL, 'CLI dispatcher: closed verb set, section 3.2 flag posture. The bare word help - scope never granted - was deleted outright, dissolving the verb-shadowing it had silently created.');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 7, 'S7', '2026-07-19T11:32:05Z', 'DONE', '0a1d444', 'make gates exit 0 (local, D-EP8)', '', NULL, 'Deterministic serializer + dump. The mutation story: two probes struck as no-ops (green against an unmutated file proves nothing), an ORDER BY removal surviving black-box fixtures killed by a white-box guard, and the critic proved a second surviving mutant (duplicated DDL block) the same way.');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 8, 'S8', '2026-07-19T19:30:39Z', 'DONE', '0b9aac8', 'make gates exit 0 (local, D-EP8)', '', NULL, 'load + whitelist-parser fuzzing - the security boundary. Adversarial close weighted parser bypass hardest and found none; the statement-boundary attack round-trips as one literal, hand-verified. Amendment import-date-bounds (spec rev 3.11) had already sharpened the import obligations this stage''s parser serves.');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 9, 'S9', '2026-07-19T20:18:49Z', 'DONE', '69be520', 'make gates exit 0 (local, D-EP8)', '', NULL, 'Task-lifecycle verbs and the write pipeline. Close critic hand-drove the binary: five resolved-but-unfixtured rows and a latent section 6.1 order inversion (sidecar before STATE.md slot) - all fixed before any row flipped.');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 10, 'S10', '2026-07-19T21:19:54Z', 'DONE', '3e95828', 'make gates exit 0 (local, D-EP8)', '', NULL, 'Relation/artifact/dictionary verbs. Four blockers fixed pre-flip: symlink containment escape, root-move nesting corruption, --with-files zero coverage, untested epic-link path. Amendments link-tables-are-relations-not-history (spec rev 3.14) and instance-scoped-events-and-r8 (3.15) applied mid-stage.');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 11, 'S11', '2026-07-19T21:57:54Z', 'DONE', '78a6dff', 'make gates exit 0 (local, D-EP8)', '', NULL, 'Epic/story/worklog/criteria verbs - the largest verb stage. Real INV-119 blocker (terminal re-transition duplicating worklog episodes) fixed via a source-guarded helper; invented scope (ready-requires-DoD) removed. Amendment dod-shape-is-authoring-convention (spec rev 3.16).');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 12, 'S12', '2026-07-20T09:03:56Z', 'DONE', '34f9b6f', 'make gates exit 0 (local, D-EP8)', '', NULL, 'verify: R1-R15 with per-rule red fixtures and the mechanized coverage audit. R1 check 2 double-count fixed; R9/R10 tightened to the literal rules. The semantics critic found the set-status DUPLICATE poison pill (fixed 2026-07-23, pre-S10). Amendment r14-rides-its-renderer-at-s8c (plan rev 14).');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 13, 'S13', '2026-07-20T09:59:29Z', 'DONE', '7ee7e0d', 'make gates exit 0 (local, D-EP8)', '', NULL, 'init scaffold + generated docs. Two data-loss bugs caught by critics before the flip: init clobbering a clone''s tracked dump, and --force wiping the DB (now a refresh). The section 6.1 order inversion recreated via old WriteFiles - fixed the pipeline way.');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 14, 'S14', '2026-07-20T14:03:21Z', 'DONE', '1239eb9', 'make gates exit 0 (local, D-EP8)', '', NULL, 'Git hooks + the section 8.4 sidecar matrix. Six robustness defects fixed pre-flip, sharpest: os.WriteFile setting mode only on creation left a refreshed hook non-executable - git skips it silently. Amendments gate-skip-joins-the-r8-carve-out (spec rev 3.17) and prime-divergence-rides-prime-at-s8c (plan rev 15).');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 15, 'S15', '2026-07-20T18:12:57Z', 'DONE', '9247018', 'make gates exit 0 (local, D-EP8)', '', NULL, 'state, prime, SessionStart, R1 check 3. load must NOT regenerate STATE.md (would mask committed drift). Accepted fixes: reflective prose-scan, atomicWrite TOCTOU, fourth SessionStart branch, non-git stale degradation. Amendment migrated-field-rides-migration-at-s11 (plan rev 16).');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 16, 'S16', '2026-07-21T03:50:39Z', 'DONE', 'acdd980', 'make gates exit 0 (local, D-EP8)', 'five fresh critics + verification re-critic; adjudication: docs/research/2026-07-21-s9-import-critic-round.md', NULL, 'import --legacy - the one sanctioned backfill door. Batch reader before the shared write pipeline in one transaction; git-first date engine (author dates, newest cited commit); deterministic per-epic source map; three relaxations gated exactly. Amendment import-guide-reviews-ride-to-s12 (plan rev 17).');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 17, 'S17', '2026-07-23T20:50:45Z', 'IN-PROGRESS', '489f4c5', '', '', NULL, 'Opened per D-EP13 (docs/stage-openings/s10.md). Pre-stage: the dup-chain poison pill fixed (74525b9); the 2026-07-24 open-questions round ran five critics on the fork analysis and the owner ratified four amendments (spec revs 3.18-3.21), applied and verified. This import is the switchover itself.');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 18, 'S17', '2026-07-23T22:29:35Z', 'DONE', '489f4c5..fc5e5c6', 'make gates exit 0 (cleared cache, binary on PATH); selftracked verify full: 0 violations', 'Three-critic close round (plan obligations, corpus fidelity, mechanics re-run), findings adjudicated and applied. Erratum: epic criterion 7 says 26 amendments - the true count is 25 (openspec/changes/ has 25 directories; spec revs 3.10-3.21 = 12 plus plan revs 5-17 = 13), and criteria text is verb-immutable, so the correction lives here. The four 2026-07-24 amendments folded into the S17 record by name: paused-epic-sprint-goal-is-intended, load-prose-matches-load, rc-triage-verify-did-not-complete, backfill-parenthetical-names-its-relaxation.', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 19, 'S18', '2026-07-23T23:47:30Z', 'IN-PROGRESS', '', '', '', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 20, 'S18', '2026-07-24T01:15:16Z', 'DONE', 'ab10c78..32dcb18', 'make gates exit 0 on a cleared cache, 2026-07-24; full verify 0 violations on the live tracker under the gated binary', 'FULL three-critic round (spec fidelity, concurrency, security); 31 findings, 9 accepted and applied incl. the loser self-block and the swap-failure recovery; amendments migration-verify-battery-is-the-load-battery (spec 3.22) and migration-gate-mechanics (3.23), both owner-ratified 2026-07-24; close appendix in docs/stage-openings/s11.md', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 21, 'S19', '2026-07-24T01:22:51Z', 'IN-PROGRESS', '', '', '', NULL, '');
INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note) VALUES ('v0-bootstrap', 22, 'S19', '2026-07-24T01:51:54Z', 'DONE', 'ab10c78..92d5fe3', 'make gates exit 0; scripts/check-doc-links.sh exit 0; rehearsal rounds verify full 0 violations (archived); live client verify full 0 violations', 'FULL three-critic round (privacy, document fidelity, rehearsal mechanics); derivation defects found, fixed, rounds re-run and the live install redone pre-close; tasks #8-#10 filed; close appendix in docs/stage-openings/s12.md; public record docs/research/2026-07-24-pilot-import-rehearsal.md', NULL, '');
INSERT INTO artifacts (id, class, scope, relpath, archived, note) VALUES (1, 'adr', '', '0001-hostile-clone-filesystem-containment-is-out-of-scope.md', 0, '');
INSERT INTO task_artifacts (task, artifact, role, note) VALUES (18, 1, 'grounding', '');
INSERT INTO epic_artifacts (epic, artifact, role, note) VALUES ('v0-bootstrap', 1, 'adr', '');
INSERT INTO events (seq, at, entity, event, detail) VALUES (1, '2026-07-23T22:09:23Z', '#1', 'import', 'import OPEN (date:i)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (2, '2026-07-23T22:09:23Z', '#2', 'import', 'import OPEN (date:i)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (3, '2026-07-23T22:09:23Z', '#3', 'import', 'import OPEN (date:i)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (4, '2026-07-23T22:09:23Z', '#4', 'import', 'import OPEN (date:i)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (5, '2026-07-23T22:09:23Z', '#5', 'import', 'import OPEN (date:i)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (6, '2026-07-23T22:09:23Z', 'epic:v0-bootstrap', 'import', '1:g 2:g 3:g 4:g 5:g 6:g 7:g 8:g 9:g 10:g 11:g 12:g 13:g 14:g 15:g 16:g 17:g');
INSERT INTO events (seq, at, entity, event, detail) VALUES (7, '2026-07-23T22:28:38Z', '#5', 'edit', 'note: The publication boundary. Evidence recor…→The publication boundary. Evidence recor…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (8, '2026-07-23T22:28:38Z', '#6', 'create', 'epic show --json emits a JSON header line plus plain-text sections, not one JSON document');
INSERT INTO events (seq, at, entity, event, detail) VALUES (9, '2026-07-23T22:28:38Z', 'epic:v0-bootstrap', 'criteria', 'check: 1 line(s), failed=true');
INSERT INTO events (seq, at, entity, event, detail) VALUES (10, '2026-07-23T22:29:35Z', '#7', 'create', 'criteria check misdiagnoses unmet owner-attested criteria as ''a runnable criterion failed''');
INSERT INTO events (seq, at, entity, event, detail) VALUES (11, '2026-07-23T22:29:35Z', 'epic:v0-bootstrap/S17', 'story', 'done: 489f4c5..fc5e5c6; gate: make gates exit 0 (cleared cache, binary on PATH); selftracked verify full: 0 violations');
INSERT INTO events (seq, at, entity, event, detail) VALUES (12, '2026-07-23T23:47:30Z', 'epic:v0-bootstrap/S18', 'story', 'ready');
INSERT INTO events (seq, at, entity, event, detail) VALUES (13, '2026-07-23T23:47:30Z', 'epic:v0-bootstrap/S18', 'story', 'start');
INSERT INTO events (seq, at, entity, event, detail) VALUES (14, '2026-07-24T01:15:16Z', 'epic:v0-bootstrap/S18', 'story', 'done: ab10c78..32dcb18; gate: make gates exit 0 on a cleared cache, 2026-07-24; full verify 0 violations on the live tracker under the gated binary');
INSERT INTO events (seq, at, entity, event, detail) VALUES (15, '2026-07-24T01:15:24Z', 'epic:v0-bootstrap', 'criteria', 'met 3: S11 landed at commits ab10c78..32dcb18: the 8.6 gate before every verb (dispatcher + pipeline re-check), the migration engine on the load path with two hydration sources, escalation with the sentinel protocol, 8.4 branches (5)-(6), prime migrated:vK->vN; make gates exit 0 on a cleared cache 2026-07-24; FULL three-critic close review; spec revs 3.22-3.23');
INSERT INTO events (seq, at, entity, event, detail) VALUES (16, '2026-07-24T01:22:51Z', 'epic:v0-bootstrap/S19', 'story', 'ready');
INSERT INTO events (seq, at, entity, event, detail) VALUES (17, '2026-07-24T01:22:51Z', 'epic:v0-bootstrap/S19', 'story', 'start');
INSERT INTO events (seq, at, entity, event, detail) VALUES (18, '2026-07-24T01:27:12Z', '#8', 'create', 'import: DUPLICATE with a later-positioned canonical refuses (forward dup_of)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (19, '2026-07-24T01:28:05Z', '#9', 'create', 'import --legacy: an unresolvable cited commit is accepted silently; only R5 catches it');
INSERT INTO events (seq, at, entity, event, detail) VALUES (20, '2026-07-24T01:39:46Z', '#5', 'edit', 'note: The publication boundary. Evidence recor…→The publication boundary. Evidence recor…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (21, '2026-07-24T01:49:31Z', '#10', 'create', 'chaining recipe: a guarded chained line would survive directory-deletion abandonment');
INSERT INTO events (seq, at, entity, event, detail) VALUES (22, '2026-07-24T01:51:54Z', 'epic:v0-bootstrap/S19', 'story', 'done: ab10c78..92d5fe3; gate: make gates exit 0; scripts/check-doc-links.sh exit 0; rehearsal rounds verify full 0 violations (archived); live client verify full 0 violations');
INSERT INTO events (seq, at, entity, event, detail) VALUES (23, '2026-07-24T01:51:54Z', 'epic:v0-bootstrap', 'criteria', 'met 4: Rung 3: repeated from-scratch imports of the pilot corpus, verify full 0 violations each archived round, determinism by timestamp-normalized byte-identity; rung 4: colocated live install, incumbent hooks chained per recipe, host gate authoritative, verify 0 violations; docs/research/2026-07-24-pilot-import-rehearsal.md');
INSERT INTO events (seq, at, entity, event, detail) VALUES (24, '2026-07-24T01:51:54Z', 'epic:v0-bootstrap', 'criteria', 'met 5: README final, CONTRIBUTING (DCO + AI clause), docs/migration-guide.md (boundary, reconciliation, both rehearsal-found authoring constraints; example round-trips through import+verify), scripts/check-doc-links.sh exit 0 - commits ab10c78..92d5fe3, reviewed by the S12 fidelity critic');
INSERT INTO events (seq, at, entity, event, detail) VALUES (25, '2026-07-24T01:52:32Z', '#8', 'set-status', '');
INSERT INTO events (seq, at, entity, event, detail) VALUES (26, '2026-07-24T01:52:32Z', '#9', 'set-status', '');
INSERT INTO events (seq, at, entity, event, detail) VALUES (27, '2026-07-24T01:52:32Z', '#10', 'set-status', '');
INSERT INTO events (seq, at, entity, event, detail) VALUES (28, '2026-07-24T01:57:31Z', '#6', 'set-status', 'epic show --json emits one JSON document; fixture in epic.txtar; spec-conformant fix (6.1 --json contract)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (29, '2026-07-24T01:57:31Z', '#7', 'set-status', 'standalone criteria check exits 1 only for failed runnables; close still blocks on unmet attested (condition 3); fixture in criteria.txtar');
INSERT INTO events (seq, at, entity, event, detail) VALUES (30, '2026-07-24T02:03:51Z', '#5', 'edit', 'note: The publication boundary. Evidence recor…→The publication boundary. Prepared 2026-…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (31, '2026-07-24T12:56:09Z', '#11', 'create', 'Pre-release review pass');
INSERT INTO events (seq, at, entity, event, detail) VALUES (32, '2026-07-24T12:56:26Z', '#11', 'park', 'Parked; rationale in the private research archive');
INSERT INTO events (seq, at, entity, event, detail) VALUES (33, '2026-07-24T13:01:13Z', '#12', 'create', 'PROMPT.md verb catalog omits gate and import; verify the deny-list claim''s wording against the live instance');
INSERT INTO events (seq, at, entity, event, detail) VALUES (34, '2026-07-24T13:26:03Z', '#13', 'create', 'Harden the agent push barrier (this repo)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (35, '2026-07-24T13:32:34Z', '#13', 'edit', 'Fields updated; detail in the private research archive');
INSERT INTO events (seq, at, entity, event, detail) VALUES (36, '2026-07-24T13:32:43Z', '#13', 'park', 'Parked; rationale in the private research archive');
INSERT INTO events (seq, at, entity, event, detail) VALUES (37, '2026-07-24T14:21:47Z', '#14', 'create', 'Pre-release repository hygiene pass');
INSERT INTO events (seq, at, entity, event, detail) VALUES (38, '2026-07-24T14:21:47Z', '#14', 'park', 'Parked; rationale in the private research archive');
INSERT INTO events (seq, at, entity, event, detail) VALUES (39, '2026-07-24T14:41:11Z', '#5', 'edit', 'note: The publication boundary. Prepared 2026-…→Publication boundary: first push, gates …');
INSERT INTO events (seq, at, entity, event, detail) VALUES (40, '2026-07-24T14:41:11Z', '#11', 'edit', 'Fields updated; detail in the private research archive');
INSERT INTO events (seq, at, entity, event, detail) VALUES (41, '2026-07-24T14:41:11Z', '#13', 'edit', 'Fields updated; detail in the private research archive');
INSERT INTO events (seq, at, entity, event, detail) VALUES (42, '2026-07-24T14:41:11Z', '#14', 'edit', 'Fields updated; detail in the private research archive');
INSERT INTO events (seq, at, entity, event, detail) VALUES (43, '2026-07-24T14:42:56Z', '#13', 'edit', 'Fields updated; detail in the private research archive');
INSERT INTO events (seq, at, entity, event, detail) VALUES (44, '2026-07-24T15:11:20Z', '#14', 'set-status', 'Completed; detail in the private research archive');
INSERT INTO events (seq, at, entity, event, detail) VALUES (45, '2026-07-24T15:11:20Z', '#14', 'unpark', 'auto-cleared by status transition');
INSERT INTO events (seq, at, entity, event, detail) VALUES (46, '2026-07-24T16:39:08Z', '#15', 'create', 'Full Windows support (post-v0): hooks, symlinks, sqlite locking, path separators');
INSERT INTO events (seq, at, entity, event, detail) VALUES (47, '2026-07-24T16:39:08Z', '#15', 'park', 'Post-v0; v0 explicitly targets POSIX per spec 16');
INSERT INTO events (seq, at, entity, event, detail) VALUES (48, '2026-07-24T20:05:41Z', '#16', 'create', 'Argument-shape guard for dump-supplied tokens reaching git argv (verify/import-dates)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (49, '2026-07-24T20:05:41Z', '#17', 'create', 'git mv in paths move receives dictionary-supplied roots with no -- separator (CWE-88 shape)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (50, '2026-07-24T20:22:36Z', '#16', 'set-status', 'Fixed: R5 refuses a leading-dash citation before git cat-file; the import-dates git show sink was already shaShape-gated (internal/verb/import_dates.go:35, pre-existing). stale --since refuses a leading-dash revision. Red fixtures proven red without the guards.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (51, '2026-07-24T20:22:36Z', '#17', 'set-status', 'Fixed: git mv now takes a -- separator, and paths move validates the new root like paths set does (one column, one validation). Covered by dict_root_test.go plus the existing git-transport case in paths-move.txtar.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (52, '2026-07-24T20:27:37Z', '#12', 'set-status', 'Fixed in the generator: PROMPT.md''s catalog now lists gate and import (Maintenance & state), so the correction reaches every adopter, and cmd/selftracked/catalog_doc_test.go compares the generated file against the registry the binary installs, so the next omission fails the build rather than shipping. Golden regenerated. Part (b) — the settings.json deny entry — was already corrected on 2026-07-24.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (53, '2026-07-24T20:30:31Z', '#9', 'set-status', 'The stored value stays verbatim — spec 6.2 mandates it so R5 can flag the typo rather than legacy: masking it. What was missing was the import-time signal: the importer now names each sha-shaped token that resolves to no commit, using the same warnf channel as the date-disagreement warning. Asserted end to end in import-typo.txtar.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (54, '2026-07-24T20:35:49Z', 'epic:v0-bootstrap', 'criteria', 'check: 1 line(s), failed=true');
INSERT INTO events (seq, at, entity, event, detail) VALUES (55, '2026-07-24T20:36:04Z', 'epic:v0-bootstrap', 'criteria', 'check: 1 line(s), failed=false');
INSERT INTO events (seq, at, entity, event, detail) VALUES (56, '2026-07-24T20:38:19Z', 'epic:v0-bootstrap', 'link', 'adr adr:0001-hostile-clone-filesystem-containment-is-out-of-scope.md');
INSERT INTO events (seq, at, entity, event, detail) VALUES (57, '2026-07-24T20:44:34Z', '#18', 'create', 'epic show omits the linked-artifacts section (ADRs and evidence are invisible to a reader)');
INSERT INTO events (seq, at, entity, event, detail) VALUES (58, '2026-07-24T20:44:43Z', '#18', 'link', 'grounding adr:0001-hostile-clone-filesystem-containment-is-out-of-scope.md');
INSERT INTO events (seq, at, entity, event, detail) VALUES (59, '2026-07-24T20:45:01Z', '#18', 'edit', 'note: epic show prints stories, criteria and w…→Both show verbs omit linked artifacts, c…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (60, '2026-07-24T21:05:02Z', '#11', 'unpark', '');
INSERT INTO events (seq, at, entity, event, detail) VALUES (61, '2026-07-24T21:05:02Z', '#11', 'set-status', 'Fulfilled by the 2026-07-24 pre-publication work, closed on owner ruling: audit round 1 (e5ad831), the red-team self-audit dossier and privacy research (private archive, 2026-07-24), the hygiene pass recorded as task #14 (0c55ceb), and the owner-sanctioned history rewrite with evidence re-stamp (ef1d4f7..). Publication to the public remote happened the same day; the go-public gate this task guarded has been passed.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (62, '2026-07-24T21:05:23Z', '#11', 'edit', 'note: Fulfilled by the 2026-07-24 pre-publicat…→Fulfilled by the 2026-07-24 pre-publicat…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (63, '2026-07-24T21:32:34Z', '#13', 'unpark', '');
INSERT INTO events (seq, at, entity, event, detail) VALUES (64, '2026-07-24T21:32:34Z', '#13', 'set-status', 'Closed on owner ruling 2026-07-24: the control is in place — git push denied at the permission layer (.claude/settings.json deny rules, e4aa352) and the this-repo-only, no-template-change decision recorded (dd0693a). No further hardening ordered.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (65, '2026-07-24T21:41:08Z', '#10', 'set-status', 'Fixed via amendment chaining-recipe-guards-abandonment (spec rev 3.24, commit b9a964f): both printed lines are existence-guarded, so deleting .selftracked/ no longer blocks the host repository''s commits with 127. All three cases (absent, RED, green) proven by hand before the change; activate_test.go pins the guarded lines; make gates green.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (66, '2026-07-24T21:41:08Z', '#18', 'set-status', 'Fixed via amendment show-verbs-print-linked-artifacts (spec rev 3.25, commit 2613766): show <id> and epic show list linked artifacts as class[@scope]:relpath (role), archived marked, text and --json ([] when empty). link.txtar asserts it; live check: ADR 0001 now prints on this task (grounding) and on epic:v0-bootstrap (adr).');
INSERT INTO events (seq, at, entity, event, detail) VALUES (67, '2026-07-24T21:41:08Z', '#8', 'set-status', 'Closed as a documented limitation on owner ruling 2026-07-24: docs/migration-guide.md section 3 already states the authoring constraint from the pilot rehearsal — a DUPLICATE''s canonical precedes it in the corpus, dup_of is the canonical''s corpus-order id, and a forward reference refuses. No code change ordered.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (68, '2026-07-24T23:25:22Z', '#10', 'edit', 'note: Fixed via amendment chaining-recipe-guar…→Fixed via amendment chaining-recipe-guar…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (69, '2026-07-24T23:25:22Z', '#18', 'edit', 'note: Fixed via amendment show-verbs-print-lin…→Fixed via amendment show-verbs-print-lin…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (70, '2026-07-24T23:25:52Z', 'epic:v0-bootstrap', 'criteria', 'met 6: CI green on the published tree (origin/main 42a3e5b): GitHub run 30132992219 success on all jobs — policy gates, macOS full suite, Ubuntu full suite, Windows dump-determinism. Published WITHOUT squash (owner decision); the tree was history-rewritten pre-push only to re-stamp evidence (ef1d4f7) and scrub the pre-publication audit findings, not squashed. Verified-by-command worklog rows S1-S19 cite story shas <=19a608f, untouched by the scrub.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (71, '2026-07-24T23:26:23Z', '#5', 'set-status', 'First push done 2026-07-24: origin/main at 42a3e5b, GitHub CI run 30132992219 green on all jobs. Evidence re-stamped (ef1d4f7) and criterion 6 met. Published without squash (owner decision) — verified worklog rows S1-S19 cite story shas <=19a608f, untouched by the pre-push leak scrub.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (72, '2026-07-24T23:27:19Z', '#1', 'edit', 'epic: v0-bootstrap→');
INSERT INTO events (seq, at, entity, event, detail) VALUES (73, '2026-07-24T23:27:19Z', '#2', 'edit', 'epic: v0-bootstrap→');
INSERT INTO events (seq, at, entity, event, detail) VALUES (74, '2026-07-24T23:27:19Z', '#3', 'edit', 'epic: v0-bootstrap→');
INSERT INTO events (seq, at, entity, event, detail) VALUES (75, '2026-07-24T23:27:19Z', '#4', 'edit', 'epic: v0-bootstrap→');
INSERT INTO events (seq, at, entity, event, detail) VALUES (76, '2026-07-24T23:27:19Z', 'epic:v0-bootstrap', 'epic', 'close: 2026-07-24');
INSERT INTO events (seq, at, entity, event, detail) VALUES (77, '2026-07-24T23:29:26Z', '#10', 'edit', 'note: Fixed via amendment chaining-recipe-guar…→Fixed via amendment chaining-recipe-guar…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (78, '2026-07-24T23:29:26Z', '#18', 'edit', 'note: Fixed via amendment show-verbs-print-lin…→Fixed via amendment show-verbs-print-lin…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (79, '2026-07-25T01:04:29Z', 'epic:pilot-adaptation', 'epic', 'create: Adapt selftracked to deployment on an external pilot repository: debug the onboarding recipe (init, hook colocation, legacy import) on local sandbox copies until it deploys cleanly and repeatably; file every generic defect or missing feature it surfaces as ordinary public tasks. Pilot specifics never enter tracked content - records here are worded purely in terms of what selftracked does.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (80, '2026-07-25T01:04:29Z', 'epic:pilot-adaptation', 'epic', 'active');
INSERT INTO events (seq, at, entity, event, detail) VALUES (81, '2026-07-25T01:11:55Z', '#15', 'set-status', '');
INSERT INTO events (seq, at, entity, event, detail) VALUES (82, '2026-07-25T01:11:55Z', '#15', 'unpark', 'auto-cleared by status transition');
INSERT INTO events (seq, at, entity, event, detail) VALUES (83, '2026-07-25T01:11:55Z', 'epic:pilot-adaptation/S1', 'story', 'add: Relocation baseline: a fresh sandbox copy passes its own native checks at the new location; breakages recorded locally');
INSERT INTO events (seq, at, entity, event, detail) VALUES (84, '2026-07-25T01:11:55Z', 'epic:pilot-adaptation/S2', 'story', 'add: Deploy recipe: init + hook colocation with the host''s existing hook chain on a fresh copy, selftracked verify green');
INSERT INTO events (seq, at, entity, event, detail) VALUES (85, '2026-07-25T01:11:55Z', 'epic:pilot-adaptation/S3', 'story', 'add: Legacy import: the host''s existing backlog imports on the sandbox copy with recorded fidelity checks');
INSERT INTO events (seq, at, entity, event, detail) VALUES (86, '2026-07-25T01:11:55Z', 'epic:pilot-adaptation/S4', 'story', 'add: Repeatability: the full documented recipe passes end-to-end on a fresh copy with no manual forks, twice consecutively');
INSERT INTO events (seq, at, entity, event, detail) VALUES (87, '2026-07-25T01:11:55Z', 'epic:pilot-adaptation', 'criteria', 'add: The documented onboarding recipe deploys selftracked end-to-end (init, hooks, import) on two consecutive fresh sandbox copies with selftracked verify green and no manual forks');
INSERT INTO events (seq, at, entity, event, detail) VALUES (88, '2026-07-25T01:11:55Z', 'epic:pilot-adaptation', 'criteria', 'add: Import fidelity on the sandbox copy is verified by recorded checks: no source records lost or altered');
INSERT INTO events (seq, at, entity, event, detail) VALUES (89, '2026-07-25T01:11:55Z', 'epic:pilot-adaptation', 'criteria', 'add: Every generic defect surfaced by the experiments is filed as a public task; every fix applied carries tests and green gates');
INSERT INTO events (seq, at, entity, event, detail) VALUES (90, '2026-07-25T01:12:15Z', 'epic:pilot-adaptation/S1', 'story', 'ready');
INSERT INTO events (seq, at, entity, event, detail) VALUES (91, '2026-07-25T01:12:15Z', 'epic:pilot-adaptation/S1', 'story', 'start');
INSERT INTO events (seq, at, entity, event, detail) VALUES (92, '2026-07-25T01:14:52Z', '#15', 'edit', 'note: →v0 targets POSIX (spec 16); Windows CI v…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (93, '2026-07-25T01:14:52Z', '#15', 'park', 'Post-v0; v0 explicitly targets POSIX per spec 16');
INSERT INTO events (seq, at, entity, event, detail) VALUES (94, '2026-07-25T01:20:02Z', 'epic:pilot-adaptation/S1', 'story', 'done: 39f94ac; gate: native checks of the sandbox copy at the relocated path: 8/9 green; the one red is content-determined and reproduced by identical bytes+history at origin (location-independent, pre-existing) - detail in the local run log');
INSERT INTO events (seq, at, entity, event, detail) VALUES (95, '2026-07-25T01:20:10Z', 'epic:pilot-adaptation/S2', 'story', 'ready');
INSERT INTO events (seq, at, entity, event, detail) VALUES (96, '2026-07-25T01:20:10Z', 'epic:pilot-adaptation/S2', 'story', 'start');
INSERT INTO events (seq, at, entity, event, detail) VALUES (97, '2026-07-25T01:22:03Z', 'epic:pilot-adaptation/S2', 'story', 'done: 39f94ac; gate: init on a fresh copy: incumbent-hooks detection correct, guarded chaining recipe applied, selftracked verify 0/0, end-to-end host commit through both hook chains green');
INSERT INTO events (seq, at, entity, event, detail) VALUES (98, '2026-07-25T01:22:03Z', 'epic:pilot-adaptation/S3', 'story', 'ready');
INSERT INTO events (seq, at, entity, event, detail) VALUES (99, '2026-07-25T01:22:03Z', 'epic:pilot-adaptation/S3', 'story', 'start');
INSERT INTO events (seq, at, entity, event, detail) VALUES (100, '2026-07-25T01:25:43Z', '#19', 'create', 'Bookkeeping commit with an empty index refuses despite the hook staging dump/STATE');
INSERT INTO events (seq, at, entity, event, detail) VALUES (101, '2026-07-25T01:28:51Z', '#19', 'set-status', 'Fixed in the generated guidance: claude_rule.md and claude_skill.md templates (+ this repo''s instances and goldens) now spell the explicit staging command for the bookkeeping commit, with the why (git refuses a commit whose only content was hook-staged when the index started empty; reproduced in a scratch repo). Asserted by content tests; go test ./... 12/12 ok.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (102, '2026-07-25T01:45:15Z', 'epic:pilot-adaptation/S3', 'story', 'done: be1669e; gate: corpus derived from the host''s prose by a fresh subagent under a written mapping spec; import --legacy exit 0 first try; verify full 0 violations (advisories read and accepted); fidelity cross-checked by independent counts on source, manifest and dump - all classes equal; detail in the local run log');
INSERT INTO events (seq, at, entity, event, detail) VALUES (103, '2026-07-25T01:45:15Z', 'epic:pilot-adaptation/S4', 'story', 'ready');
INSERT INTO events (seq, at, entity, event, detail) VALUES (104, '2026-07-25T01:45:15Z', 'epic:pilot-adaptation/S4', 'story', 'start');
INSERT INTO events (seq, at, entity, event, detail) VALUES (105, '2026-07-25T01:50:45Z', 'epic:pilot-adaptation/S4', 'story', 'done: be1669e; gate: two consecutive fresh sandbox copies adopted by fresh-context agents from the handoff instruction alone: init + chaining + import, verify 0 violations, fidelity table exact on both; final run reported zero deviations');
INSERT INTO events (seq, at, entity, event, detail) VALUES (106, '2026-07-25T01:50:45Z', 'epic:pilot-adaptation', 'criteria', 'met 1: runs two and three: fresh-context agents executing only the handoff reached verify green (0 violations) end-to-end with no manual forks; third run deviation-free');
INSERT INTO events (seq, at, entity, event, detail) VALUES (107, '2026-07-25T01:50:45Z', 'epic:pilot-adaptation', 'criteria', 'met 2: per-status task counts, story/worklog/criteria row counts equal across source prose (independent greps), derivation manifest and imported dump on every rehearsal run');
INSERT INTO events (seq, at, entity, event, detail) VALUES (108, '2026-07-25T01:50:45Z', 'epic:pilot-adaptation', 'criteria', 'met 3: one generic defect surfaced (#19, bookkeeping commit vs empty index) - fixed in the scaffold templates with content tests, go test ./... 12/12 ok (be1669e); no other generic defect found by the rehearsals');
INSERT INTO events (seq, at, entity, event, detail) VALUES (109, '2026-07-25T07:56:21Z', '#20', 'create', 'Probe: fresh-agent working loop on an adapted host');
INSERT INTO events (seq, at, entity, event, detail) VALUES (110, '2026-07-25T08:01:28Z', '#20', 'set-status', 'Probe executed on a sandbox host copy: a fresh-context agent, given only the generated PROMPT.md, ran the full loop - prime, create, a file change committed through the host''s chained hook stack (host gate silent on success; selftracked printed its state-refresh and staged-refresh lines), set-status DONE with --note, explicit-staging bookkeeping commit (plain commit refused on empty index exactly as #19 documents), verify 0 violations with the advisory census unchanged, clean tree. Coordinator independently re-verified commits, task state, verify output and file content. Four PROMPT.md usability gaps surfaced; filed as a follow-up task.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (111, '2026-07-25T08:01:28Z', '#21', 'create', 'PROMPT.md: close the usability gaps a fresh-agent probe surfaced');
INSERT INTO events (seq, at, entity, event, detail) VALUES (112, '2026-07-25T08:12:46Z', '#21', 'set-status', 'Template rewritten: working-loop section (prime named as the session-start read; bookkeeping commit with explicit staging and the empty-index rationale), full verb catalog with signatures, and the task/story/epic status vocabularies with transition rules (set-status note-rewrite trap documented). Repo''s own PROMPT.md instance refreshed from the template (it predated even the gate/import fix). Evidence: go test ./... 12/12 ok incl. 8 new content assertions and the catalog honesty test; commit 4e546e2. CLI usage strings left untouched: the spec''s verb table pins signatures, so enriching them is amendment territory - PROMPT.md is where the spec says the catalog lives.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (113, '2026-07-25T08:14:53Z', 'epic:pilot-adaptation/S5', 'story', 'add: Exercise the full verb surface on an adapted sandbox host');
INSERT INTO events (seq, at, entity, event, detail) VALUES (114, '2026-07-25T08:15:06Z', 'epic:pilot-adaptation/S5', 'edit', 'dod: →Coverage checklist reaches zero uncovere…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (115, '2026-07-25T08:15:06Z', 'epic:pilot-adaptation/S5', 'story', 'ready');
INSERT INTO events (seq, at, entity, event, detail) VALUES (116, '2026-07-25T08:15:06Z', 'epic:pilot-adaptation/S5', 'story', 'start');
INSERT INTO events (seq, at, entity, event, detail) VALUES (117, '2026-07-25T08:23:47Z', '#22', 'create', 'list --epic accepts any slug silently: unknown or epic:-prefixed slugs return 0 rows, exit 0');
INSERT INTO events (seq, at, entity, event, detail) VALUES (118, '2026-07-25T08:23:47Z', '#23', 'create', 'criteria check prints nothing on success');
INSERT INTO events (seq, at, entity, event, detail) VALUES (119, '2026-07-25T08:23:47Z', '#24', 'create', 'Verbs never say which tracker they operate on');
INSERT INTO events (seq, at, entity, event, detail) VALUES (120, '2026-07-25T08:42:33Z', '#25', 'create', 'Scaffolded divergence recipe names a verb form that always refuses');
INSERT INTO events (seq, at, entity, event, detail) VALUES (121, '2026-07-25T08:43:39Z', '#25', 'set-status', 'Templates and instances corrected to load --force with the discard warning and the re-apply-local-writes step; plain load''s refusal documented. Evidence: go test ./... 12/12 incl. 3 new content assertions; commit recorded in git history. P5 docs-only exception: no verb behavior touched.');
INSERT INTO events (seq, at, entity, event, detail) VALUES (122, '2026-07-25T08:53:40Z', '#26', 'create', 'create --help does not state the default status');
INSERT INTO events (seq, at, entity, event, detail) VALUES (123, '2026-07-25T08:53:40Z', '#27', 'create', '--dup-of rejects the #N id form every other argument accepts');
INSERT INTO events (seq, at, entity, event, detail) VALUES (124, '2026-07-25T08:53:40Z', '#28', 'create', 'Refusal surface is split between domain messages and raw SQLite text');
INSERT INTO events (seq, at, entity, event, detail) VALUES (125, '2026-07-25T08:53:40Z', '#29', 'create', '--json silently ignored by rel add/rm/tree/cycles and paths ls');
INSERT INTO events (seq, at, entity, event, detail) VALUES (126, '2026-07-25T08:53:40Z', '#30', 'create', 'show does not surface a DUPLICATE task''s canonical');
INSERT INTO events (seq, at, entity, event, detail) VALUES (127, '2026-07-25T08:53:40Z', '#31', 'create', 'list --status LABEL returns 0 rows unless --labels is also passed');
INSERT INTO events (seq, at, entity, event, detail) VALUES (128, '2026-07-25T08:53:40Z', '#32', 'create', 'rel''s usage-class refusal exits 1, not 2');
INSERT INTO events (seq, at, entity, event, detail) VALUES (129, '2026-07-25T08:53:40Z', '#33', 'create', 'Does an archived home link satisfy R13?');
INSERT INTO events (seq, at, entity, event, detail) VALUES (130, '2026-07-25T09:01:16Z', '#34', 'create', 'Restarting the story that holds the WIP slot names itself as the blocker');
INSERT INTO events (seq, at, entity, event, detail) VALUES (131, '2026-07-25T09:01:16Z', '#35', 'create', 'Does a CLOSED epic accept new criteria - and should verify notice an unmet one there?');
INSERT INTO events (seq, at, entity, event, detail) VALUES (132, '2026-07-25T09:01:16Z', '#29', 'edit', 'note: The spec promises --json on every verb; …→The spec promises --json on every verb; …');
INSERT INTO events (seq, at, entity, event, detail) VALUES (133, '2026-07-25T09:12:11Z', '#36', 'create', 'Path dictionary: a mistyped class is permanent and unvalidated');
INSERT INTO events (seq, at, entity, event, detail) VALUES (134, '2026-07-25T09:12:11Z', '#37', 'create', 'paths set accepts a nonexistent root; the mistake surfaces later as RED R2');
INSERT INTO events (seq, at, entity, event, detail) VALUES (135, '2026-07-25T09:12:11Z', '#38', 'create', 'paths move --with-files cannot see untracked files and says the directory is empty');
INSERT INTO events (seq, at, entity, event, detail) VALUES (136, '2026-07-25T09:12:11Z', '#39', 'create', 'md-table import: a corpus with no recognized section heading imports as a silent no-op');
INSERT INTO events (seq, at, entity, event, detail) VALUES (137, '2026-07-25T09:12:11Z', '#40', 'create', 'md-table import: an unknown extra column is silently discarded');
INSERT INTO events (seq, at, entity, event, detail) VALUES (138, '2026-07-25T09:12:11Z', '#41', 'create', 'The md-table import format is documented nowhere');
INSERT INTO events (seq, at, entity, event, detail) VALUES (139, '2026-07-25T09:12:11Z', '#28', 'edit', 'note: Some refusals are clean domain envelopes…→Some refusals are clean domain envelopes…');
INSERT INTO events (seq, at, entity, event, detail) VALUES (140, '2026-07-25T09:12:11Z', '#29', 'edit', 'note: The spec promises --json on every verb; …→The spec promises --json on every verb; …');
