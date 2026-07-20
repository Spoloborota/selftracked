package verify_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // the raw, FK-off connection the Stage-0 fixture needs

	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/rules"
	"github.com/Spoloborota/selftracked/internal/schema"
	"github.com/Spoloborota/selftracked/internal/verify"
)

// harness is a fresh tracker in a temp git repo: .selftracked/db.sqlite
// created, a matching dump written, ready to seed pathological state.
type harness struct {
	t    *testing.T
	root string
	dir  string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init")
	dir := filepath.Join(root, ".selftracked")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, root: root, dir: dir}
	h.withDB(func(db *sql.DB) {
		if err := schema.Create(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	})
	h.regen()
	return h
}

// withDB opens the tracker with the production posture (FKs on), runs fn,
// and closes — so no handle is held open across a verify call.
func (h *harness) withDB(fn func(db *sql.DB)) {
	h.t.Helper()
	db, err := schema.Open(filepath.Join(h.dir, "db.sqlite"))
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	fn(db)
}

// exec runs one statement with FKs enforced (the normal write path).
func (h *harness) exec(query string, args ...any) {
	h.t.Helper()
	h.withDB(func(db *sql.DB) {
		if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
			h.t.Fatalf("seed %q: %v", query, err)
		}
	})
}

// execNoFK runs a statement on a raw connection with foreign keys OFF —
// the "foreign process wrote with FKs off" case Stage 0's foreign_key_check
// exists to catch (INV-010).
func (h *harness) execNoFK(query string) {
	h.t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(h.dir, "db.sqlite"))
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// A bare sql.Open (no schema DSN) leaves foreign_keys at SQLite's OFF
	// default; pinning one connection keeps every statement on that same
	// FK-off connection rather than a pooled one that might differ.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(context.Background(), query); err != nil {
		h.t.Fatalf("raw seed %q: %v", query, err)
	}
}

// regen rewrites dump.sql from the live DB, so R1 stays green after a seed.
func (h *harness) regen() {
	h.t.Helper()
	h.withDB(func(db *sql.DB) {
		text, err := dump.Serialize(context.Background(), db)
		if err != nil {
			h.t.Fatal(err)
		}
		if err := dump.WriteFiles(h.dir, text); err != nil {
			h.t.Fatal(err)
		}
	})
}

