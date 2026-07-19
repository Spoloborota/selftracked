// S1b: every gate the schema declares, shown failing. Each subtest name is
// the fixture slug of exactly one inventory row (docs/stage-openings/s1b.md
// maps row to slug), so `go test ./internal/schema -run 'TestGates/<slug>'`
// is the row's verification command. A red fixture performs the violating
// write and asserts SQLite itself refuses it; the handful of green twins
// (deliberate absences the spec states) assert the write succeeds.
package schema_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Spoloborota/selftracked/internal/schema"
)

// exec1 runs a statement that must succeed.
func exec1(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("setup %q: %v", query, err)
	}
}

// refuse runs a statement that must fail, with the given substring in the
// error. An empty substring accepts any refusal (used where two gates race
// for the same write and either message proves the write did not land).
func refuse(t *testing.T, db *sql.DB, wantSub, query string, args ...any) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), query, args...)
	if err == nil {
		t.Fatalf("%q succeeded; want refusal containing %q", query, wantSub)
	}
	if wantSub != "" && !strings.Contains(err.Error(), wantSub) {
		t.Fatalf("%q refused with %q; want substring %q", query, err, wantSub)
	}
}

// Seed helpers: minimal parents so FK-bearing rows can exist. INSERT paths
// are deliberately open (§1.1), which is what lets fixtures place rows in
// arbitrary starting states without verbs.

func addEpic(t *testing.T, db *sql.DB, slug, status string) {
	t.Helper()
	note := ""
	if status == "PAUSED" || status == "DISSOLVED" {
		note = "why"
	}
	sweep := ""
	if status == "CLOSED" {
		sweep = "2026-07-19"
	}
	exec1(t, db, `INSERT INTO epics (slug, goal, status, status_note, close_sweep, created_at)
	              VALUES (?, 'goal', ?, ?, ?, 'd')`, slug, status, note, sweep)
}

func addTask(t *testing.T, db *sql.DB, status string, dupOf any) int64 {
	t.Helper()
	note := ""
	if status == "DONE" || status == "WONT-DO" {
		note = "note"
	}
	var id int64
	err := db.QueryRowContext(t.Context(), `INSERT INTO tasks (title, status, status_note, dup_of, created_at, updated_at)
	                    VALUES ('t', ?, ?, ?, 'd', 'd') RETURNING id`, status, note, dupOf).Scan(&id)
	if err != nil {
		t.Fatalf("seed task %s: %v", status, err)
	}
	return id
}

func addStory(t *testing.T, db *sql.DB, status string) {
	t.Helper()
	blocked := ""
	if status == "BLOCKED" {
		blocked = "reason"
	}
	exec1(t, db, `INSERT INTO stories (epic, id, title, status, blocked) VALUES (?, ?, 't', ?, ?)`,
		"e", "S1", status, blocked)
}

func addArtifact(t *testing.T, db *sql.DB, relpath string) {
	t.Helper()
	exec1(t, db, `INSERT INTO artifacts (class, scope, relpath) VALUES ('research', '', ?)`, relpath)
}

func seedPathDict(t *testing.T, db *sql.DB) {
	t.Helper()
	exec1(t, db, `INSERT INTO path_dictionary (class, scope, root) VALUES ('research', '', 'docs/research')`)
}

// gateCase is one red (or deliberately green) fixture.
type gateCase struct {
	name    string
	setup   func(t *testing.T, db *sql.DB)
	stmt    string
	args    []any
	wantSub string // "" with green=false accepts any refusal
	green   bool   // the write must SUCCEED — a stated absence, not a gate
}

