package verify_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // the raw, FK-off connection the Stage-0 fixture needs

	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/rules"
	"github.com/Spoloborota/selftracked/internal/schema"
	"github.com/Spoloborota/selftracked/internal/state"
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

// regen rewrites dump.sql AND STATE.md from the live DB, so R1 (checks 1 and
// 3) stays green after a seed — a real tracker carries both, and R1 check 3
// (the folded R14, S8c) now asserts STATE.md byte-equals its render.
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
		stateMD, err := state.Render(context.Background(), db)
		if err != nil {
			h.t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(h.root, "STATE.md"), stateMD, 0o644); err != nil {
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

// rulesNamed is every message a given rule emitted, in report order — the
// shape a test needs when one rule name carries several distinguishable
// findings and presence alone would not say which fired.
func rulesNamed(vs []rules.Violation, rule string) []string {
	var out []string
	for _, v := range vs {
		if v.Rule == rule {
			out = append(out, v.Message)
		}
	}
	return out
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

// --- R1 check 3 (the folded R14): STATE.md byte-equals its render (INV-275/293) ---

// TestR1StateMdTamperRed: a STATE.md that no longer matches its render is a
// red R1 finding — the former standalone R14, now check 3.
func TestR1StateMdTamperRed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// A consistent tracker is green (the render matches the committed file).
	if hasRule(h.verify(false).Red, "R1") {
		t.Fatal("a consistent tracker must not fire R1")
	}
	// Hand-edit STATE.md so it drifts from the DB-derived render.
	if err := os.WriteFile(filepath.Join(h.root, "STATE.md"), []byte("# tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRed(t, h.verify(false), "R1")
}

// TestR1StateMdMissingRed: an absent STATE.md is red — the tracked projection
// is gone, which R1 check 3 treats as a violation, not an infra failure.
func TestR1StateMdMissingRed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.Remove(filepath.Join(h.root, "STATE.md")); err != nil {
		t.Fatal(err)
	}
	mustRed(t, h.verify(false), "R1")
}

// TestR1StateCheckIsNotFast is the partition assertion: R1 check 3 is a
// serialization-bound rule (§7), so --fast skips it — a tampered STATE.md is
// invisible to the pre-commit path, which regenerates it right after anyway.
func TestR1StateCheckIsNotFast(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.root, "STATE.md"), []byte("# tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hasRule(h.verify(true).Red, "R1") {
		t.Fatal("--fast must skip R1 (STATE.md check included)")
	}
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

// --- R10 advisory: the homeless trigger, the idle report, and the
// correction-clock exclusion (INV-306/117; amendment
// `r10-sees-the-window-it-was-meant-to-watch`) ---

func TestR10IdleEpic(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	old := time.Now().UTC().AddDate(0, 0, -60).Format("2006-01-02T15:04:05Z")
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', ?)`, old)
	h.regen()
	mustAdvisory(t, h.verify(false), "R10")
}

// TestR10Messages pins trigger (a), trigger (b), their combination and
// their silences by MESSAGE, not by rule presence: the two triggers share
// one rule name, so a presence check cannot tell which fired — and the
// "one epic, one line" contract is only visible in the count.
//
// The dead-zone row (`all-terminal-recent-append`) is the seed the retired
// TestR10RecentAppendSuppresses asserted SILENCE on. It is the exact state
// #58 described — the last story's DONE row appended minutes ago, so the
// idle clause is false by construction while no work can be recorded
// anywhere — and it is now the trigger's headline case.
func TestR10Messages(t *testing.T) {
	t.Parallel()
	old := time.Now().UTC().AddDate(0, 0, -60).Format("2006-01-02T15:04:05Z")
	recent := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	const (
		homeless = "epic e has no story that can receive work; new work has no home"
		idle     = "epic e is idle (no active story, no append in 14 days)"
		both     = "epic e has no story that can receive work; new work has no home; " +
			"and it is idle (no append in 14 days)"
	)
	for name, tc := range map[string]struct {
		seed func(h *harness)
		want string // "" = R10 must stay silent
	}{
		"no-stories-at-all": {
			// No story and no append: both facts hold, one line states both.
			seed: func(_ *harness) {},
			want: both,
		},
		"all-terminal-recent-append": {
			// Trigger (a) alone — the dead zone the idle clock cannot see.
			seed: func(h *harness) {
				h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DONE')`)
				h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits)
				        VALUES ('e',1,'S1',?,'DONE','abc')`, recent)
			},
			want: homeless,
		},
		"all-terminal-old-append": {
			seed: func(h *harness) {
				h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DONE')`)
				h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits)
				        VALUES ('e',1,'S1',?,'DONE','abc')`, old)
			},
			want: both,
		},
		"planned-story-old-append": {
			// Trigger (b) alone: PLANNED is a home, so (a) is silent, but it
			// is not READY/IN-PROGRESS, so the idle clause still watches it.
			seed: func(h *harness) {
				h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','PLANNED')`)
				// The row's STATE is immaterial to R10 — only its date and its
				// non-correction-ness are read.
				h.exec(`INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e',1,'S1',?,'IN-PROGRESS')`, old)
			},
			want: idle,
		},
		"planned-story-recent-append": {
			// The property the retired fixture was really guarding: a recent
			// non-correction append still suppresses the IDLE half.
			seed: func(h *harness) {
				h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','PLANNED')`)
				h.exec(`INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e',1,'S1',?,'IN-PROGRESS')`, recent)
			},
			want: "",
		},
		"ready-story-old-append": {
			// READY silences both clauses; (a) because it is non-terminal,
			// (b) because it is exactly what (b) looks for.
			seed: func(h *harness) {
				h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','READY')`)
			},
			want: "",
		},
		"blocked-story": {
			// BLOCKED is the sanctioned PO-absent state: a home for (a), not
			// an active story for (b) — so only the idle line may appear.
			seed: func(h *harness) {
				h.exec(`INSERT INTO stories (epic, id, title, status, blocked) VALUES ('e','S1','t','BLOCKED','why')`)
			},
			want: idle,
		},
		"paused-epic-is-silent": {
			seed: func(h *harness) {
				h.exec(`UPDATE epics SET status = 'PAUSED', status_note = 'parked' WHERE slug = 'e'`)
			},
			want: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE',?)`, old)
			tc.seed(h)
			h.regen()
			got := rulesNamed(h.verify(false).Advisory, "R10")
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("R10 must stay silent; got %v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("one epic must yield exactly one R10 line; got %v", got)
			}
			if got[0] != tc.want {
				t.Fatalf("R10 message:\n got %q\nwant %q", got[0], tc.want)
			}
		})
	}
}