func (h *harness) verify(fast bool) verify.Report {
	h.t.Helper()
	rep, err := verify.Run(context.Background(), h.dir, fast)
	if err != nil {
		h.t.Fatalf("verify.Run: %v", err)
	}
	return rep
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	// Isolate from the machine's global/system git config — a global
	// core.hooksPath or includeIf would otherwise leak into R5/R11 tests.
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func hasRule(vs []rules.Violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

func mustRed(t *testing.T, rep verify.Report, rule string) {
	t.Helper()
	if !hasRule(rep.Red, rule) {
		t.Fatalf("expected red %s; red=%v advisory=%v", rule, rep.Red, rep.Advisory)
	}
}

func mustAdvisory(t *testing.T, rep verify.Report, rule string) {
	t.Helper()
	if !hasRule(rep.Advisory, rule) {
		t.Fatalf("expected advisory %s; red=%v advisory=%v", rule, rep.Red, rep.Advisory)
	}
}

// --- the green baseline: a consistent tracker has no red findings ---

func TestGreenTrackerNoRed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	rep := h.verify(false)
	if len(rep.Red) != 0 {
		t.Fatalf("a fresh tracker must have no red findings; got %v", rep.Red)
	}
}

// --- Stage 0: foreign_key_check red while integrity_check passes (INV-544/010) ---

func TestStage0ForeignKeyCheck(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// A dangling FK, insertable only with enforcement off. No R-rule reads
	// epic_artifacts, so foreign_key_check is the only thing that can fire.
	h.execNoFK(`INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('ghost', 999, 'home')`)
	h.regen()
	rep := h.verify(false)
	mustRed(t, rep, "fk")
}

// --- R1: a dirty dump (DB changed, dump stale) is red (INV-291/350) ---

func TestR1DirtyDump(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// A raw write with NO regen: the DB moved, the tracked dump did not.
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', '2000-01-01T00:00:00Z')`)
	rep := h.verify(false)
	mustRed(t, rep, "R1")
}

func TestR1TamperedDumpDoesNotReload(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.dir, "dump.sql"), []byte("PRAGMA evil;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep := h.verify(false)
	mustRed(t, rep, "R1") // both check 1 (mismatch) and check 2 (won't parse) fire
}

// --- R2: a missing path root is red (INV-294) ---

func TestR2MissingRoot(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.exec(`INSERT INTO path_dictionary (class, scope, root) VALUES ('research', '', 'docs/research')`)
	h.regen()
	rep := h.verify(false)
	mustRed(t, rep, "R2")

	// Creating the root clears it.
	if err := os.MkdirAll(filepath.Join(h.root, "docs/research"), 0o750); err != nil {
		t.Fatal(err)
	}
	if hasRule(h.verify(false).Red, "R2") {
		t.Fatal("R2 must clear once the root exists")
	}
}

// --- R3: an unresolved non-archived artifact is red; archived/ephemeral exempt (INV-295) ---

func TestR3UnresolvedArtifact(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.MkdirAll(filepath.Join(h.root, "docs/research"), 0o750); err != nil {
		t.Fatal(err)
	}
	h.exec(`INSERT INTO path_dictionary (class, scope, root) VALUES ('research', '', 'docs/research')`)
	h.exec(`INSERT INTO path_dictionary (class, scope, root, ephemeral) VALUES ('workdir', '', 'work', 1)`)
	if err := os.MkdirAll(filepath.Join(h.root, "work"), 0o750); err != nil {
		t.Fatal(err)
	}
	// A non-archived, non-ephemeral artifact pointing at a missing file.
	h.exec(`INSERT INTO artifacts (class, scope, relpath) VALUES ('research', '', 'gone.md')`)
	// An archived one and an ephemeral one must NOT be flagged.
	h.exec(`INSERT INTO artifacts (class, scope, relpath, archived) VALUES ('research', '', 'old.md', 1)`)
	h.exec(`INSERT INTO artifacts (class, scope, relpath) VALUES ('workdir', '', 'scratch')`)
	h.regen()
	rep := h.verify(false)
	mustRed(t, rep, "R3")
	if n := countRule(rep.Red, "R3"); n != 1 {
		t.Fatalf("only the live non-ephemeral artifact should flag; got %d R3 violations", n)
	}
}

func countRule(vs []rules.Violation, rule string) int {
	n := 0
	for _, v := range vs {
		if v.Rule == rule {
			n++
		}
	}
	return n
}

// --- R4: a correction targeting a correction is red (INV-297) ---

func TestR4CorrectionChain(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', '2020-01-01T00:00:00Z')`)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S1', 't', 'DONE')`)
	h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits) VALUES ('e', 1, 'S1', 'd', 'DONE', 'abc')`)
	h.exec(`INSERT INTO worklog (epic, seq, story, date, state, corrects) VALUES ('e', 2, 'S1', 'd', 'DONE', 1)`)
	// seq 3 corrects seq 2 — a correction of a correction: the no-chain backstop.
	h.exec(`INSERT INTO worklog (epic, seq, story, date, state, corrects) VALUES ('e', 3, 'S1', 'd', 'DONE', 2)`)
	h.regen()
	mustRed(t, h.verify(false), "R4")
}

func TestR4StoryMembership(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', '2020-01-01T00:00:00Z')`)
	// worklog names story S9, which does not exist.
	h.exec(`INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S9', 'd', 'IN-PROGRESS')`)
	h.regen()
	mustRed(t, h.verify(false), "R4")
}

// --- R5: an unresolvable commit is red; a real one resolves (INV-299) ---