func TestGates(t *testing.T) {
	t.Parallel()
	cases := []gateCase{
		// --- meta / path_dictionary ---
		{
			name: "meta-duplicate-key-rejected",
			stmt: `INSERT INTO meta (key, value) VALUES ('schema_version', '9')`, wantSub: "UNIQUE",
		},
		{
			name:    "path_dictionary-duplicate-class-scope-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); seedPathDict(t, db) },
			stmt:    `INSERT INTO path_dictionary (class, scope, root) VALUES ('research', '', 'elsewhere')`,
			wantSub: "UNIQUE",
		},
		{
			name:    "path_dictionary-ephemeral-out-of-range-rejected",
			stmt:    `INSERT INTO path_dictionary (class, scope, root, ephemeral) VALUES ('run', '', 'work/runs', 2)`,
			wantSub: "CHECK",
		},

		// --- epics ---
		{
			name:    "epics-duplicate-slug-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "BACKLOG") },
			stmt:    `INSERT INTO epics (slug, goal, created_at) VALUES ('e', 'goal2', 'd')`,
			wantSub: "UNIQUE",
		},
		{
			name: "epics-empty-goal-rejected",
			stmt: `INSERT INTO epics (slug, goal, created_at) VALUES ('e', '', 'd')`, wantSub: "CHECK",
		},
		{
			name: "epics-status-enum-rejects-unknown-value",
			stmt: `INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'SHIPPED', 'd')`, wantSub: "CHECK",
		},
		{
			name: "epics-close_sweep-status-mismatch-rejected",
			// Both directions of the biconditional: a sweep without CLOSED,
			// and CLOSED without a sweep (the INSERT path, where the
			// close-gate trigger does not sit — the CHECK alone must hold).
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				refuse(t, db, "CHECK", `INSERT INTO epics (slug, goal, status, close_sweep, created_at)
				                        VALUES ('e1', 'g', 'BACKLOG', '2026-07-19', 'd')`)
			},
			stmt:    `INSERT INTO epics (slug, goal, status, close_sweep, created_at) VALUES ('e2', 'g', 'CLOSED', '', 'd')`,
			wantSub: "CHECK",
		},
		{
			name: "epics-paused-dissolved-without-note-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				refuse(t, db, "CHECK", `INSERT INTO epics (slug, goal, status, created_at) VALUES ('e1', 'g', 'DISSOLVED', 'd')`)
			},
			stmt:    `INSERT INTO epics (slug, goal, status, created_at) VALUES ('e2', 'g', 'PAUSED', 'd')`,
			wantSub: "CHECK",
		},

		// --- epic_criteria ---
		{
			name:    "epic_criteria-orphan-epic-rejected",
			stmt:    `INSERT INTO epic_criteria (epic, seq, criterion) VALUES ('ghost', 1, '$ true')`,
			wantSub: "FOREIGN KEY",
		},
		{
			name: "epic_criteria-duplicate-epic-seq-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addEpic(t, db, "e", "BACKLOG")
				exec1(t, db, `INSERT INTO epic_criteria (epic, seq, criterion) VALUES ('e', 1, 'c1')`)
			},
			stmt:    `INSERT INTO epic_criteria (epic, seq, criterion) VALUES ('e', 1, 'c2')`,
			wantSub: "UNIQUE",
		},
		{
			name:    "epic_criteria-empty-criterion-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "BACKLOG") },
			stmt:    `INSERT INTO epic_criteria (epic, seq, criterion) VALUES ('e', 1, '')`,
			wantSub: "CHECK",
		},
		{
			name:    "epic_criteria-met-out-of-range-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "BACKLOG") },
			stmt:    `INSERT INTO epic_criteria (epic, seq, criterion, met) VALUES ('e', 1, 'c', 2)`,
			wantSub: "CHECK",
		},
		{
			name:    "epic_criteria-met-without-evidence-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "BACKLOG") },
			stmt:    `INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('e', 1, 'c', 1, '')`,
			wantSub: "CHECK",
		},

		// --- tasks ---
		{
			name: "tasks-empty-title-rejected",
			stmt: `INSERT INTO tasks (title, created_at, updated_at) VALUES ('', 'd', 'd')`, wantSub: "CHECK",
		},
		{
			name:    "tasks-status-enum-rejects-unknown-value",
			stmt:    `INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t', 'SHIPPED', 'd', 'd')`,
			wantSub: "CHECK",
		},
		{
			name: "tasks-duplicate-status-dup_of-mismatch-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				id := addTask(t, db, "OPEN", nil)
				// direction 1: dup_of set while status is not DUPLICATE
				refuse(t, db, "CHECK", `INSERT INTO tasks (title, status, dup_of, created_at, updated_at)
				                        VALUES ('t', 'OPEN', ?, 'd', 'd')`, id)
			},
			// direction 2: DUPLICATE without dup_of
			stmt:    `INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t', 'DUPLICATE', 'd', 'd')`,
			wantSub: "CHECK",
		},
		{
			name: "tasks-done-wontdo-without-note-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				refuse(t, db, "CHECK",
					`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t', 'WONT-DO', 'd', 'd')`)
			},
			stmt:    `INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t', 'DONE', 'd', 'd')`,
			wantSub: "CHECK",
		},
		{
			name: "tasks-self-referential-dup_of-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				id := addTask(t, db, "OPEN", nil)
				refuse(t, db, "CHECK", `UPDATE tasks SET status='DUPLICATE', dup_of=? WHERE id=?`, id, id)
			},
			// The INSERT path cannot know its own id in advance, so the
			// self-reference is probed by number: id 1 in an empty table.
			stmt: `INSERT INTO tasks (id, title, status, dup_of, created_at, updated_at)
			       VALUES (7, 't', 'DUPLICATE', 7, 'd', 'd')`,
			wantSub: "CHECK",
		},
		{
			name: "tasks-parked-on-illegal-status-rejected",
			stmt: `INSERT INTO tasks (title, status, status_note, parked, created_at, updated_at)
			       VALUES ('t', 'DONE', 'n', 'later', 'd', 'd')`,
			wantSub: "CHECK",
		},
		{
			name: "tasks-dup_of-orphan-rejected",
			stmt: `INSERT INTO tasks (title, status, dup_of, created_at, updated_at)
			       VALUES ('t', 'DUPLICATE', 999, 'd', 'd')`,
			wantSub: "FOREIGN KEY",
		},
		{
			name:    "tasks-orphan-epic-rejected",
			stmt:    `INSERT INTO tasks (title, epic, created_at, updated_at) VALUES ('t', 'ghost', 'd', 'd')`,
			wantSub: "FOREIGN KEY",
		},

		// --- task_links ---
		{
			name: "task_links-duplicate-triple-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				a, b := addTask(t, db, "OPEN", nil), addTask(t, db, "OPEN", nil)
				exec1(t, db, `INSERT INTO task_links (from_task, to_task, type) VALUES (?, ?, 'depends')`, a, b)
			},
			stmt:    `INSERT INTO task_links (from_task, to_task, type) VALUES (1, 2, 'depends')`,
			wantSub: "UNIQUE",
		},
		{
			name:    "task_links-self-link-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addTask(t, db, "OPEN", nil) },
			stmt:    `INSERT INTO task_links (from_task, to_task, type) VALUES (1, 1, 'relates')`,
			wantSub: "CHECK",
		},
		{
			name: "task_links-type-enum-rejects-unknown-value",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addTask(t, db, "OPEN", nil)
				addTask(t, db, "OPEN", nil)
			},
			stmt:    `INSERT INTO task_links (from_task, to_task, type) VALUES (1, 2, 'blocks')`,
			wantSub: "CHECK",
		},
		{
			name:    "task_links-orphan-from_task-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addTask(t, db, "OPEN", nil) },
			stmt:    `INSERT INTO task_links (from_task, to_task, type) VALUES (999, 1, 'depends')`,
			wantSub: "FOREIGN KEY",
		},
		{
			name:    "task_links-orphan-to_task-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addTask(t, db, "OPEN", nil) },
			stmt:    `INSERT INTO task_links (from_task, to_task, type) VALUES (1, 999, 'depends')`,
			wantSub: "FOREIGN KEY",
		},

		// --- stories ---
		{
			name: "stories-orphan-epic-rejected",
			stmt: `INSERT INTO stories (epic, id, title) VALUES ('ghost', 'S1', 't')`, wantSub: "FOREIGN KEY",
		},
		{
			name:    "stories-id-format-rejects-non-matching",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "BACKLOG") },
			stmt:    `INSERT INTO stories (epic, id, title) VALUES ('e', 'X1', 't')`,
			wantSub: "CHECK",
		},
		{
			name:    "stories-empty-title-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "BACKLOG") },
			stmt:    `INSERT INTO stories (epic, id, title) VALUES ('e', 'S1', '')`,
			wantSub: "CHECK",
		},
		{
			name:    "stories-status-enum-rejects-unknown-value",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "BACKLOG") },
			stmt:    `INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S1', 't', 'SHIPPED')`,
			wantSub: "CHECK",
		},
		{
			name:    "stories-in-progress-with-blocked-text-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "BACKLOG") },
			stmt:    `INSERT INTO stories (epic, id, title, status, blocked) VALUES ('e', 'S1', 't', 'IN-PROGRESS', 'stuck')`,
			wantSub: "CHECK",
		},
		{
			name: "stories-duplicate-epic-id-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addEpic(t, db, "e", "BACKLOG")
				addStory(t, db, "PLANNED")
			},
			stmt:    `INSERT INTO stories (epic, id, title) VALUES ('e', 'S1', 'again')`,
			wantSub: "UNIQUE",
		},
		{
			name: "stories-second-in-progress-same-epic-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addEpic(t, db, "e", "ACTIVE")
				addStory(t, db, "IN-PROGRESS")
			},
			stmt:    `INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S2', 't', 'IN-PROGRESS')`,
			wantSub: "UNIQUE",
		},

		// --- worklog ---
		{
			name: "worklog-orphan-epic-rejected",
			stmt: `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('ghost', 1, 'S1', 'd', 'DONE')`,
			// The contiguity trigger runs BEFORE INSERT and its subquery on an
			// absent epic yields seq 1, so the FK is what refuses this row.
			wantSub: "FOREIGN KEY",
		},
		{
			name:    "worklog-state-enum-rejects-unknown-value",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "ACTIVE") },
			stmt:    `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S1', 'd', 'SHIPPED')`,
			wantSub: "CHECK",
		},
		{
			name: "worklog-duplicate-epic-seq-rejected",
			// A duplicate seq is also non-contiguous, and the BEFORE INSERT
			// trigger runs before the PRIMARY KEY is checked, so the trigger
			// message is the deterministic refusal here; the PK itself is
			// asserted by introspection in TestWorklogPrimaryKeyColumns.
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addEpic(t, db, "e", "ACTIVE")
				exec1(t, db, `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S1', 'd', 'IN-PROGRESS')`)
			},
			stmt:    `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S1', 'd', 'DONE')`,
			wantSub: "contiguous",
		},
		{
			name: "worklog-story-orphan-not-schema-rejected-caught-by-R4",
			// GREEN by design: the FK-free column is a stated limitation
			// (INV-109); the compensating check is R4, owned by its own rows.
			setup: func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "ACTIVE") },
			stmt:  `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S99', 'd', 'IN-PROGRESS')`,
			green: true,
		},

		// --- artifacts & links ---
		{
			name:    "artifacts-archived-out-of-range-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); seedPathDict(t, db) },
			stmt:    `INSERT INTO artifacts (class, scope, relpath, archived) VALUES ('research', '', 'x.md', 2)`,
			wantSub: "CHECK",
		},
		{
			name:    "artifacts-orphan-class-scope-rejected",
			stmt:    `INSERT INTO artifacts (class, scope, relpath) VALUES ('nosuch', '', 'x.md')`,
			wantSub: "FOREIGN KEY",
		},
		{
			name: "artifacts-duplicate-class-scope-relpath-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addArtifact(t, db, "x.md")
			},
			stmt:    `INSERT INTO artifacts (class, scope, relpath) VALUES ('research', '', 'x.md')`,
			wantSub: "UNIQUE",
		},
		{
			name: "task_artifacts-role-enum-rejects-unknown-value",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addTask(t, db, "OPEN", nil)
				addArtifact(t, db, "x.md")
			},
			stmt:    `INSERT INTO task_artifacts (task, artifact, role) VALUES (1, 1, 'muse')`,
			wantSub: "CHECK",
		},
		{
			name: "task_artifacts-duplicate-task-artifact-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addTask(t, db, "OPEN", nil)
				addArtifact(t, db, "x.md")
				exec1(t, db, `INSERT INTO task_artifacts (task, artifact, role) VALUES (1, 1, 'home')`)
			},
			stmt:    `INSERT INTO task_artifacts (task, artifact, role) VALUES (1, 1, 'evidence')`,
			wantSub: "UNIQUE",
		},
		{
			name: "task_artifacts-orphan-task-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addArtifact(t, db, "x.md")
			},
			stmt:    `INSERT INTO task_artifacts (task, artifact, role) VALUES (999, 1, 'home')`,
			wantSub: "FOREIGN KEY",
		},
		{
			name:    "task_artifacts-orphan-artifact-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addTask(t, db, "OPEN", nil) },
			stmt:    `INSERT INTO task_artifacts (task, artifact, role) VALUES (1, 999, 'home')`,
			wantSub: "FOREIGN KEY",
		},
		{
			name: "epic_artifacts-role-enum-rejects-unknown-value",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addEpic(t, db, "e", "BACKLOG")
				addArtifact(t, db, "x.md")
			},
			stmt:    `INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('e', 1, 'muse')`,
			wantSub: "CHECK",
		},
		{
			name: "epic_artifacts-duplicate-epic-artifact-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addEpic(t, db, "e", "BACKLOG")
				addArtifact(t, db, "x.md")
				exec1(t, db, `INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('e', 1, 'home')`)
			},
			stmt:    `INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('e', 1, 'evidence')`,
			wantSub: "UNIQUE",
		},
		{
			name: "epic_artifacts-orphan-epic-rejected",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addArtifact(t, db, "x.md")
			},
			stmt:    `INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('ghost', 1, 'home')`,
			wantSub: "FOREIGN KEY",
		},
		{
			name:    "epic_artifacts-orphan-artifact-rejected",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "BACKLOG") },
			stmt:    `INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('e', 999, 'home')`,
			wantSub: "FOREIGN KEY",
		},
		{
			name: "two-home-links-one-task-both-succeed",
			// GREEN by design (INV-543): several narrative homes are legal.
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addTask(t, db, "OPEN", nil)
				addArtifact(t, db, "a.md")
				addArtifact(t, db, "b.md")
				exec1(t, db, `INSERT INTO task_artifacts (task, artifact, role) VALUES (1, 1, 'home')`)
			},
			stmt:  `INSERT INTO task_artifacts (task, artifact, role) VALUES (1, 2, 'home')`,
			green: true,
		},

		// --- events ---
		{
			name:    "events-event-enum-rejects-unknown-value",
			stmt:    `INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'destroy')`,
			wantSub: "CHECK",
		},

		// --- append-only / no-delete triggers ---
		{
			name: "worklog-update-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addEpic(t, db, "e", "ACTIVE")
				exec1(t, db, `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S1', 'd', 'IN-PROGRESS')`)
			},
			stmt:    `UPDATE worklog SET note = 'edited' WHERE epic = 'e' AND seq = 1`,
			wantSub: "worklog is append-only",
		},
		{
			name: "worklog-delete-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addEpic(t, db, "e", "ACTIVE")
				exec1(t, db, `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S1', 'd', 'IN-PROGRESS')`)
			},
			stmt:    `DELETE FROM worklog WHERE epic = 'e'`,
			wantSub: "worklog is append-only",
		},
		{
			name: "events-update-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				exec1(t, db, `INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`)
			},
			stmt:    `UPDATE events SET detail = 'edited' WHERE seq = 1`,
			wantSub: "events is append-only",
		},
		{
			name: "events-delete-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				exec1(t, db, `INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`)
			},
			stmt:    `DELETE FROM events`,
			wantSub: "events is append-only",
		},
		{
			name:    "tasks-delete-raises-abort",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addTask(t, db, "OPEN", nil) },
			stmt:    `DELETE FROM tasks`,
			wantSub: "never deleted",
		},
		{
			name:    "epics-delete-raises-abort",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addEpic(t, db, "e", "BACKLOG") },
			stmt:    `DELETE FROM epics`,
			wantSub: "never deleted",
		},
		{
			name: "stories-delete-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addEpic(t, db, "e", "BACKLOG")
				addStory(t, db, "PLANNED")
			},
			stmt:    `DELETE FROM stories`,
			wantSub: "never deleted",
		},
		{
			name: "epic_criteria-delete-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addEpic(t, db, "e", "BACKLOG")
				exec1(t, db, `INSERT INTO epic_criteria (epic, seq, criterion) VALUES ('e', 1, 'c')`)
			},
			stmt:    `DELETE FROM epic_criteria`,
			wantSub: "never deleted",
		},
		{
			name: "artifacts-delete-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addArtifact(t, db, "x.md")
			},
			stmt:    `DELETE FROM artifacts`,
			wantSub: "never deleted",
		},
		{
			name: "task_links-delete-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				a, b := addTask(t, db, "OPEN", nil), addTask(t, db, "OPEN", nil)
				exec1(t, db, `INSERT INTO task_links (from_task, to_task, type) VALUES (?, ?, 'relates')`, a, b)
			},
			stmt:    `DELETE FROM task_links`,
			wantSub: "never deleted",
		},
		{
			name: "task_artifacts-delete-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addTask(t, db, "OPEN", nil)
				addArtifact(t, db, "x.md")
				exec1(t, db, `INSERT INTO task_artifacts (task, artifact, role) VALUES (1, 1, 'home')`)
			},
			stmt:    `DELETE FROM task_artifacts`,
			wantSub: "never deleted",
		},
		{
			name: "epic_artifacts-delete-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				seedPathDict(t, db)
				addEpic(t, db, "e", "BACKLOG")
				addArtifact(t, db, "x.md")
				exec1(t, db, `INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('e', 1, 'home')`)
			},
			stmt:    `DELETE FROM epic_artifacts`,
			wantSub: "never deleted",
		},
		{
			name:    "path_dictionary-delete-raises-abort",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); seedPathDict(t, db) },
			stmt:    `DELETE FROM path_dictionary`,
			wantSub: "never deleted",
		},

		// --- worklog seq contiguity ---
		{
			name: "worklog-insert-non-contiguous-seq-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addEpic(t, db, "e", "ACTIVE")
				addEpic(t, db, "e2", "ACTIVE")
				exec1(t, db, `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S1', 'd', 'IN-PROGRESS')`)
				// a gap
				refuse(t, db, "contiguous",
					`INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 3, 'S1', 'd', 'DONE')`)
				// zero on a fresh epic
				refuse(t, db, "contiguous",
					`INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e2', 0, 'S1', 'd', 'IN-PROGRESS')`)
				// contiguity is per epic: e2 starts at 1 regardless of e
				exec1(t, db, `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e2', 1, 'S1', 'd', 'IN-PROGRESS')`)
			},
			stmt:    `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S1', 'd', 'DONE')`,
			wantSub: "contiguous",
		},

		// --- the IN-REVIEW exit note ---
		{
			name:    "tasks-inreview-exit-without-note-raises-abort",
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addTask(t, db, "IN-REVIEW", nil) },
			stmt:    `UPDATE tasks SET status = 'OPEN', status_note = '' WHERE id = 1`,
			wantSub: "requires the owner verdict",
		},
		{
			name: "illegal-transition-from-inreview-reports-transition-error-not-note-error",
			// INV-160: the note trigger's legal-target clause must stay
			// silent on a matrix-illegal move, so the transition trigger
			// reports it whatever SQLite's (undocumented) firing order is.
			setup:   func(t *testing.T, db *sql.DB) { t.Helper(); addTask(t, db, "IN-REVIEW", nil) },
			stmt:    `UPDATE tasks SET status = 'LABEL', status_note = '' WHERE id = 1`,
			wantSub: "illegal status transition",
		},

		// --- the epic close gate ---
		{
			name: "epics-close-fields-written-outside-epic-close-raises-abort",
			setup: func(t *testing.T, db *sql.DB) {
				t.Helper()
				addEpic(t, db, "e", "ACTIVE")
				// close_sweep alone, status untouched: still gated
				refuse(t, db, "written only by epic close",
					`UPDATE epics SET close_sweep = '2026-07-19' WHERE slug = 'e'`)
				// the green half: with active_verb set the same write lands
				exec1(t, db, `INSERT INTO meta (key, value) VALUES ('active_verb', 'epic-close')`)
				exec1(t, db, `UPDATE epics SET status = 'CLOSED', close_sweep = '2026-07-19' WHERE slug = 'e'`)
				exec1(t, db, `DELETE FROM meta WHERE key = 'active_verb'`)
				addEpic(t, db, "e2", "ACTIVE")
			},
			stmt:    `UPDATE epics SET status = 'CLOSED', close_sweep = '2026-07-19' WHERE slug = 'e2'`,
			wantSub: "written only by epic close",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := fresh(t)
			if tc.setup != nil {
				tc.setup(t, db)
			}
			if tc.green {
				exec1(t, db, tc.stmt, tc.args...)
				return
			}
			refuse(t, db, tc.wantSub, tc.stmt, tc.args...)
		})
	}
}