func TestR10CorrectionDoesNotResetClock(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	old := time.Now().UTC().AddDate(0, 0, -60).Format("2006-01-02T15:04:05Z")
	recent := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', ?)`, old)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S1', 't', 'DONE')`)
	// S2 keeps the epic OUT of trigger (a) so this test still isolates the
	// idle clock: without it the homeless trigger would fire regardless of
	// the correction and the assertion would prove nothing.
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S2', 't', 'PLANNED')`)
	h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits) VALUES ('e', 1, 'S1', ?, 'DONE', 'abc')`, old)
	// A RECENT correction of the old row must NOT reset the idle clock (§5.7).
	h.exec(`INSERT INTO worklog (epic, seq, story, date, state, corrects) VALUES ('e', 2, 'S1', ?, 'DONE', 1)`, recent)
	h.regen()
	got := rulesNamed(h.verify(false).Advisory, "R10")
	want := "epic e is idle (no active story, no append in 14 days)"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("R10 must report the IDLE half only; got %v, want [%q]", got, want)
	}
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

// TestFastRunsR15AndR10SkipsR11 pins the advisory half of the partition:
// R15 (a bare file check) and R10 (pure SQL, moved in by amendment
// `r10-sees-the-window-it-was-meant-to-watch`) run at the commit boundary;
// the git-bound R11 does not.
func TestFastRunsR15AndR10SkipsR11(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.dir, "skip-pending"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
	h.regen()
	fast := h.verify(true)
	mustAdvisory(t, fast, "R15")
	mustAdvisory(t, fast, "R10")
	if hasRule(fast.Advisory, "R11") {
		t.Fatal("--fast must skip the git-bound advisory rule R11")
	}
	if hasRule(fast.Advisory, "R16") {
		t.Fatal("--fast must skip the full-only advisory rule R16")
	}
}

// --- R16: every finding names a repair (amendment
// `r16-reports-only-what-a-verb-can-clear`) ---

// TestR16NamesItsRepair pins all three clause texts. The repair is the
// finding's whole point — an advisory a reader cannot act on is one they
// learn to scroll past — so it is asserted verbatim, not by substring of
// the condition half alone.
func TestR16NamesItsRepair(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.exec(`INSERT INTO epics (slug, goal, status, close_sweep, created_at)
	        VALUES ('e','g','CLOSED','2026-01-01','d')`)
	h.exec(`INSERT INTO events (at, entity, event, detail) VALUES ('d','epic:e','epic','close: 2026-01-01')`)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','PLANNED')`)
	h.exec(`INSERT INTO tasks (title, epic, status, created_at, updated_at) VALUES ('t','e','OPEN','d','d')`)
	h.exec(`INSERT INTO epic_criteria (epic, seq, criterion, met) VALUES ('e',1,'c',0)`)
	h.regen()
	got := rulesNamed(h.verify(false).Advisory, "R16")
	want := []string{
		"closed epic e: criterion 1 is not met; no verb writes this state here — it arrived by raw SQL",
		"closed epic e: story S1 is not terminal; repair: selftracked story dissolve e S1 --why TEXT",
		"closed epic e: task #1 (OPEN) is homed here; repair: selftracked edit 1 --detach, or set it terminal",
	}
	if len(got) != len(want) {
		t.Fatalf("R16 findings:\n got %v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("R16 finding %d:\n got %q\nwant %q", i, got[i], want[i])
		}
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

// --- R13 counts LIVE homes only (amendment `r13-counts-live-homes`) ---

func TestR13ArchivedHomeDoesNotCount(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.exec(`INSERT INTO path_dictionary (class, scope, root) VALUES ('research','','docs/research')`)
	h.exec(`INSERT INTO artifacts (class, scope, relpath, archived) VALUES ('research','','gone.md',1)`)
	h.exec(`INSERT INTO tasks (id, title, status, created_at, updated_at) VALUES (1,'t','OPEN','d','d')`)
	h.exec(`INSERT INTO task_artifacts (task, artifact, role) VALUES (1,1,'home')`)
	h.regen()
	// The root is missing on disk on purpose: an archived artifact is
	// exempt from R3, which is exactly why counting it as a home lets an
	// OPEN task hold a pointer to nothing with no signal anywhere.
	mustAdvisory(t, h.verify(false), "R13")
}

func TestR13LiveHomeBesideAnArchivedOneCounts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := os.MkdirAll(filepath.Join(h.root, "docs/research"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, "docs/research/live.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.exec(`INSERT INTO path_dictionary (class, scope, root) VALUES ('research','','docs/research')`)
	h.exec(`INSERT INTO artifacts (id, class, scope, relpath, archived) VALUES (1,'research','','old.md',1)`)
	h.exec(`INSERT INTO artifacts (id, class, scope, relpath) VALUES (2,'research','','live.md')`)
	h.exec(`INSERT INTO tasks (id, title, status, created_at, updated_at) VALUES (1,'t','OPEN','d','d')`)
	h.exec(`INSERT INTO task_artifacts (task, artifact, role) VALUES (1,1,'home')`)
	h.exec(`INSERT INTO task_artifacts (task, artifact, role) VALUES (1,2,'home')`)
	h.regen()
	if hasRule(h.verify(false).Advisory, "R13") {
		t.Fatal("one live home is a home, whatever else was archived")
	}
}

// --- R16: a closed epic still satisfies what it was closed on ---

func TestR16ImportedClosedEpicIsExempt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// Same divergent shape as R16's fixture, but the epic arrived CLOSED
	// through import: it never passed the gate, so there is no claim to
	// re-check (amendment `terminal-epic-conditions-stay-true`).
	h.exec(`INSERT INTO epics (slug, goal, status, close_sweep, created_at)
	        VALUES ('e','g','CLOSED','2026-01-01','d')`)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','PLANNED')`)
	h.exec(`INSERT INTO events (at, entity, event, detail) VALUES ('d','epic:e','import','corpus')`)
	h.regen()
	if hasRule(h.verify(false).Advisory, "R16") {
		t.Fatal("an imported CLOSED epic never passed the close gate; R16 must not claim it did")
	}
}

func TestR16ClosedEpicWithHomedWorkableTask(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.exec(`INSERT INTO epics (slug, goal, status, close_sweep, created_at)
	        VALUES ('e','g','CLOSED','2026-01-01','d')`)
	h.exec(`INSERT INTO events (at, entity, event, detail) VALUES ('d','epic:e','epic','close: 2026-01-01')`)
	h.exec(`INSERT INTO tasks (id, title, status, epic, created_at, updated_at)
	        VALUES (1,'t','OPEN','e','d','d')`)
	h.regen()
	mustAdvisory(t, h.verify(false), "R16")
}

func TestR16CleanClosedEpicIsSilent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.exec(`INSERT INTO epics (slug, goal, status, close_sweep, created_at)
	        VALUES ('e','g','CLOSED','2026-01-01','d')`)
	h.exec(`INSERT INTO events (at, entity, event, detail) VALUES ('d','epic:e','epic','close: 2026-01-01')`)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DISSOLVED')`)
	h.exec(`INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('e',1,'c',1,'ev')`)
	h.regen()
	if hasRule(h.verify(false).Advisory, "R16") {
		t.Fatal("a closed epic that still satisfies its conditions must stay silent")
	}
}

