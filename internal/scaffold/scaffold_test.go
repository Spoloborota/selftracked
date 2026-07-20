package scaffold

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/schema"
)

var updateGolden = flag.Bool("update-golden", false, "regenerate the init golden files")

const goldenDir = "testdata/golden"

// dbFileRel is the one generated file excluded from golden comparison:
// db.sqlite's bytes are not deterministic across runs.
const dbFileRel = ".selftracked/db.sqlite"

// TestInitGolden regenerates every deterministic artifact `init` writes and
// compares it byte-for-byte to a tracked golden copy. The golden files ARE
// the review surface for what a fresh tracker ships; -update-golden
// regenerates them FROM the code (never hand-edit them to match).
func TestInitGolden(t *testing.T) {
	root := t.TempDir()
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	got := walkFiles(t, root)

	if *updateGolden {
		regenGolden(t, root, got)
		return
	}
	// Every generated file (bar the binary DB) must match its golden.
	for _, rel := range got {
		if rel == dbFileRel {
			continue
		}
		gp := filepath.Join(goldenDir, rel)
		want, err := os.ReadFile(gp)
		if err != nil {
			t.Fatalf("no golden for %s (run: go test ./internal/scaffold -update-golden): %v", rel, err)
		}
		have, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if string(have) != string(want) {
			t.Errorf("%s differs from its golden; regenerate with -update-golden if intended", rel)
		}
	}
	// No orphan goldens: a golden with no matching generated file means init
	// stopped writing something.
	assertNoOrphanGoldens(t, got)
}

func walkFiles(t *testing.T, root string) []string {
	t.Helper()
	var rels []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(rels)
	return rels
}

