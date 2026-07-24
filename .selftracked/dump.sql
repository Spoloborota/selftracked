-- selftracked dump schema_version=1 tasks=18 artifacts=1
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
INSERT INTO epics (slug, goal, status, status_note, close_sweep, created_at) VALUES ('v0-bootstrap', 'Build selftracked v0 per docs/v0-spec.md and switch this repository to self-hosting', 'ACTIVE', '', '', '2026-07-18T23:39:52Z');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 1, 'Verb catalog, integrity engine (R1-R15), init, hooks, reader half, and import built; make gates green locally through S9', 1, 'git history: docs/v0-progress.md ledger @ 489f4c5; local gates run @ acdd980 (interim, D-EP8)');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 2, '$ selftracked verify', 1, 'PASS selftracked verify @ 2026-07-24T20:36:04Z');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 3, 'Schema version gate and migration branches ship (plan S11)', 1, 'S11 landed at commits ab10c78..32dcb18: the 8.6 gate before every verb (dispatcher + pipeline re-check), the migration engine on the load path with two hydration sources, escalation with the sentinel protocol, 8.4 branches (5)-(6), prime migrated:vK->vN; make gates exit 0 on a cleared cache 2026-07-24; FULL three-critic close review; spec revs 3.22-3.23');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 4, 'Pilot ladder rungs 3-4: gitignored disposable-clone import rehearsal, then colocated live install (plan S12)', 1, 'Rung 3: repeated from-scratch imports of the pilot corpus, verify full 0 violations each archived round, determinism by timestamp-normalized byte-identity; rung 4: colocated live install, incumbent hooks chained per recipe, host gate authoritative, verify 0 violations; docs/research/2026-07-24-pilot-import-rehearsal.md');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 5, 'README, CONTRIBUTING (DCO + AI clause), generic migration guide, docs link-check (plan S12)', 1, 'README final, CONTRIBUTING (DCO + AI clause), docs/migration-guide.md (boundary, reconciliation, both rehearsal-found authoring constraints; example round-trips through import+verify), scripts/check-doc-links.sh exit 0 - commits ab10c78..92d5fe3, reviewed by the S12 fidelity critic');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 6, 'First push: gates re-run against the squashed tree, verified rows re-stamped, CI green (D-EP11)', 0, '');
INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('v0-bootstrap', 7, 'Spec governed proposal-first end to end: every deviation an archived amendment (26 amendments through spec rev 3.21 / plan rev 17)', 1, 'openspec/changes/ directory; amendments log in git history (docs/v0-progress.md @ d4cddcc)');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (1, 'Align dump.sql file mode with the other tracked files (0600 from CreateTemp+rename vs 0644)', 'OPEN', 'Parked at the S8a close (code critic): more restrictive, not a leak; git tracks content, not mode. internal/dump owns it if it ever matters.', '', NULL, 'v0-bootstrap', '2026-07-23T22:09:23Z', '2026-07-23T22:09:23Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (2, 'Read-transaction for prime/state snapshot consistency, if the owner ever wants it', 'OPEN', 'Refuted as a defect at the S8c close under the single-writer axiom (spec section 1); a momentary off-by-a-few in advisory totals that self-heals on the next prime. Remedy on record: wrap prime/state reads in one read transaction.', '', NULL, 'v0-bootstrap', '2026-07-23T22:09:23Z', '2026-07-23T22:09:23Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (3, 'dump_divergence is a boolean and cannot distinguish a behind DB from a truly divergent one', 'OPEN', 'Post-v0 note from the 2026-07-24 open-questions round (fork analysis F2, architecture critic): both cases route to load --force + re-apply; the flag carries no distinguishing detail.', '', NULL, 'v0-bootstrap', '2026-07-23T22:09:23Z', '2026-07-23T22:09:23Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (4, 'Revisit the OpenSpec proposal+delta+tasks.md convention once the tool is installed', 'OPEN', 'Parked at the first-commit review: change directories hold proposal.md only; the tool is adopted but not installed, so the convention has nothing to run against yet.', '', NULL, 'v0-bootstrap', '2026-07-23T22:09:23Z', '2026-07-23T22:09:23Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (5, 'First push: re-run gates on the squashed tree and re-stamp every verified row (D-EP11)', 'OPEN', 'Publication boundary: first push, gates re-run on the published tree, criterion 6. Detail in the private research archive.', '', NULL, 'v0-bootstrap', '2026-07-23T22:09:23Z', '2026-07-24T14:41:11Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (6, 'epic show --json emits a JSON header line plus plain-text sections, not one JSON document', 'DONE', 'epic show --json emits one JSON document; fixture in epic.txtar; spec-conformant fix (6.1 --json contract)', '', NULL, 'v0-bootstrap', '2026-07-23T22:28:38Z', '2026-07-24T01:57:31Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (7, 'criteria check misdiagnoses unmet owner-attested criteria as ''a runnable criterion failed''', 'DONE', 'standalone criteria check exits 1 only for failed runnables; close still blocks on unmet attested (condition 3); fixture in criteria.txtar', '', NULL, 'v0-bootstrap', '2026-07-23T22:29:35Z', '2026-07-24T01:57:31Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (8, 'import: DUPLICATE with a later-positioned canonical refuses (forward dup_of)', 'OPEN', '', '', NULL, NULL, '2026-07-24T01:27:12Z', '2026-07-24T01:52:32Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (9, 'import --legacy: an unresolvable cited commit is accepted silently; only R5 catches it', 'DONE', 'The stored value stays verbatim — spec 6.2 mandates it so R5 can flag the typo rather than legacy: masking it. What was missing was the import-time signal: the importer now names each sha-shaped token that resolves to no commit, using the same warnf channel as the date-disagreement warning. Asserted end to end in import-typo.txtar.', '', NULL, NULL, '2026-07-24T01:28:05Z', '2026-07-24T20:30:31Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (10, 'chaining recipe: a guarded chained line would survive directory-deletion abandonment', 'OPEN', '', '', NULL, NULL, '2026-07-24T01:49:31Z', '2026-07-24T01:52:32Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (11, 'Pre-release review pass', 'DONE', 'Fulfilled by the 2026-07-24 pre-publication work, closed on owner ruling: audit round 1 (e5ad831), the red-team self-audit dossier and privacy research (private archive, 2026-07-24), the hygiene pass recorded as task #14 (0c55ceb), and the owner-sanctioned history rewrite with evidence re-stamp (ef1d4f7). Publication to the public remote happened the same day; the go-public gate this task guarded has been passed.', '', NULL, NULL, '2026-07-24T12:56:09Z', '2026-07-24T21:05:23Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (12, 'PROMPT.md verb catalog omits gate and import; verify the deny-list claim''s wording against the live instance', 'DONE', 'Fixed in the generator: PROMPT.md''s catalog now lists gate and import (Maintenance & state), so the correction reaches every adopter, and cmd/selftracked/catalog_doc_test.go compares the generated file against the registry the binary installs, so the next omission fails the build rather than shipping. Golden regenerated. Part (b) — the settings.json deny entry — was already corrected on 2026-07-24.', '', NULL, NULL, '2026-07-24T13:01:13Z', '2026-07-24T20:27:37Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (13, 'Harden the agent push barrier (this repo)', 'DONE', 'Closed on owner ruling 2026-07-24: the control is in place — git push denied at the permission layer (.claude/settings.json deny rules, e4aa352) and the this-repo-only, no-template-change decision recorded (dd0693a). No further hardening ordered.', '', NULL, NULL, '2026-07-24T13:26:03Z', '2026-07-24T21:32:34Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (14, 'Pre-release repository hygiene pass', 'DONE', 'Completed; detail in the private research archive', '', NULL, NULL, '2026-07-24T14:21:47Z', '2026-07-24T15:11:20Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (15, 'Full Windows support (post-v0): hooks, symlinks, sqlite locking, path separators', 'NEEDS-TRIAGE', 'v0 targets POSIX (spec 16); Windows CI verifies only dump byte-determinism. Full support is future work. First cross-platform CI run surfaced: pre-commit hook tests fail on Windows (generated hook is POSIX sh, run directly without a shell); symlink-containment tests need Windows symlink privileges; TestGateRaceOneWinner sees different sqlite file-locking semantics; TestScripts path-separator assumptions. Scope a Windows-portability pass if/when Windows becomes a target.', 'Post-v0; v0 explicitly targets POSIX per spec 16', NULL, NULL, '2026-07-24T16:39:08Z', '2026-07-24T16:39:08Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (16, 'Argument-shape guard for dump-supplied tokens reaching git argv (verify/import-dates)', 'DONE', 'Fixed: R5 refuses a leading-dash citation before git cat-file; the import-dates git show sink was already shaShape-gated (internal/verb/import_dates.go:35, pre-existing). stale --since refuses a leading-dash revision. Red fixtures proven red without the guards.', '', NULL, NULL, '2026-07-24T20:05:41Z', '2026-07-24T20:22:36Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (17, 'git mv in paths move receives dictionary-supplied roots with no -- separator (CWE-88 shape)', 'DONE', 'Fixed: git mv now takes a -- separator, and paths move validates the new root like paths set does (one column, one validation). Covered by dict_root_test.go plus the existing git-transport case in paths-move.txtar.', '', NULL, NULL, '2026-07-24T20:05:41Z', '2026-07-24T20:22:36Z');
INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at) VALUES (18, 'epic show omits the linked-artifacts section (ADRs and evidence are invisible to a reader)', 'NEEDS-TRIAGE', 'Both show verbs omit linked artifacts, confirmed empirically 2026-07-24: epic show --json keys are criteria/epic/goal/status/stories/worklog, and task show --json keys are epic/note/parked/ref/status/title — neither carries artifacts, and neither prints them in text. A linked ADR or research doc is reachable only via ''log <ref>'' events or by browsing the tracked directories, so R3''s on-disk guarantee has no reader. Found while checking whether a fresh agent would discover ADR 0001 (linked to v0-bootstrap as adr, and to this task as grounding — neither link shows).', '', NULL, NULL, '2026-07-24T20:44:34Z', '2026-07-24T20:45:01Z');
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