// --- INV-495: a mechanized audit that every emittable rule has a red fixture ---

// ruleFixture is one entry of the INV-495 coverage gate: the pathological
// state to seed, and the message fragments the rule must then emit.
//
// `want` exists because rule-name presence is too weak a gate for a rule
// with several branches. R10 carries two independent triggers and R16
// three clauses; a fixture that trips ANY of them keeps the gate green
// while another branch is deleted outright — which is what happened, and
// what these fragments close. A rule with one branch may leave `want`
// empty; the seed then only has to make the rule speak.
type ruleFixture struct {
	seed func(h *harness)
	want []string
}

// TestRuleFixtureCoverage is the gate INV-495 asks for: a failing fixture
// per rule, enumerated so a rule added to the engine without one is caught
// here rather than by a manual second pass (the way R7/R8 were the first
// time). wantRules is the authoritative list of what the §7 engine can emit
// (Stage 0's fk plus every Stage-1 rule except R14, deferred to S8c); adding
// a rule to the engine means adding it here AND a fixture below, or this
// test fails. integrity_check is excluded: its red state (a corrupt page)
// is not constructible hermetically without risking an unopenable DB.
//
// §7's "a gate that cannot fail is decoration" is the standard this test is
// held to itself, which is why the multi-branch rules assert their message
// text: reverting a branch must turn THIS gate red, not merely some ad-hoc
// test elsewhere in the file.
func TestRuleFixtureCoverage(t *testing.T) {
	t.Parallel()
	wantRules := []string{
		"fk", "R1", "R2", "R3", "R4", "R5", "R6", "R7", "R8", "R9", "R10", "R11", "R12", "R13", "R15",
		"R16",
	}
	advisory := map[string]bool{"R10": true, "R11": true, "R13": true, "R15": true, "R16": true}
	recent := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	fixtures := map[string]ruleFixture{
		"fk": {seed: func(h *harness) {
			h.execNoFK(`INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('ghost', 999, 'home')`)
			h.regen()
		}},
		"R1": {seed: func(h *harness) {
			_ = os.WriteFile(filepath.Join(h.dir, "dump.sql"), []byte("garbage\n"), 0o600)
		}},
		"R2": {seed: func(h *harness) {
			h.exec(`INSERT INTO path_dictionary (class, scope, root) VALUES ('research','','docs/research')`)
			h.regen()
		}},
		"R3": {seed: func(h *harness) {
			_ = os.MkdirAll(filepath.Join(h.root, "docs/research"), 0o750)
			h.exec(`INSERT INTO path_dictionary (class, scope, root) VALUES ('research','','docs/research')`)
			h.exec(`INSERT INTO artifacts (class, scope, relpath) VALUES ('research','','gone.md')`)
			h.regen()
		}},
		"R4": {seed: func(h *harness) {
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e',1,'V-1','d','DONE')`)
			h.regen()
		}},
		"R5": {seed: func(h *harness) {
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DONE')`)
			h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits)
			        VALUES ('e',1,'S1','d','DONE','deadbeefdeadbeefdeadbeef')`)
			h.regen()
		}},
		"R6": {seed: func(h *harness) {
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','g','ACTIVE','2020-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','DONE')`)
			h.regen()
		}},
		"R7": {seed: func(h *harness) {
			h.exec(`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('a','OPEN','d','d')`)
			h.exec(`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('b','OPEN','d','d')`)
			h.exec(`INSERT INTO task_links (from_task, to_task, type) VALUES (1,2,'duplicates')`)
			h.regen()
		}},
		"R8": {seed: func(h *harness) {
			h.exec(`INSERT INTO events (at, entity, event) VALUES ('d','#999','set-status')`)
			h.regen()
		}},
		"R9": {seed: func(h *harness) {
			h.exec(`UPDATE meta SET value = '5' WHERE key = 'events_archived_through'`)
			h.regen()
		}},
		// R10 has TWO independent triggers, so one epic cannot cover it: an
		// old-dated epic with no stories trips both at once, and the gate
		// would stay green with either predicate deleted. Two epics, each
		// tripping exactly one trigger, and both messages required.
		"R10": {seed: func(h *harness) {
			// (a) homeless but NOT idle: every story terminal, appended just now.
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('homeless','g','ACTIVE','2000-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('homeless','S1','t','DONE')`)
			h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits)
			        VALUES ('homeless',1,'S1',?,'DONE','abc')`, recent)
			// (b) idle but NOT homeless: a PLANNED story is a home, and it is
			// not READY/IN-PROGRESS, so only the idle clause can speak.
			h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('idler','g','ACTIVE','2000-01-01T00:00:00Z')`)
			h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('idler','S1','t','PLANNED')`)
			h.regen()
		}, want: []string{
			"epic homeless has no story that can receive work; new work has no home",
			"epic idler is idle (no active story, no append in 14 days)",
		}},
		"R11": {seed: func(_ *harness) {}}, // a fresh git repo has no chained hooks
		"R12": {seed: func(h *harness) {
			h.exec(`INSERT INTO tasks (title, status, status_note, created_at, updated_at) VALUES ('t','DONE','d','d','d')`)
			h.regen()
		}},
		"R13": {seed: func(h *harness) {
			h.exec(`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t','OPEN','d','d')`)
			h.regen()
		}},
		"R15": {seed: func(h *harness) {
			_ = os.WriteFile(filepath.Join(h.dir, "skip-pending"), []byte(""), 0o600)
		}},
		// All three clauses, and each one's REPAIR text. The repair is the
		// clause's whole point (amendment
		// `r16-reports-only-what-a-verb-can-clear`), so a fixture that only
		// proved the condition half would leave the feature ungated.
		"R16": {seed: func(h *harness) {
			h.exec(`INSERT INTO epics (slug, goal, status, close_sweep, created_at)
			        VALUES ('e','g','CLOSED','2026-01-01','d')`)
			h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','PLANNED')`)
			h.exec(`INSERT INTO tasks (id, title, status, epic, created_at, updated_at)
			        VALUES (1,'t','OPEN','e','d','d')`)
			h.exec(`INSERT INTO epic_criteria (epic, seq, criterion, met) VALUES ('e',1,'c',0)`)
			h.exec(`INSERT INTO events (at, entity, event, detail)
			        VALUES ('d','epic:e','epic','close: 2026-01-01')`)
			h.regen()
		}, want: []string{
			"story S1 is not terminal; repair: selftracked story dissolve e S1 --why TEXT",
			"task #1 (OPEN) is homed here; repair: selftracked edit 1 --detach, or set it terminal",
			"criterion 1 is not met; no verb writes this state here — it arrived by raw SQL",
		}},
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
	// Each fixture must actually make verify emit its rule — and, where the
	// rule has more than one branch, emit every branch's text.
	for _, rule := range wantRules {
		t.Run(rule, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			fx := fixtures[rule]
			fx.seed(h)
			rep := h.verify(false)
			if advisory[rule] {
				mustAdvisory(t, rep, rule)
			} else {
				mustRed(t, rep, rule)
			}
			got := append(rulesNamed(rep.Red, rule), rulesNamed(rep.Advisory, rule)...)
			for _, want := range fx.want {
				if !slices.ContainsFunc(got, func(m string) bool { return strings.Contains(m, want) }) {
					t.Errorf("%s must emit a finding containing %q; got %v", rule, want, got)
				}
			}
		})
	}
}
