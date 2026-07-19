# S1b opening record

Stage: S1b — schema gates (plan §4). Opened 2026-07-19, per D-EP13.
Spec revision at open: 3.11. Plan revision at open: 12.
Rows owned at open: 89. After the open: 86 at S1b — three moved (below).

Every row was read against the spec section its anchor names (content) and
against S1b's scope and DoD text (placement). S1b's machinery is the schema
package and raw driver connections; it has no verbs, no serializer, no CLI,
no git integration — a row needing any of those cannot be executed here.

**Resolved verification command.** Unless a row's line says otherwise, the
fixture named in its Verification cell resolves to
`go test ./internal/schema -run 'TestGates/<fixture>'` — a table-driven
red-fixture suite (each subtest performs the violating write and asserts
SQLite itself rejects it with the expected error). Fixture slugs are the
inventory's, kept verbatim so row ↔ subtest matching is mechanical.

## Rows moved (placement defects)

| Row | To | Why |
|---|---|---|
| INV-018 | S7 | The cluster's distinctive obligation — "done is provable by a commit range a fresh agent can `git show`" — is verify's R5 (commits resolve via `git cat-file`) plus R6 (DONE story ⇔ DONE worklog row with commits), both git-bound and executable only where `verify` exists. The append-only half is already owned row-per-trigger by INV-144/145 at S1b. |
| INV-097 | S5b | "`rel add` refuses cycles" needs the `rel` verb, built at S5b. A verify-rule row was sitting on a schema stage; the S1a close moved this exact class. |
| INV-143 | S5a | "The events trail retains superseded IN-REVIEW verdicts" needs `set-status` writing events rows — the S5a write-verb pipeline. Schema alone cannot produce the trail. |

## Escalated to the owner (rows kept, blocked)

- **INV-016** — "Epic: a goal decomposing into ≥2 stories…". The ≥2
  cardinality is enforced nowhere in the specification: no CHECK, no
  trigger, `epic close` (§6.4) checks terminality/criteria/homed-tasks but
  never story count, and no R-rule counts stories. As written, no stage can
  execute this row's cardinality clause. The fork is the owner's: rule it
  definitional (row narrows to the schema-carried parts) or add enforcement
  (a spec amendment naming the mechanism). Row stays at S1b, `planned`,
  blocked on that ruling; every other clause of the row is covered by
  dedicated rows (INV-068 goal, §5.4 criteria rows, §5.7 worklog rows,
  INV-070/163 close stamp).

## Spec defect found (amendment filed, not applied)

- §5.7's `worklog.story` comment says "guarded by R5"; the rule that guards
  story membership is **R4** (§7) — R5 resolves commits. Stale internal
  pointer. Proposal: `openspec/changes/worklog-story-guard-rule-pointer/`,
  awaiting owner review; the spec text is untouched until ratified.
  INV-109's statement inherits the pointer and is corrected when the
  amendment lands; its verdict below is written against the R4 reading.

## Systemic finding (parked, owner decision)

Row anchors carry spec line numbers extracted at rev 3.9; amendments 3.10
and 3.11 shifted the file, so those numbers now point a few lines off
(e.g. INV-052 cites `preamble:250`, the STRICT sentence now sits at 255).
Section names remain correct throughout — content matching below used
sections, not line offsets. The fork — re-stamp all line numbers after
every spec amendment, or strip them and anchor by section only — is an
inventory-format question (plan §3) and is parked in the ledger.

## Per-row verdicts

Content `ok` = the statement faithfully renders the current spec text at
its anchor. Placement `ok` = executable by S1b's machinery. Notes carry
anything a close reviewer must not have to rediscover.