// --- transition matrices, swept edge for edge ---

// sweepTransitions drives every ordered status pair through an UPDATE and
// asserts the trigger's verdict matches the matrix. seed places a row of the
// from-status and returns the UPDATE statement (with the target and its
// companion columns already bound), so CHECK constraints never mask the
// transition verdict under test.
func sweepTransitions(t *testing.T, statuses []string, legal map[string][]string,
	wantSub string, run func(t *testing.T, db *sql.DB, from, to string) error,
) {
	t.Helper()
	isLegal := func(from, to string) bool {
		return from == to || slices.Contains(legal[from], to)
	}
	for _, from := range statuses {
		for _, to := range statuses {
			db := fresh(t)
			err := run(t, db, from, to)
			switch {
			case isLegal(from, to) && err != nil:
				t.Errorf("%s -> %s: legal per the matrix, refused: %v", from, to, err)
			case !isLegal(from, to) && err == nil:
				t.Errorf("%s -> %s: illegal per the matrix, accepted", from, to)
			case !isLegal(from, to) && !strings.Contains(err.Error(), wantSub):
				t.Errorf("%s -> %s: refused with %q; want substring %q", from, to, err, wantSub)
			}
		}
	}
}

func TestGatesTasksTransitionMatrix(t *testing.T) {
	t.Parallel()
	t.Run("tasks-illegal-status-transition-raises-abort", func(t *testing.T) {
		t.Parallel()
		statuses := []string{"OPEN", "IN-REVIEW", "NEEDS-TRIAGE", "DONE", "WONT-DO", "DUPLICATE", "LABEL"}
		legal := map[string][]string{
			"OPEN":         {"IN-REVIEW", "NEEDS-TRIAGE", "DONE", "WONT-DO", "DUPLICATE"},
			"IN-REVIEW":    {"OPEN", "NEEDS-TRIAGE", "DONE", "WONT-DO", "DUPLICATE"},
			"NEEDS-TRIAGE": {"OPEN", "IN-REVIEW", "DONE", "WONT-DO", "DUPLICATE"},
			"DONE":         {"OPEN"},
			"WONT-DO":      {"OPEN"},
			"DUPLICATE":    {"OPEN"},
		}
		sweepTransitions(t, statuses, legal, "illegal status transition",
			func(t *testing.T, db *sql.DB, from, to string) error {
				t.Helper()
				other := addTask(t, db, "OPEN", nil) // dup_of target
				var fromDup any
				if from == "DUPLICATE" {
					fromDup = other
				}
				id := addTask(t, db, from, fromDup)
				// Companion columns keep every CHECK satisfied so the
				// transition trigger alone decides: a note for terminal
				// targets and for any IN-REVIEW exit, dup_of exactly when
				// the target is DUPLICATE.
				note := "n"
				var dup any
				if to == "DUPLICATE" {
					dup = other
				}
				if _, err := db.ExecContext(t.Context(), `UPDATE tasks SET status=?, status_note=?, dup_of=? WHERE id=?`,
					to, note, dup, id); err != nil {
					return fmt.Errorf("transition update: %w", err)
				}
				return nil
			})
	})
}