func TestR5CommitResolution(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.root, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "add", "-A")
	git(t, h.root, "commit", "-m", "seed")
	realSHA := git(t, h.root, "rev-parse", "HEAD")

	const wl = `INSERT INTO worklog (epic, seq, story, date, state, commits) VALUES ('e', ?, 'S1', 'd', 'DONE', ?)`
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', '2020-01-01T00:00:00Z')`)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S1', 't', 'DONE')`)
	h.exec(wl, 1, realSHA)
	// A sha-shaped token that resolves to nothing (an import-preserved typo).
	h.exec(wl, 2, "deadbeefdeadbeefdeadbeef")
	// A legacy: cell is exempt.
	h.exec(wl, 3, "legacy: no sha")
	h.regen()
	rep := h.verify(false)
	mustRed(t, rep, "R5")
	if n := countRule(rep.Red, "R5"); n != 1 {
		t.Fatalf("only the deadbeef token should flag; got %d R5 violations", n)
	}
}

// --- the DB-only red rules verify must surface, each branch (R4, R6-R9, R12) ---

func TestDBRulesRoutedToRed(t *testing.T) {
	t.Parallel()
	// Every case regens the dump after seeding, so R1 stays green and only
	// the target rule fires; a fresh harness has no events, so no boundary
	// tamper drops rows. mustRed asserts the target is among the red set.
	cases := []struct {
		name string
		rule string
		seed func(h *harness)
	}{
		{name: "R4-vrow-on-non-closed-epic", rule: "R4", seed: func(h *harness) {
			// Clause 1b: a V-row is legal only on a CLOSED epic.
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'V-1', 'd', 'DONE')`)
		}},
		{name: "R6-done-story-no-worklog", rule: "R6", seed: func(h *harness) {
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DONE')`)
		}},
		{name: "R6-worklog-done-no-done-story", rule: "R6", seed: func(h *harness) {
			// Second direction: a DONE worklog row whose story is not DONE.
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','PLANNED')`)
			h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits) VALUES ('e', 1, 'S1', 'd', 'DONE', 'abc')`)
		}},
		{name: "R7-duplicates-link-no-dup_of", rule: "R7", seed: func(h *harness) {
			h.exec(`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('a','OPEN','d','d')`)
			h.exec(`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('b','OPEN','d','d')`)
			// A duplicates link with no matching dup_of: the mapping is not one-to-one.
			h.exec(`INSERT INTO task_links (from_task, to_task, type) VALUES (1, 2, 'duplicates')`)
		}},
		{name: "R8-unresolved-event-entity", rule: "R8", seed: func(h *harness) {
			// Existence branch: parses, but names a task that does not exist.
			h.exec(`INSERT INTO events (at, entity, event) VALUES ('d', '#999', 'set-status')`)
		}},
		{name: "R8-malformed-event-entity", rule: "R8", seed: func(h *harness) {
			// Grammar branch: does not parse as a §4 reference at all.
			h.exec(`INSERT INTO events (at, entity, event) VALUES ('d', 'not a ref', 'set-status')`)
		}},
		{name: "R9-boundary-tamper", rule: "R9", seed: func(h *harness) {
			h.exec(`UPDATE meta SET value = '5' WHERE key = 'events_archived_through'`)
		}},
		{name: "R9-boundary-noncanonical-zero", rule: "R9", seed: func(h *harness) {
			// "00" is numerically zero but not the canonical byte-string — a
			// raw write no verb could have made (regression for the S7 fix).
			h.exec(`UPDATE meta SET value = '00' WHERE key = 'events_archived_through'`)
		}},
		{name: "R9-sqlite-sequence-floor", rule: "R9", seed: func(h *harness) {
			// The AUTOINCREMENT high-water dropped below MAX(id): a raw tamper.
			// sqlite_sequence is not serialized, so the dump is unaffected.
			h.exec(`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t','OPEN','d','d')`)
			h.exec(`UPDATE sqlite_sequence SET seq = 0 WHERE name = 'tasks'`)
		}},
		{name: "R12-terminal-task-no-trail", rule: "R12", seed: func(h *harness) {
			h.exec(`INSERT INTO tasks (title, status, status_note, created_at, updated_at)
			        VALUES ('t','DONE','done','d','d')`)
		}},
		{name: "R12-terminal-epic-no-trail", rule: "R12", seed: func(h *harness) {
			// The epic branch: a CLOSED epic with no matching events row.
			h.exec(`INSERT INTO epics (slug, goal, status, close_sweep, created_at)
			        VALUES ('e','g','CLOSED','swept','d')`)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			c.seed(h)
			h.regen()
			mustRed(t, h.verify(false), c.rule)
		})
	}
}

// --- R10 advisory idle report, and its correction-clock exclusion (INV-306/117) ---

func TestR10IdleEpic(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	old := time.Now().UTC().AddDate(0, 0, -60).Format("2006-01-02T15:04:05Z")
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', ?)`, old)
	h.regen()
	mustAdvisory(t, h.verify(false), "R10")
}

func TestR10RecentAppendSuppresses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	old := time.Now().UTC().AddDate(0, 0, -60).Format("2006-01-02T15:04:05Z")
	recent := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', ?)`, old)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S1', 't', 'DONE')`)
	h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits) VALUES ('e', 1, 'S1', ?, 'DONE', 'abc')`, recent)
	h.regen()
	if hasRule(h.verify(false).Advisory, "R10") {
		t.Fatal("a recent non-correction append must suppress R10")
	}
}

