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
	db.SetMaxOpenConns(1) // pin one connection so PRAGMA foreign_keys(off) holds
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
	//nolint:gosec // test helper, fixed git binary
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
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

// --- the DB-only red rules verify must surface (R6-R9, R12) ---

func TestDBRulesRoutedToRed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rule string
		seed func(h *harness)
	}{
		{"R6-done-story-no-worklog", "R6", func(h *harness) {
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DONE')`)
		}},
		{"R9-boundary-tamper", "R9", func(h *harness) {
			h.exec(`UPDATE meta SET value = '5' WHERE key = 'events_archived_through'`)
		}},
		{"R12-terminal-no-trail", "R12", func(h *harness) {
			h.exec(`INSERT INTO tasks (title, status, status_note, created_at, updated_at)
			        VALUES ('t','DONE','done','d','d')`)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			c.seed(h)
			// No regen for the boundary case (regen would drop events); R1
			// may also fire, which is fine — we assert the target is present.
			if c.rule != "R9" {
				h.regen()
			}
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