func TestGatesStoriesTransitionMatrix(t *testing.T) {
	t.Parallel()
	t.Run("stories-illegal-status-transition-raises-abort", func(t *testing.T) {
		t.Parallel()
		statuses := []string{"PLANNED", "READY", "BLOCKED", "IN-PROGRESS", "DONE", "DISSOLVED"}
		legal := map[string][]string{
			"PLANNED":     {"READY", "BLOCKED", "DISSOLVED"},
			"READY":       {"IN-PROGRESS", "BLOCKED", "DISSOLVED"},
			"BLOCKED":     {"READY", "DISSOLVED"},
			"IN-PROGRESS": {"DONE", "BLOCKED", "DISSOLVED"},
		}
		sweepTransitions(t, statuses, legal, "illegal story transition",
			func(t *testing.T, db *sql.DB, from, to string) error {
				t.Helper()
				addEpic(t, db, "e", "ACTIVE")
				addStory(t, db, from)
				blocked := ""
				if to == "BLOCKED" {
					blocked = "reason"
				}
				if _, err := db.ExecContext(t.Context(), `UPDATE stories SET status=?, blocked=? WHERE epic='e' AND id='S1'`,
					to, blocked); err != nil {
					return fmt.Errorf("transition update: %w", err)
				}
				return nil
			})
	})
}