func TestR10CorrectionDoesNotResetClock(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	old := time.Now().UTC().AddDate(0, 0, -60).Format("2006-01-02T15:04:05Z")
	recent := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', ?)`, old)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S1', 't', 'DONE')`)
	h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits) VALUES ('e', 1, 'S1', ?, 'DONE', 'abc')`, old)
	// A RECENT correction of the old row must NOT reset the idle clock (§5.7).
	h.exec(`INSERT INTO worklog (epic, seq, story, date, state, corrects) VALUES ('e', 2, 'S1', ?, 'DONE', 1)`, recent)
	h.regen()
	mustAdvisory(t, h.verify(false), "R10")
}

// --- R13 advisory: OPEN task with no home link (INV-311) ---

func TestR13OpenTaskNoHome(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.exec(`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t', 'OPEN', 'd', 'd')`)
	h.regen()
	mustAdvisory(t, h.verify(false), "R13")
}

// --- R15 advisory: a pending skip marker (INV-278/312) ---

func TestR15SkipMarker(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.dir, "skip-pending"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	mustAdvisory(t, h.verify(false), "R15")
}

// --- the --fast partition: pure-SQL + R15, skipping filesystem/git rules (INV-289) ---

func TestFastPartition(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// One filesystem violation (R2) and one pure-SQL violation (R12).
	h.exec(`INSERT INTO path_dictionary (class, scope, root) VALUES ('research', '', 'docs/research')`)
	h.exec(`INSERT INTO tasks (title, status, status_note, created_at, updated_at) VALUES ('t','DONE','done','d','d')`)
	h.regen()

	full := h.verify(false)
	mustRed(t, full, "R2")  // filesystem rule runs in full
	mustRed(t, full, "R12") // pure-SQL rule runs in full

	fast := h.verify(true)
	mustRed(t, fast, "R12") // pure-SQL rule runs in --fast
	if hasRule(fast.Red, "R2") {
		t.Fatal("--fast must skip the filesystem rule R2")
	}
	if hasRule(fast.Red, "R1") {
		t.Fatal("--fast must skip the serialization rule R1")
	}
}

func TestFastRunsR15SkipsR11(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.dir, "skip-pending"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	fast := h.verify(true)
	mustAdvisory(t, fast, "R15") // R15 is the one advisory cheap enough for --fast
	if hasRule(fast.Advisory, "R11") {
		t.Fatal("--fast must skip the git-bound advisory rule R11")
	}
}

// --- a missing tracker is an error, not an empty green report ---

func TestNoTracker(t *testing.T) {
	t.Parallel()
	_, err := verify.Run(context.Background(), t.TempDir(), false)
	if err == nil {
		t.Fatal("verify on a directory with no tracker must error")
	}
}

// --- R1 check 2 must not re-surface a DB-only violation as a second finding ---

// TestR1CheckTwoNoDoubleCount guards the S7-close fix: an R6 violation must
// appear once (as R6), never a second time mislabelled R1 "does not
// rebuild" — load.Build re-runs the DB-only rules, and check 2 is skipped
// when the DB is already dirty.
func TestR1CheckTwoNoDoubleCount(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DONE')`)
	h.regen()
	rep := h.verify(false)
	mustRed(t, rep, "R6")
	if hasRule(rep.Red, "R1") {
		t.Fatalf("R6 must not also surface as an R1 rebuild failure; red=%v", rep.Red)
	}
}

// --- R7's chain clause (INV-301's named clause): dup_of target is DUPLICATE ---

func TestR7ChainClause(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// C(3) OPEN ← B(2) DUPLICATE ← A(1) DUPLICATE. A points at B, itself a
	// DUPLICATE: a chain. Links are added so only the chain clause fires.
	h.exec(`INSERT INTO tasks (id, title, status, created_at, updated_at) VALUES (3,'c','OPEN','d','d')`)
	h.exec(`INSERT INTO tasks (id, title, status, dup_of, created_at, updated_at) VALUES (2,'b','DUPLICATE',3,'d','d')`)
	h.exec(`INSERT INTO tasks (id, title, status, dup_of, created_at, updated_at) VALUES (1,'a','DUPLICATE',2,'d','d')`)
	h.exec(`INSERT INTO task_links (from_task, to_task, type) VALUES (1,2,'duplicates'),(2,3,'duplicates')`)
	// Terminal DUPLICATE tasks need an events trail or R12 also fires.
	h.exec(`INSERT INTO events (at, entity, event) VALUES ('d','#1','set-status'),('d','#2','set-status')`)
	h.regen()
	mustRed(t, h.verify(false), "R7")
}