func regenGolden(t *testing.T, root string, rels []string) {
	t.Helper()
	if err := os.RemoveAll(goldenDir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range rels {
		if rel == dbFileRel {
			continue
		}
		src := filepath.Join(root, rel)
		dst := filepath.Join(goldenDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("regenerated %d golden files under %s", len(rels)-1, goldenDir)
}

func assertNoOrphanGoldens(t *testing.T, got []string) {
	t.Helper()
	present := map[string]bool{}
	for _, r := range got {
		present[r] = true
	}
	err := filepath.WalkDir(goldenDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(goldenDir, p)
		if err != nil {
			return err
		}
		if rel = filepath.ToSlash(rel); !present[rel] {
			t.Errorf("orphan golden %s: init no longer writes it", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestInitSeedsMetaAndPaths covers INV-418 (meta system rows) and INV-064
// (the default path dictionary), read from the DB init built.
func TestInitSeedsMetaAndPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	db, err := schema.Open(filepath.Join(root, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var version, boundary string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='events_archived_through'`).Scan(&boundary); err != nil {
		t.Fatal(err)
	}
	if version == "" || boundary != "0" {
		t.Fatalf("meta seed wrong: schema_version=%q events_archived_through=%q", version, boundary)
	}
	// Assert the SPECIFIC seeded classes, not just a count against the same
	// slice that produced them — and that none of the opt-in classes
	// (INV-404: documented, never pre-registered) leaked into the seed.
	seeded := map[string]int{}
	rows, err := db.Query(`SELECT class, ephemeral FROM path_dictionary`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var class string
		var ephemeral int
		if err := rows.Scan(&class, &ephemeral); err != nil {
			t.Fatal(err)
		}
		seeded[class] = ephemeral
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"research": 0, "adr": 0, "workdir": 1, "run": 1, "report": 0}
	if len(seeded) != len(want) {
		t.Fatalf("seeded classes = %v, want %v", seeded, want)
	}
	for class, eph := range want {
		if got, ok := seeded[class]; !ok || got != eph {
			t.Errorf("class %s: seeded=%v (present=%v), want ephemeral=%d", class, got, ok, eph)
		}
	}
	for _, optIn := range []string{"runbook", "guide", "rfc", "src", "external"} {
		if _, present := seeded[optIn]; present {
			t.Errorf("opt-in class %q must NOT be pre-registered (INV-404)", optIn)
		}
	}
}

// TestInitForcePreservesData covers INV-188's --force: a second init
// refuses without it, and --force REFRESHES rather than rebuilds — a task
// recorded before the re-init must survive (the close review's data-loss
// finding: --force must not silently destroy a populated tracker).
func TestInitForcePreservesData(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx := context.Background()
	if err := writeScaffold(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	execOn(t, root, `INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('keep me','OPEN','d','d')`)

	var ee *existsError
	if err := writeScaffold(ctx, root, false); !errors.As(err, &ee) {
		t.Fatalf("second init without --force must refuse with existsError, got %v", err)
	}
	if err := writeScaffold(ctx, root, true); err != nil {
		t.Fatalf("second init with --force must succeed, got %v", err)
	}
	if n := countTasks(t, root, "keep me"); n != 1 {
		t.Fatalf("--force destroyed tracked data: 'keep me' task count = %d, want 1", n)
	}
}

// TestInitRefusesOnClone covers the CRITICAL guard: a clone has the tracked
// dump.sql but no (gitignored) db.sqlite; init must refuse and point at
// load rather than overwrite the tracked state — even with --force.
func TestInitRefusesOnClone(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, instanceDir), 0o750); err != nil {
		t.Fatal(err)
	}
	const tracked = "-- tracked dump, do not clobber\n"
	writeFile(t, root, filepath.Join(instanceDir, dumpFile), tracked)

	var ce *cloneError
	if err := writeScaffold(context.Background(), root, false); !errors.As(err, &ce) {
		t.Fatalf("init on a clone must refuse with cloneError, got %v", err)
	}
	if err := writeScaffold(context.Background(), root, true); !errors.As(err, &ce) {
		t.Fatalf("--force must NOT override the clone guard, got %v", err)
	}
	if got := readFile(t, root, filepath.Join(instanceDir, dumpFile)); got != tracked {
		t.Fatalf("init clobbered a clone's tracked dump: %q", got)
	}
}

func execOn(t *testing.T, root, query string, args ...any) {
	t.Helper()
	db, err := schema.Open(filepath.Join(root, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func countTasks(t *testing.T, root, title string) int {
	t.Helper()
	db, err := schema.Open(filepath.Join(root, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM tasks WHERE title = ?`, title).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestInitDoesNotClobberAdopterFiles covers the adoption posture: an
// existing AGENTS.md / .claude settings and a populated .gitignore survive.
func TestInitDoesNotClobberAdopterFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "MY OWN AGENTS FILE\n")
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".claude/settings.json", "{\"mine\":true}\n")
	writeFile(t, root, ".gitignore", "node_modules/\n")
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, root, "AGENTS.md"); got != "MY OWN AGENTS FILE\n" {
		t.Fatalf("init clobbered the adopter's AGENTS.md: %q", got)
	}
	if got := readFile(t, root, ".claude/settings.json"); got != "{\"mine\":true}\n" {
		t.Fatalf("init clobbered the adopter's .claude/settings.json: %q", got)
	}
	gi := readFile(t, root, ".gitignore")
	if !strings.Contains(gi, "node_modules/") || !strings.Contains(gi, ".selftracked/db.sqlite") {
		t.Fatalf(".gitignore merge dropped a line: %q", gi)
	}
	// Idempotent merge: a second (forced) init adds no duplicate entries.
	if err := writeScaffold(context.Background(), root, true); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(readFile(t, root, ".gitignore"), ".selftracked/db.sqlite"); n != 1 {
		t.Fatalf("gitignore entry duplicated on re-init: %d occurrences", n)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestPrivacyContentPublishesToDumpAndState covers INV-484: user-authored
// content — here a local path, the documented leak — appears verbatim in
// the published surfaces (dump.sql and STATE.md), which is what the privacy
// warning exists to make agents expect. The append-only worklog/events
// tables mean it also persists; --force below regenerates the surfaces from
// the DB without dropping it.
func TestPrivacyContentPublishesToDumpAndState(t *testing.T) {
	t.Parallel()
	const secret = "/Users/someone/private/notes.md"
	root := t.TempDir()
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	const insTask = `INSERT INTO tasks (title, status, created_at, updated_at) VALUES (?, 'OPEN', 'd', 'd')`
	const insEvent = `INSERT INTO events (at, entity, event, detail) VALUES ('2020-01-01T00:00:00Z','#1','set-status',?)`
	execOn(t, root, insTask, "see "+secret)
	execOn(t, root, insEvent, "ref "+secret)
	// Refresh the derived surfaces from the DB.
	if err := writeScaffold(context.Background(), root, true); err != nil {
		t.Fatal(err)
	}
	if dump := readFile(t, root, filepath.Join(instanceDir, dumpFile)); !strings.Contains(dump, secret) {
		t.Error("INV-484: a local path in a task title is published to dump.sql")
	}
	if st := readFile(t, root, stateFile); !strings.Contains(st, secret) {
		t.Error("INV-484: a local path in an event detail is published to STATE.md")
	}
}

// TestTaskStoryCorrelationNotSchematic covers INV-475: the task↔story
// correspondence is conventional (shared epic + prose), not a schema link —
// the tasks table carries no story reference, so with several blocked
// stories and several IN-REVIEW tasks in one epic nothing in the data model
// says which task answers which story.
func TestTaskStoryCorrelationNotSchematic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	db, err := schema.Open(filepath.Join(root, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(context.Background(), `SELECT name FROM pragma_table_info('tasks')`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name == "story" {
			t.Fatal("INV-475: tasks has a 'story' column — the correlation would be schematic, not conventional")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// TestNoConfigFileReader covers INV-545's second clause: no code path reads
// a config file. Configuration lives only in meta rows edited via `config`;
// a stray config-file reader would reintroduce the file the design forbids.
func TestNoConfigFileReader(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		"config.toml", "config.json", "config.yaml", "config.yml",
		".selftracked/config", "selftracked.toml",
	}
	roots := []string{"../../internal", "../../cmd"}
	for _, r := range roots {
		err := filepath.WalkDir(r, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, tok := range forbidden {
				if strings.Contains(string(b), tok) {
					t.Errorf("INV-545: %s references a config-file path %q — configuration must live in meta only", p, tok)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