func TestGatesEpicsTransitionMatrix(t *testing.T) {
	t.Parallel()
	t.Run("epics-illegal-status-transition-raises-abort", func(t *testing.T) {
		t.Parallel()
		statuses := []string{"BACKLOG", "ACTIVE", "PAUSED", "CLOSED", "DISSOLVED"}
		legal := map[string][]string{
			"BACKLOG": {"ACTIVE", "DISSOLVED"},
			"ACTIVE":  {"PAUSED", "CLOSED", "DISSOLVED"},
			"PAUSED":  {"ACTIVE", "CLOSED", "DISSOLVED"},
		}
		sweepTransitions(t, statuses, legal, "illegal epic transition",
			func(t *testing.T, db *sql.DB, from, to string) error {
				t.Helper()
				addEpic(t, db, "e", from)
				// The close gate is a different trigger with its own fixture;
				// satisfy it here so the matrix verdict is what surfaces.
				exec1(t, db, `INSERT INTO meta (key, value) VALUES ('active_verb', 'epic-close')`)
				note := ""
				if to == "PAUSED" || to == "DISSOLVED" {
					note = "why"
				}
				sweep := ""
				if to == "CLOSED" || from == "CLOSED" && to == "CLOSED" {
					sweep = "2026-07-19"
				}
				if from == "CLOSED" && to != "CLOSED" {
					sweep = "" // leaving CLOSED must also drop the stamp to satisfy the CHECK
				}
				if from == "CLOSED" && to == "CLOSED" {
					sweep = "2026-07-19"
				}
				if _, err := db.ExecContext(t.Context(), `UPDATE epics SET status=?, status_note=?, close_sweep=? WHERE slug='e'`,
					to, note, sweep); err != nil {
					return fmt.Errorf("transition update: %w", err)
				}
				return nil
			})
	})
}

// --- vocabulary closure and the legal unknown (INV-005) ---

func TestGatesStatusVocabulary(t *testing.T) {
	t.Parallel()
	t.Run("status-vocabulary-closed-and-needs-triage-legal", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		// "Unknown" is a legal state, spelled NEEDS-TRIAGE, and it is the
		// default: a task inserted with no status lands there.
		exec1(t, db, `INSERT INTO tasks (title, created_at, updated_at) VALUES ('t', 'd', 'd')`)
		var got string
		if err := db.QueryRowContext(t.Context(), `SELECT status FROM tasks WHERE id = 1`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != "NEEDS-TRIAGE" {
			t.Fatalf("default status = %q; want NEEDS-TRIAGE", got)
		}
		// Closure of every status vocabulary is proven per table by the enum
		// fixtures in TestGates; this asserts the one home rule's flip side —
		// there is no second legal spelling of "unknown".
		refuse(t, db, "CHECK", `INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t', 'UNKNOWN', 'd', 'd')`)
	})
}

// --- single status home (INV-015) ---

func TestGatesTasksSingleStatusHome(t *testing.T) {
	t.Parallel()
	t.Run("tasks-single-status-home", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		rows, err := db.QueryContext(t.Context(), `SELECT name FROM pragma_table_info('tasks') WHERE name LIKE '%status%'`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		var statusCols []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				t.Fatal(err)
			}
			statusCols = append(statusCols, n)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		// status itself and its note; a second status-bearing column would
		// be a second home.
		want := []string{"status", "status_note"}
		if fmt.Sprint(statusCols) != fmt.Sprint(want) {
			t.Fatalf("status columns on tasks = %v; want exactly %v", statusCols, want)
		}
	})
}

// --- STRICT per table (INV-052) ---