| Row | Content | Placement | Verification (resolved) |
|---|---|---|---|
| INV-005 | ok — "Unknown" is embodied by NEEDS-TRIAGE ("unknown is data", §5.5), not a literal status value | ok | status-vocabulary-closed-and-needs-triage-legal: enum rejection is INV-069/080/102/107; this fixture asserts NEEDS-TRIAGE inserts cleanly |
| INV-009 | ok | ok | raw-connection-rejects-check-strict-notnull-wip: a second, pragma-free connection (FKs off) still has CHECK/STRICT/NOT NULL/WIP-index violations rejected |
| INV-015 | ok | ok | tasks-single-status-home: introspection — one status column in tasks, no other table stores task status |
| INV-016 | defect — ≥2-stories clause unenforced anywhere in the spec | blocked | escalated (see above); no fixture until the owner rules |
| INV-030 | ok | ok with note — journal posture and crash recovery are behaviour, not a gate, but S1b's machinery (Open + a kill harness) executes it and no other open stage can without a plan §4 scope amendment; close review re-judges | no-wal-shm-and-hot-journal-recovery: extends S1a's TestJournalModeIsNotWAL; adds kill-mid-write reopen probe (subprocess harness, temp dir) |
| INV-032 | ok | ok — largely proven by S1a's TestFreshDatabaseCarriesItsIdentityAndVersion | `go test ./internal/schema -run 'TestFreshDatabaseCarriesItsIdentityAndVersion|TestSeededMetaRowsArePresent'` plus meta-mirror equality assertion |
| INV-052 | ok | ok | strict-rejects-wrong-type-per-table: per-table loop; S1a's TestStrictTablesRejectAWrongTypedValue covers one table, this covers all |
| INV-057 | ok | ok | meta-duplicate-key-rejected |
| INV-062 | ok | ok | path_dictionary-duplicate-class-scope-rejected |
| INV-063 | ok | ok | path_dictionary-ephemeral-out-of-range-rejected |
| INV-067 | ok | ok | epics-duplicate-slug-rejected |
| INV-068 | ok | ok | epics-empty-goal-rejected |
| INV-069 | ok | ok | epics-status-enum-rejects-unknown-value |
| INV-070 | ok | ok | epics-close_sweep-status-mismatch-rejected (both directions: sweep without CLOSED, CLOSED without sweep) |
| INV-071 | ok | ok | epics-paused-dissolved-without-note-rejected |
| INV-072 | ok | ok | epic_criteria-orphan-epic-rejected |
| INV-073 | ok | ok | epic_criteria-duplicate-epic-seq-rejected |
| INV-074 | ok | ok | epic_criteria-empty-criterion-rejected |
| INV-075 | ok | ok | epic_criteria-met-out-of-range-rejected |
| INV-076 | ok | ok | epic_criteria-met-without-evidence-rejected |
| INV-078 | ok | ok | tasks-id-never-reused: AUTOINCREMENT in sqlite_master + sqlite_sequence row survives a rolled-back higher insert; direct delete is refused (INV-148), so the reuse path SQLite documents is closed both ways |
| INV-079 | ok | ok | tasks-empty-title-rejected |
| INV-080 | ok | ok | tasks-status-enum-rejects-unknown-value |
| INV-081 | ok | ok | tasks-duplicate-status-dup_of-mismatch-rejected (both directions) |
| INV-082 | ok | ok | tasks-done-wontdo-without-note-rejected |
| INV-083 | ok | ok | tasks-self-referential-dup_of-rejected |
| INV-084 | ok | ok | tasks-parked-on-illegal-status-rejected |
| INV-085 | ok | ok | tasks-dup_of-orphan-rejected |
| INV-086 | ok | ok | tasks-orphan-epic-rejected |
| INV-090 | ok | ok | task_links-duplicate-triple-rejected |
| INV-091 | ok | ok | task_links-self-link-rejected |
| INV-092 | ok | ok | task_links-type-enum-rejects-unknown-value |
| INV-093 | ok | ok | task_links-orphan-from_task-rejected |
| INV-094 | ok | ok | task_links-orphan-to_task-rejected |
| INV-097 | ok | moved → S5b | carried with the row |
| INV-099 | ok | ok | stories-orphan-epic-rejected |
| INV-100 | ok | ok | stories-id-format-rejects-non-matching |
| INV-101 | ok | ok | stories-empty-title-rejected |
| INV-102 | ok | ok | stories-status-enum-rejects-unknown-value |
| INV-103 | ok | ok | stories-in-progress-with-blocked-text-rejected |
| INV-104 | ok | ok | stories-duplicate-epic-id-rejected |
| INV-105 | ok | ok | stories-second-in-progress-same-epic-rejected |
| INV-106 | ok | ok | worklog-orphan-epic-rejected |
| INV-107 | ok | ok | worklog-state-enum-rejects-unknown-value |
| INV-108 | ok | ok | worklog-duplicate-epic-seq-rejected (seq PK; contiguity is INV-157's trigger) |
| INV-109 | ok against the R4 reading (statement says R5 — follows the filed amendment) | ok | worklog-story-orphan-not-schema-rejected: the orphan insert SUCCEEDS (the limitation is the obligation); the compensating guard is owned by the R4 rows (INV-296 at S6, INV-297 at S7) |
| INV-120 | ok | ok | artifacts-archived-out-of-range-rejected |
| INV-121 | ok | ok | artifacts-orphan-class-scope-rejected (composite FK) |
| INV-122 | ok | ok | artifacts-duplicate-class-scope-relpath-rejected |
| INV-123 | ok | ok | artifacts-id-never-reused (same shape as INV-078) |
| INV-124 | ok | ok | task_artifacts-role-enum-rejects-unknown-value |
| INV-125 | ok | ok | task_artifacts-duplicate-task-artifact-rejected |
| INV-126 | ok | ok | task_artifacts-orphan-task-rejected |
| INV-127 | ok | ok | task_artifacts-orphan-artifact-rejected |
| INV-128 | ok | ok | epic_artifacts-role-enum-rejects-unknown-value |
| INV-129 | ok | ok | epic_artifacts-duplicate-epic-artifact-rejected |
| INV-130 | ok | ok | epic_artifacts-orphan-epic-rejected |
| INV-131 | ok | ok | epic_artifacts-orphan-artifact-rejected |
| INV-134 | ok | ok | events-seq-never-reused (same shape as INV-078) |
| INV-135 | ok | ok | events-event-enum-rejects-unknown-value |
| INV-136 | ok | ok — index existence; "rides in every dump" is a serializer property proven where the serializer exists (S3's dump rows) | events_entity-index-present: sqlite_master assertion; already red-able via S1a's golden objects test on index drop |
| INV-143 | ok | moved → S5a | carried with the row |
| INV-144 | ok | ok | worklog-update-raises-abort |
| INV-145 | ok | ok | worklog-delete-raises-abort |
| INV-146 | ok — "same pair" comment, trigger implied by §5:491 | ok | events-update-raises-abort |
| INV-147 | ok — same | ok | events-delete-raises-abort |
| INV-148 | ok | ok | tasks-delete-raises-abort |
| INV-149 | ok | ok | epics-delete-raises-abort |
| INV-150 | ok | ok | stories-delete-raises-abort |
| INV-151 | ok | ok | epic_criteria-delete-raises-abort |
| INV-152 | ok | ok | artifacts-delete-raises-abort |
| INV-153 | ok | ok | task_links-delete-raises-abort |
| INV-154 | ok | ok | task_artifacts-delete-raises-abort |
| INV-155 | ok | ok | epic_artifacts-delete-raises-abort |
| INV-156 | ok | ok | path_dictionary-delete-raises-abort |
| INV-157 | ok | ok | worklog-insert-non-contiguous-seq-raises-abort (gap, repeat, zero, and cross-epic independence) |
| INV-158 | ok — matrix matches §5:508-514 edge for edge | ok | tasks-illegal-status-transition-raises-abort (full matrix sweep: every illegal pair) |
| INV-159 | ok | ok | tasks-inreview-exit-without-note-raises-abort |
| INV-160 | ok — the deliberate silence is the obligation | ok | illegal-transition-from-inreview-reports-transition-error-not-note-error: asserts the error TEXT names the transition, not the note |
| INV-161 | ok — matrix matches §5:530-538; DONE/DISSOLVED terminal | ok | stories-illegal-status-transition-raises-abort (full matrix sweep) |
| INV-162 | ok — matrix matches §5:540-547; CLOSED/DISSOLVED terminal | ok | epics-illegal-status-transition-raises-abort (full matrix sweep) |
| INV-163 | ok | ok | epics-close-fields-written-outside-epic-close-raises-abort: S1a's TestCloseStampIsAnAccidentGuardNotAnAdversaryGuard covers part; this adds the close_sweep-alone path |
| INV-165 | ok | ok — view queryable with raw inserts | v_ready-excludes-blocked-parked-non-open-tasks: matrix of parked/non-OPEN/unresolved-depends rows, plus the resolved-depends admit path |
| INV-166 | ok | ok | v_backlog-excludes-label-tasks (and projects the seven named columns) |
| INV-167 | ok | ok | raw-sql-write-violating-any-CHECK-rejected: same raw-connection harness as INV-009, iterating every CHECK named by the enforcement map |
| INV-171 | ok | ok | raw-sql-second-in-progress-story-same-epic-rejected (raw connection, FKs off) |
| INV-174 | ok | ok with note — locking behaviour, not a gate; kept for the same reason as INV-030, and the enforcement map is S1b's DoD checklist | second-writer-connection-blocked-by-exclusive-lock: extends S1a's TestWriteConnectionsLockExclusivelyAndSyncFully with a two-connection BUSY probe |
| INV-543 | ok | ok | two-home-links-one-task-both-succeed: the green twin of the role-vocabulary gates — asserts the ABSENCE of a uniqueness constraint on role='home' |

## What this open did not do

No code was written; no fixture exists yet — every command above is a name
the implementation must now make real, red first. Content was matched
against sections, not the drifted line anchors. The close review (item vi)
should sample these verdicts against the spec, not trust them: an `ok`
here is one reader's claim, recorded so it can be checked.