// --- R11 driven in situ through verify.Run, not only the string matcher ---

func TestR11InSitu(t *testing.T) {
	// t.Setenv (no t.Parallel): isolate the in-process git calls verify makes
	// from the machine's global/system config, e.g. a global core.hooksPath.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	// Subtests are NOT parallel: t.Setenv above forbids it, and each mutates
	// process-global git config state through its own repo anyway.
	t.Run("hooksPath-points-at-ours-clears", func(t *testing.T) {
		h := newHarness(t)
		if err := os.MkdirAll(filepath.Join(h.dir, "hooks"), 0o750); err != nil {
			t.Fatal(err)
		}
		git(t, h.root, "config", "core.hooksPath", ".selftracked/hooks")
		if hasRule(h.verify(false).Advisory, "R11") {
			t.Fatal("R11 must clear when the effective hooks dir IS .selftracked/hooks")
		}
	})

	t.Run("chained-hook-content-clears", func(t *testing.T) {
		h := newHarness(t)
		for _, name := range []string{"pre-commit", "post-commit"} {
			writeHook(t, h.root, name, "#!/bin/sh\nexec .selftracked/hooks/"+name+" \"$@\"\n")
		}
		if hasRule(h.verify(false).Advisory, "R11") {
			t.Fatal("R11 must clear when both hook files chain their counterpart")
		}
	})

	t.Run("hook-exists-but-unchained-warns", func(t *testing.T) {
		h := newHarness(t)
		writeHook(t, h.root, "pre-commit", "#!/bin/sh\nexec ./some-other-linter\n")
		if !hasRule(h.verify(false).Advisory, "R11") {
			t.Fatal("a hook that exists but does not chain the counterpart must warn")
		}
	})
}