func TestGatesStrictPerTable(t *testing.T) {
	t.Parallel()
	t.Run("strict-rejects-wrong-type-per-table", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		seedPathDict(t, db)
		addEpic(t, db, "e", "ACTIVE")
		addTask(t, db, "OPEN", nil)
		addTask(t, db, "OPEN", nil)
		addArtifact(t, db, "x.md")
		// One wrong-typed write per table: text into INTEGER, or a blob into
		// TEXT — STRICT refuses what a lax table would coerce.
		wrong := map[string]string{
			"meta":            `INSERT INTO meta (key, value) VALUES ('k', x'00')`,
			"path_dictionary": `INSERT INTO path_dictionary (class, scope, root, ephemeral) VALUES ('c', '', 'r', 'soon')`,
			"epics":           `INSERT INTO epics (slug, goal, created_at) VALUES ('e2', x'00', 'd')`,
			"epic_criteria":   `INSERT INTO epic_criteria (epic, seq, criterion) VALUES ('e', 'first', 'c')`,
			"tasks":           `INSERT INTO tasks (title, created_at, updated_at) VALUES (x'00', 'd', 'd')`,
			"task_links":      `INSERT INTO task_links (from_task, to_task, type) VALUES ('one', 2, 'depends')`,
			"stories":         `INSERT INTO stories (epic, id, title) VALUES ('e', 'S1', x'00')`,
			"worklog":         `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 'first', 'S1', 'd', 'DONE')`,
			"artifacts":       `INSERT INTO artifacts (class, scope, relpath, archived) VALUES ('research', '', 'y.md', 'yes')`,
			"task_artifacts":  `INSERT INTO task_artifacts (task, artifact, role) VALUES ('one', 1, 'home')`,
			"epic_artifacts":  `INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('e', 'one', 'home')`,
			"events":          `INSERT INTO events (at, entity, event, detail) VALUES ('d', '#1', 'create', x'00')`,
		}
		for table, stmt := range wrong {
			_, err := db.ExecContext(t.Context(), stmt)
			if err == nil {
				t.Errorf("%s: wrong-typed write accepted; STRICT should refuse", table)
				continue
			}
			// SQLite validates STRICT typing before trigger WHEN clauses
			// run, so the type error is the deterministic refusal — pin its
			// message so a different gate cannot pass for this one.
			if !strings.Contains(err.Error(), "cannot store") {
				t.Errorf("%s: refused with %q; want the STRICT type error", table, err)
			}
		}
	})
}

// --- AUTOINCREMENT: ids are never reused (INV-078, INV-123, INV-134) ---

// autoincrementNeverReuses asserts the two observable halves of the
// guarantee: the table opts into AUTOINCREMENT (sqlite_master carries the
// keyword — dropping it is the red path), and the high-water mark lives in
// sqlite_sequence at least as high as the maximum id. Deletion — the reuse
// vector AUTOINCREMENT exists to close — is itself refused by the no-delete
// triggers, so the two gates together close both reuse paths.
func autoincrementNeverReuses(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var ddl string
	err := db.QueryRowContext(t.Context(),
		`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&ddl)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ddl, "AUTOINCREMENT") {
		t.Fatalf("%s: AUTOINCREMENT missing from its declaration", table)
	}
	var seq, maxID int64
	err = db.QueryRowContext(t.Context(),
		`SELECT seq FROM sqlite_sequence WHERE name=?`, table).Scan(&seq)
	if err != nil {
		t.Fatalf("%s: no sqlite_sequence row after an insert: %v", table, err)
	}
	col := "id"
	if table == "events" {
		col = "seq"
	}
	if err := db.QueryRowContext(t.Context(), `SELECT COALESCE(MAX(`+col+`),0) FROM `+table).Scan(&maxID); err != nil {
		t.Fatal(err)
	}
	if seq < maxID {
		t.Fatalf("%s: sqlite_sequence %d below MAX(id) %d", table, seq, maxID)
	}
}

func TestGatesAutoincrement(t *testing.T) {
	t.Parallel()
	t.Run("tasks-id-never-reused", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		addTask(t, db, "OPEN", nil)
		autoincrementNeverReuses(t, db, "tasks")
	})
	t.Run("artifacts-id-never-reused", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		seedPathDict(t, db)
		addArtifact(t, db, "x.md")
		autoincrementNeverReuses(t, db, "artifacts")
	})
	t.Run("events-seq-never-reused", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		exec1(t, db, `INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`)
		autoincrementNeverReuses(t, db, "events")
	})
}

// --- the events_entity index (INV-136) ---

func TestGatesEventsEntityIndex(t *testing.T) {
	t.Parallel()
	t.Run("events_entity-index-present", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		var tbl string
		err := db.QueryRowContext(t.Context(),
			`SELECT tbl_name FROM sqlite_master WHERE type='index' AND name='events_entity'`).Scan(&tbl)
		if err != nil {
			t.Fatalf("events_entity index absent: %v", err)
		}
		if tbl != "events" {
			t.Fatalf("events_entity indexes %q; want events", tbl)
		}
	})
}

// --- worklog PK, asserted where the contiguity trigger masks it ---

func TestWorklogPrimaryKeyColumns(t *testing.T) {
	t.Parallel()
	db := fresh(t)
	rows, err := db.QueryContext(t.Context(), `SELECT name FROM pragma_table_info('worklog') WHERE pk > 0 ORDER BY pk`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var pk []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		pk = append(pk, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(pk) != fmt.Sprint([]string{"epic", "seq"}) {
		t.Fatalf("worklog PK = %v; want (epic, seq)", pk)
	}
}

// --- identity pragmas mirror the meta table (INV-032) ---

func TestGatesIdentityMirrorsMeta(t *testing.T) {
	t.Parallel()
	db := fresh(t)
	var userVersion int
	if err := db.QueryRowContext(t.Context(), `PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatal(err)
	}
	var metaVersion string
	err := db.QueryRowContext(t.Context(),
		`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&metaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if strconv.Itoa(userVersion) != metaVersion {
		t.Fatalf("user_version %d does not mirror meta schema_version %q", userVersion, metaVersion)
	}
}

// --- views (INV-165, INV-166) ---

func TestGatesViews(t *testing.T) {
	t.Parallel()
	t.Run("v_ready-excludes-blocked-parked-non-open-tasks", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		in := addTask(t, db, "OPEN", nil) // plain OPEN: ready
		parked := addTask(t, db, "OPEN", nil)
		exec1(t, db, `UPDATE tasks SET parked = 'later' WHERE id = ?`, parked)
		addTask(t, db, "NEEDS-TRIAGE", nil) // not OPEN: out
		blockedBy := addTask(t, db, "OPEN", nil)
		waiting := addTask(t, db, "OPEN", nil) // depends on an unresolved task: out
		exec1(t, db, `INSERT INTO task_links (from_task, to_task, type) VALUES (?, ?, 'depends')`, waiting, blockedBy)
		doneDep := addTask(t, db, "DONE", nil)
		freed := addTask(t, db, "OPEN", nil) // depends only on a terminal task: ready
		exec1(t, db, `INSERT INTO task_links (from_task, to_task, type) VALUES (?, ?, 'depends')`, freed, doneDep)

		rows, err := db.QueryContext(t.Context(), `SELECT id FROM v_ready ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		var got []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			got = append(got, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		// blockedBy is itself a plain OPEN task, so it is ready too.
		want := []int64{in, blockedBy, freed}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("v_ready = %v; want %v", got, want)
		}
	})
	t.Run("v_backlog-excludes-label-tasks", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		open := addTask(t, db, "OPEN", nil)
		addTask(t, db, "LABEL", nil)
		rows, err := db.QueryContext(t.Context(), `SELECT id, ref FROM v_backlog ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		var ids []int64
		var refs []string
		for rows.Next() {
			var id int64
			var ref string
			if err := rows.Scan(&id, &ref); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
			refs = append(refs, ref)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(ids) != fmt.Sprint([]int64{open}) || refs[0] != fmt.Sprintf("#%d", open) {
			t.Fatalf("v_backlog ids=%v refs=%v; want the one non-LABEL task as #id", ids, refs)
		}
		// The projection is the seven named columns, no more.
		cols, err := db.QueryContext(t.Context(), `SELECT name FROM pragma_table_info('v_backlog')`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = cols.Close() }()
		var names []string
		for cols.Next() {
			var n string
			if err := cols.Scan(&n); err != nil {
				t.Fatal(err)
			}
			names = append(names, n)
		}
		if err := cols.Err(); err != nil {
			t.Fatal(err)
		}
		want := []string{"id", "ref", "title", "status", "status_note", "parked", "epic"}
		if fmt.Sprint(names) != fmt.Sprint(want) {
			t.Fatalf("v_backlog columns = %v; want %v", names, want)
		}
	})
}

// --- schema-enforced integrity binds a pragma-free connection ---
// (INV-009, INV-167, INV-171: the enforcement map's "any process" claims)

// rawConn opens the same file the way a foreign process would: no DSN
// pragmas at all, foreign keys at SQLite's default (off).
func rawConn(t *testing.T, dir string) *sql.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return raw
}

// freshAt is fresh with the directory exposed, for tests that need a second
// connection to the same file.
func freshAt(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := schema.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := schema.Create(t.Context(), db); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return db, dir
}

func TestGatesBindAnyProcess(t *testing.T) {
	t.Parallel()
	t.Run("raw-connection-rejects-check-strict-notnull-wip", func(t *testing.T) {
		t.Parallel()
		db, dir := freshAt(t)
		addEpic(t, db, "e", "ACTIVE")
		addStory(t, db, "IN-PROGRESS")
		raw := rawConn(t, dir)
		// CHECK
		refuse(t, raw, "CHECK", `INSERT INTO tasks (title, created_at, updated_at) VALUES ('', 'd', 'd')`)
		// STRICT
		refuse(t, raw, "cannot store", `INSERT INTO tasks (title, created_at, updated_at) VALUES (x'00', 'd', 'd')`)
		// NOT NULL
		refuse(t, raw, "NOT NULL", `INSERT INTO meta (key, value) VALUES ('k', NULL)`)
		// the WIP partial unique index
		refuse(t, raw, "UNIQUE", `INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S2', 't', 'IN-PROGRESS')`)
	})
	t.Run("raw-sql-write-violating-any-CHECK-rejected", func(t *testing.T) {
		t.Parallel()
		db, dir := freshAt(t)
		id := addTask(t, db, "OPEN", nil)
		raw := rawConn(t, dir)
		// Every CHECK family the enforcement map names, one violating write
		// each, all through the pragma-free connection.
		checks := map[string]string{
			"closed vocabulary": `INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t', 'SHIPPED', 'd', 'd')`,
			"DUPLICATE<->dup_of": `INSERT INTO tasks (title, status, created_at, updated_at)
			                      VALUES ('t', 'DUPLICATE', 'd', 'd')`,
			"DONE/WONT-DO=>note": `INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t', 'DONE', 'd', 'd')`,
			"PAUSED/DISSOLVED=>note": `INSERT INTO epics (slug, goal, status, created_at)
			                          VALUES ('p', 'g', 'PAUSED', 'd')`,
			"CLOSED<->stamp": `INSERT INTO epics (slug, goal, status, created_at) VALUES ('c', 'g', 'CLOSED', 'd')`,
			"met=>evidence":  `INSERT INTO epic_criteria (epic, seq, criterion, met) VALUES ('ghost', 1, 'c', 1)`,
			"non-empty title": `INSERT INTO tasks (title, created_at, updated_at)
			                   VALUES ('', 'd', 'd')`,
			"non-empty goal": `INSERT INTO epics (slug, goal, created_at) VALUES ('g2', '', 'd')`,
			"parked-only-on-open": fmt.Sprintf(
				`UPDATE tasks SET status='DONE', status_note='n', parked='later' WHERE id=%d`, id),
		}
		for family, stmt := range checks {
			if _, err := raw.ExecContext(t.Context(), stmt); err == nil || !strings.Contains(err.Error(), "CHECK") {
				t.Errorf("%s: want a CHECK refusal on the raw connection, got %v", family, err)
			}
		}
	})
	t.Run("raw-sql-second-in-progress-story-same-epic-rejected", func(t *testing.T) {
		t.Parallel()
		db, dir := freshAt(t)
		addEpic(t, db, "e", "ACTIVE")
		addStory(t, db, "IN-PROGRESS")
		raw := rawConn(t, dir)
		refuse(t, raw, "UNIQUE", `INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S2', 't', 'IN-PROGRESS')`)
	})
	t.Run("raw-connection-triggers-still-fire", func(t *testing.T) {
		t.Parallel()
		// The enforcement map's trigger row binds "accidental raw writes
		// (UPDATE/DELETE)" too: triggers are schema objects, not connection
		// settings, so a pragma-free connection cannot shed them.
		db, dir := freshAt(t)
		addTask(t, db, "OPEN", nil)
		raw := rawConn(t, dir)
		refuse(t, raw, "never deleted", `DELETE FROM tasks`)
		refuse(t, raw, "illegal status transition", `UPDATE tasks SET status='LABEL' WHERE id=1`)
	})
}

// --- single writer: the exclusive lock (INV-174) ---