func writeHook(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// --- a populated, valid tracker has no red findings (not a vacuous green) ---

func TestPopulatedTrackerNoRed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// A real commit so the DONE worklog row's R5 reference resolves.
	if err := os.WriteFile(filepath.Join(h.root, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "add", "-A")
	git(t, h.root, "commit", "-m", "seed")
	sha := git(t, h.root, "rev-parse", "HEAD")

	// A valid DONE story with its matching DONE worklog row (R4/R6 pass).
	const wl = `INSERT INTO worklog (epic, seq, story, date, state, commits)
	            VALUES ('e',1,'S1','2020-01-02T00:00:00Z','DONE',?)`
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DONE')`)
	h.exec(wl, sha)
	// A valid duplicate pair with its link and an events trail (R7/R12 pass).
	h.exec(`INSERT INTO tasks (id, title, status, created_at, updated_at) VALUES (1,'canon','OPEN','d','d')`)
	h.exec(`INSERT INTO tasks (id, title, status, dup_of, created_at, updated_at) VALUES (2,'dupe','DUPLICATE',1,'d','d')`)
	h.exec(`INSERT INTO task_links (from_task, to_task, type) VALUES (2,1,'duplicates')`)
	h.exec(`INSERT INTO events (at, entity, event) VALUES ('d','#2','set-status')`)
	h.regen()

	rep := h.verify(false)
	if len(rep.Red) != 0 {
		t.Fatalf("a populated, valid tracker must have no red findings; got %v", rep.Red)
	}
}

// --- R13 negative control: an OPEN task WITH a home link is not flagged ---

func TestR13OpenTaskWithHomeNotFlagged(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.MkdirAll(filepath.Join(h.root, "docs/research"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, "docs/research/home.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.exec(`INSERT INTO path_dictionary (class, scope, root) VALUES ('research','','docs/research')`)
	h.exec(`INSERT INTO artifacts (class, scope, relpath) VALUES ('research','','home.md')`)
	h.exec(`INSERT INTO tasks (id, title, status, created_at, updated_at) VALUES (1,'t','OPEN','d','d')`)
	h.exec(`INSERT INTO task_artifacts (task, artifact, role) VALUES (1,1,'home')`)
	h.regen()
	if hasRule(h.verify(false).Advisory, "R13") {
		t.Fatal("an OPEN task with a home link must not be flagged by R13")
	}
}

// --- INV-495: a mechanized audit that every emittable rule has a red fixture ---

// TestRuleFixtureCoverage is the gate INV-495 asks for: a failing fixture
// per rule, enumerated so a rule added to the engine without one is caught
// here rather than by a manual second pass (the way R7/R8 were the first
// time). wantRules is the authoritative list of what the §7 engine can emit
// (Stage 0's fk plus every Stage-1 rule except R14, deferred to S8c); adding
// a rule to the engine means adding it here AND a fixture below, or this
// test fails. integrity_check is excluded: its red state (a corrupt page)
// is not constructible hermetically without risking an unopenable DB.
func TestRuleFixtureCoverage(t *testing.T) {
	t.Parallel()
	wantRules := []string{
		"fk", "R1", "R2", "R3", "R4", "R5", "R6", "R7", "R8", "R9", "R10", "R11", "R12", "R13", "R15",
	}
	advisory := map[string]bool{"R10": true, "R11": true, "R13": true, "R15": true}
	fixtures := map[string]func(h *harness){
		"fk": func(h *harness) {
			h.execNoFK(`INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('ghost', 999, 'home')`)
			h.regen()
		},
		"R1": func(h *harness) {
			_ = os.WriteFile(filepath.Join(h.dir, "dump.sql"), []byte("garbage\n"), 0o600)
		},
		"R2": func(h *harness) {
			h.exec(`INSERT INTO path_dictionary (class, scope, root) VALUES ('research','','docs/research')`)
			h.regen()
		},
		"R3": func(h *harness) {
			_ = os.MkdirAll(filepath.Join(h.root, "docs/research"), 0o750)
			h.exec(`INSERT INTO path_dictionary (class, scope, root) VALUES ('research','','docs/research')`)
			h.exec(`INSERT INTO artifacts (class, scope, relpath) VALUES ('research','','gone.md')`)
			h.regen()
		},
		"R4": func(h *harness) {
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e',1,'V-1','d','DONE')`)
			h.regen()
		},
		"R5": func(h *harness) {
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DONE')`)
			h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits)
			        VALUES ('e',1,'S1','d','DONE','deadbeefdeadbeefdeadbeef')`)
			h.regen()
		},
		"R6": func(h *harness) {
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DONE')`)
			h.regen()
		},
		"R7": func(h *harness) {
			h.exec(`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('a','OPEN','d','d')`)
			h.exec(`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('b','OPEN','d','d')`)
			h.exec(`INSERT INTO task_links (from_task, to_task, type) VALUES (1,2,'duplicates')`)
			h.regen()
		},
		"R8": func(h *harness) {
			h.exec(`INSERT INTO events (at, entity, event) VALUES ('d','#999','set-status')`)
			h.regen()
		},
		"R9": func(h *harness) {
			h.exec(`UPDATE meta SET value = '5' WHERE key = 'events_archived_through'`)
			h.regen()
		},
		"R10": func(h *harness) {
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2000-01-01T00:00:00Z')`)
			h.regen()
		},
		"R11": func(_ *harness) {}, // a fresh git repo has no chained hooks
		"R12": func(h *harness) {
			h.exec(`INSERT INTO tasks (title, status, status_note, created_at, updated_at) VALUES ('t','DONE','d','d','d')`)
			h.regen()
		},
		"R13": func(h *harness) {
			h.exec(`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t','OPEN','d','d')`)
			h.regen()
		},
		"R15": func(h *harness) {
			_ = os.WriteFile(filepath.Join(h.dir, "skip-pending"), []byte(""), 0o600)
		},
	}

	// Completeness: the fixture set must match wantRules exactly.
	if len(fixtures) != len(wantRules) {
		t.Fatalf("fixture count %d != wantRules count %d", len(fixtures), len(wantRules))
	}
	for _, r := range wantRules {
		if _, ok := fixtures[r]; !ok {
			t.Fatalf("rule %s is in wantRules but has no red fixture", r)
		}
	}
	// Each fixture must actually make verify emit its rule.
	for _, rule := range wantRules {
		t.Run(rule, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			fixtures[rule](h)
			rep := h.verify(false)
			if advisory[rule] {
				mustAdvisory(t, rep, rule)
			} else {
				mustRed(t, rep, rule)
			}
		})
	}
}