func TestGatesSecondWriterBlocked(t *testing.T) {
	t.Parallel()
	t.Run("second-writer-connection-blocked-by-exclusive-lock", func(t *testing.T) {
		t.Parallel()
		if testing.Short() {
			t.Skip("waits out busy_timeout")
		}
		_, dir := freshAt(t)
		path := filepath.Join(dir, "db.sqlite")

		w1, err := schema.OpenWrite(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = w1.Close() }()
		// Pin one pool connection: the exclusive OS lock is taken at the
		// connection's FIRST WRITE and held until the CONNECTION closes —
		// not at open, not until the statement ends.
		c1, err := w1.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		// §3.1's exclusive lock arrives at the first write; empirically the
		// exclusion of foreign WRITERS starts even earlier — establishing
		// an EXCLUSIVE-mode connection reads the file header under a shared
		// lock the mode never releases, so "blocked from open" is the
		// observed behaviour and a before-first-write probe cannot pass.
		// The spec's claim (write ⇒ exclusive ⇒ everyone excluded until
		// close) is what the assertions below prove.
		_, err = c1.ExecContext(t.Context(),
			`INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`)
		if err != nil {
			t.Fatal(err)
		}

		w3, err := schema.OpenWrite(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = w3.Close() }()
		start := time.Now()
		_, err = w3.ExecContext(t.Context(), `INSERT INTO events (at, entity, event) VALUES ('d', '#2', 'create')`)
		if err == nil {
			t.Fatalf("second writer landed a write while the first holds the exclusive lock")
		}
		low := strings.ToLower(err.Error())
		if !strings.Contains(low, "busy") && !strings.Contains(low, "locked") {
			t.Fatalf("second writer failed with %q; want a busy/locked refusal", err)
		}
		// It retried within busy_timeout before giving up, as §3.1 states.
		if elapsed := time.Since(start); elapsed < 4*time.Second {
			t.Fatalf("second writer gave up after %v; want a retry window near busy_timeout (5s)", elapsed)
		}
		// A reader is excluded for the writer's lifetime too — §3.1's
		// concurrent-read-verb BUSY case.
		r, err := schema.OpenRead(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Close() }()
		var n int
		if err := r.QueryRowContext(t.Context(), `SELECT count(*) FROM events`).Scan(&n); err == nil {
			t.Fatalf("a reader read through the exclusive lock")
		}

		// And closing the holder releases the lock: the blocked writer's
		// next attempt goes through.
		if err := c1.Close(); err != nil {
			t.Fatal(err)
		}
		if err := w1.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := w3.ExecContext(t.Context(),
			`INSERT INTO events (at, entity, event) VALUES ('d', '#3', 'create')`); err != nil {
			t.Fatalf("the lock outlived the connection that held it: %v", err)
		}
	})
}

// --- journal posture and crash recovery (INV-030) ---

// TestGatesJournalRecovery proves hot-journal rollback deterministically:
// the child commits 25 rows, then opens a transaction it will never commit
// and bloats it past the page-cache spill point, so the rollback journal is
// guaranteed on disk when the parent kills it. The parent asserts the hot
// journal's presence after the kill, and after reopening: integrity_check
// ok, exactly 25 rows (the uncommitted transaction fully rolled back), and
// the journal consumed. The no-WAL half extends S1a's
// TestJournalModeIsNotWAL by asserting no -wal/-shm litter appears through
// actual write traffic.
func TestGatesJournalRecovery(t *testing.T) {
	t.Parallel()
	t.Run("no-wal-shm-and-hot-journal-recovery", func(t *testing.T) {
		t.Parallel()
		if os.Getenv("GATES_CRASH_CHILD") == "1" {
			crashChildMain()
			return
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "db.sqlite")
		// Parent creates the schema so the child only writes.
		db, err := schema.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Create(t.Context(), db); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}

		marker := filepath.Join(dir, "child-committed")
		// The child is this same test binary re-executed; os.Args[0] is not
		// user input, and the env-var paths point into t.TempDir().
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run", "TestGatesJournalRecovery") //nolint:gosec
		cmd.Env = append(os.Environ(),
			"GATES_CRASH_CHILD=1", "GATES_CRASH_DB="+path, "GATES_CRASH_MARKER="+marker)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		// The child holds the exclusive lock from its first write until it
		// dies, so the database cannot be probed from here; the child drops
		// the marker only after its uncommitted transaction has spilled to
		// the rollback journal, and then spins holding the transaction open
		// — the kill is guaranteed to land mid-transaction.
		deadline := time.Now().Add(15 * time.Second)
		for {
			if time.Now().After(deadline) {
				_ = cmd.Process.Kill()
				t.Fatal("child never reached its uncommitted transaction")
			}
			if _, err := os.Stat(marker); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err := cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		_ = cmd.Wait()

		// The smoking gun: a hot journal on disk. Without it, "recovery"
		// would be indistinguishable from a crash that touched nothing.
		if _, err := os.Stat(path + "-journal"); err != nil {
			t.Fatalf("no hot journal after the kill — the crash interrupted nothing: %v", err)
		}

		// Recovery: the reopened database must be internally consistent —
		// whatever the kill interrupted was rolled back, not half-kept.
		re, err := schema.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = re.Close() }()
		var verdict string
		if err := re.QueryRowContext(t.Context(), `PRAGMA integrity_check`).Scan(&verdict); err != nil {
			t.Fatal(err)
		}
		if verdict != "ok" {
			t.Fatalf("integrity_check after crash = %q; want ok", verdict)
		}
		var count int
		if err := re.QueryRowContext(t.Context(), `SELECT count(*) FROM events`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		// Exactly the committed prefix: fewer means committed data was
		// lost, more means part of the interrupted transaction survived —
		// either way rollback did not do its job.
		if count != 25 {
			t.Fatalf("rows after recovery = %d; want exactly the 25 committed ones", count)
		}
		// Recovery consumed the journal, and the posture never produced
		// WAL litter.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), "-journal") {
				t.Fatalf("hot journal still present after recovery: %s", e.Name())
			}
			if strings.HasSuffix(e.Name(), "-wal") || strings.HasSuffix(e.Name(), "-shm") {
				t.Fatalf("WAL litter present after write traffic: %s", e.Name())
			}
		}
	})
}

// crashChildMain is the killed process: 25 auto-commit rows, then one
// transaction it never commits, bloated with megabyte rows until SQLite
// spills it to the rollback journal on disk. Only then does it write the
// marker — the exclusive lock it holds makes the database itself
// unreadable from outside — and spins holding the transaction open, so
// the parent's kill always interrupts an in-flight transaction with a hot
// journal behind it.
func crashChildMain() {
	ctx := context.Background()
	db, err := schema.OpenWrite(os.Getenv("GATES_CRASH_DB"))
	if err != nil {
		os.Exit(1)
	}
	for range 25 {
		_, err := db.ExecContext(ctx,
			`INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`)
		if err != nil {
			os.Exit(1)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		os.Exit(1)
	}
	big := strings.Repeat("x", 1<<20)
	for range 30 {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO events (at, entity, event, detail) VALUES ('d', '#1', 'create', ?)`, big)
		if err != nil {
			os.Exit(1)
		}
	}
	// 30 MB of uncommitted rows is far past the page-cache spill point, so
	// the hot journal is on disk before the marker exists.
	// The marker path is set by the parent test and points into its
	// t.TempDir(); nothing here is user input.
	if err := os.WriteFile(os.Getenv("GATES_CRASH_MARKER"), nil, 0o600); err != nil { //nolint:gosec
		os.Exit(1)
	}
	select {} // hold the transaction open until the kill
}
