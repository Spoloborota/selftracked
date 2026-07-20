package verb

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// primeExec runs write statements on the instance DB (schema posture, FKs on).
func primeExec(t *testing.T, dir string, stmts ...string) {
	t.Helper()
	db, err := schema.Open(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, s := range stmts {
		if _, err := db.ExecContext(context.Background(), s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
}

// buildFromDB runs the DB-only fill steps (everything but fillStale, which
// needs a git working tree) so the contract can be asserted without one.
func buildFromDB(t *testing.T, dir string) primeOutput {
	t.Helper()
	db, err := schema.OpenRead(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	var out primeOutput
	capN := primeCap(ctx, db)
	for _, step := range []func(context.Context, *sql.DB, *primeOutput, int) error{
		fillActiveEpics, fillEpicLists, fillTaskLists, fillSprintGoals, fillDivergence,
	} {
		if err := step(ctx, db, &out, capN); err != nil {
			t.Fatal(err)
		}
	}
	ensureEmpty(&out)
	return out
}

// TestPrimeActiveEpicsShape is INV-458: epics_active carries slug, goal, the
// story tallies (done/in_progress counts, ready/blocked id lists), and
// criteria_unmet as a count — never criterion text.
func TestPrimeActiveEpicsShape(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)
	primeExec(t, dir,
		`INSERT INTO epics (slug, goal, status, created_at) VALUES ('alpha', 'Ship alpha', 'ACTIVE', 'd')`,
		`INSERT INTO stories (epic, id, title, status) VALUES ('alpha','S1','done one','DONE')`,
		`INSERT INTO stories (epic, id, title, status) VALUES ('alpha','S2','wire it','IN-PROGRESS')`,
		`INSERT INTO stories (epic, id, title, status) VALUES ('alpha','S3','next','READY')`,
		`INSERT INTO stories (epic, id, title, status, blocked) VALUES ('alpha','S4','stuck','BLOCKED','waiting')`,
		`INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES ('alpha',1,'builds',1,'ci')`,
		`INSERT INTO epic_criteria (epic, seq, criterion, met) VALUES ('alpha',2,'ships secretly',0)`,
	)
	out := buildFromDB(t, dir)

	if len(out.EpicsActive) != 1 {
		t.Fatalf("want 1 active epic, got %d", len(out.EpicsActive))
	}
	e := out.EpicsActive[0]
	if e.Slug != "alpha" || e.Goal != "Ship alpha" {
		t.Fatalf("epic identity wrong: %+v", e)
	}
	if e.Stories.Done != 1 || e.Stories.InProgress != 1 {
		t.Fatalf("story counts wrong: %+v", e.Stories)
	}
	if !reflect.DeepEqual(e.Stories.Ready, []string{"S3"}) {
		t.Fatalf("ready stories = %v, want [S3]", e.Stories.Ready)
	}
	if !reflect.DeepEqual(e.Stories.Blocked, []string{"S4"}) {
		t.Fatalf("blocked stories = %v, want [S4]", e.Stories.Blocked)
	}
	if e.CriteriaUnmet != 1 {
		t.Fatalf("criteria_unmet = %d, want 1", e.CriteriaUnmet)
	}
	// The criterion text must never appear anywhere in the payload (INV-458).
	if blob := marshal(t, out); strings.Contains(blob, "ships secretly") {
		t.Fatalf("criterion text leaked into prime output:\n%s", blob)
	}
}

// TestPrimeEpicAndTaskLists is INV-459/460/465: slug-only paused/backlog epic
// lists, the ready/triage/in_review task lists (refs, no prose), the parked
// total, and sprint_goals as every IN-PROGRESS story with its title.
func TestPrimeEpicAndTaskLists(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)
	primeExec(t, dir,
		`INSERT INTO epics (slug, goal, status, created_at) VALUES ('alpha','g','ACTIVE','d')`,
		`INSERT INTO epics (slug, goal, status, status_note, created_at) VALUES ('pz','g','PAUSED','later','d')`,
		`INSERT INTO epics (slug, goal, status, created_at) VALUES ('bk','g','BACKLOG','d')`,
		`INSERT INTO stories (epic, id, title, status) VALUES ('alpha','S2','wire it','IN-PROGRESS')`,
		`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('open one','OPEN','d','d')`,
		`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('triage me','NEEDS-TRIAGE','d','d')`,
		`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('review me','IN-REVIEW','d','d')`,
		`INSERT INTO tasks (title, status, parked, created_at, updated_at) VALUES ('parked','OPEN','busy','d','d')`,
	)
	out := buildFromDB(t, dir)

	if !reflect.DeepEqual(out.EpicsPaused, []string{"pz"}) {
		t.Fatalf("epics_paused = %v, want [pz]", out.EpicsPaused)
	}
	if !reflect.DeepEqual(out.EpicsBacklog, []string{"bk"}) {
		t.Fatalf("epics_backlog = %v, want [bk]", out.EpicsBacklog)
	}
	if !reflect.DeepEqual(out.Ready, []string{"#1"}) {
		t.Fatalf("ready = %v, want [#1] (the parked OPEN task is excluded)", out.Ready)
	}
	if !reflect.DeepEqual(out.Triage, []string{"#2"}) {
		t.Fatalf("triage = %v, want [#2]", out.Triage)
	}
	if !reflect.DeepEqual(out.InReview, []string{"#3"}) {
		t.Fatalf("in_review = %v, want [#3]", out.InReview)
	}
	if out.Totals.Parked != 1 {
		t.Fatalf("totals.parked = %d, want 1", out.Totals.Parked)
	}
	if len(out.SprintGoals) != 1 || out.SprintGoals[0] != (sprintGoal{Epic: "alpha", Story: "S2", Title: "wire it"}) {
		t.Fatalf("sprint_goals = %+v, want one (alpha,S2,wire it)", out.SprintGoals)
	}
}

// TestPrimeCaps is INV-466/467/468: the backlog-type lists cap at prime_cap
// in the stated order, totals report the FULL count, and sprint_goals /
// epics_active are never capped.
func TestPrimeCaps(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)
	primeExec(t, dir, `INSERT INTO meta (key, value) VALUES ('prime_cap','2') ON CONFLICT(key) DO UPDATE SET value='2'`)
	// Five NEEDS-TRIAGE tasks; cap is 2.
	for range 5 {
		primeExec(t, dir, `INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t','NEEDS-TRIAGE','d','d')`)
	}
	// Three active epics, each WIP=1 — the uncapped lists must keep all three.
	for _, slug := range []string{"a1", "a2", "a3"} {
		primeExec(t, dir,
			`INSERT INTO epics (slug, goal, status, created_at) VALUES ('`+slug+`','g','ACTIVE','d')`,
			`INSERT INTO stories (epic, id, title, status) VALUES ('`+slug+`','S1','wip','IN-PROGRESS')`,
		)
	}
	out := buildFromDB(t, dir)

	if len(out.Triage) != 2 {
		t.Fatalf("triage capped to %d, want 2", len(out.Triage))
	}
	if out.Totals.Triage != 5 {
		t.Fatalf("totals.triage = %d, want the full 5", out.Totals.Triage)
	}
	// Order: id ASC — the first two ids.
	if !reflect.DeepEqual(out.Triage, []string{"#1", "#2"}) {
		t.Fatalf("triage order = %v, want [#1 #2] (id ASC)", out.Triage)
	}
	if len(out.EpicsActive) != 3 {
		t.Fatalf("epics_active capped to %d — it must never cap (INV-468)", len(out.EpicsActive))
	}
	if len(out.SprintGoals) != 3 {
		t.Fatalf("sprint_goals capped to %d — it must never cap (INV-468)", len(out.SprintGoals))
	}
}

// TestPrimeTotalsAndNoProseScan is INV-461/469/470: totals enumerate every
// capped list plus parked with no duplicate parked scalar, and the ONLY
// free-text fields in the whole payload are epics_active.goal and
// sprint_goals.title.
func TestPrimeTotalsAndNoProseScan(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)
	out := buildFromDB(t, dir)

	// totals{} has exactly the seven contract counts (six capped lists +
	// parked); a struct-field scan catches an accidental extra scalar.
	tv := reflect.TypeFor[primeTotals]()
	wantFields := map[string]bool{
		"ready": true, "triage": true, "in_review": true, "stale": true,
		"epics_paused": true, "epics_backlog": true, "parked": true,
	}
	if tv.NumField() != len(wantFields) {
		t.Fatalf("totals has %d fields, want %d", tv.NumField(), len(wantFields))
	}
	for i := range tv.NumField() {
		tag := tv.Field(i).Tag.Get("json")
		if !wantFields[tag] {
			t.Fatalf("unexpected totals field %q", tag)
		}
	}
	// No top-level "parked" list/scalar duplicates totals.parked.
	var generic map[string]json.RawMessage
	if err := json.Unmarshal([]byte(marshal(t, out)), &generic); err != nil {
		t.Fatal(err)
	}
	if _, dup := generic["parked"]; dup {
		t.Fatal("a top-level parked field duplicates totals.parked (INV-461)")
	}

	// INV-469 as its verification promises: a reflective walk of the whole
	// primeOutput type graph, asserting the complete set of string-bearing
	// json fields is EXACTLY the known set. Adding any new string field —
	// prose or identifier — trips this, forcing a reviewer to classify it and
	// keeping "the only prose is goal/title" a checked property, not a manual
	// observation. The two prose fields (goal, title) are named; every other
	// string here is an identifier/path by construction.
	got := stringJSONFields(reflect.TypeFor[primeOutput]())
	want := map[string]bool{
		// identifiers / paths (not prose):
		"epics_paused": true, "epics_backlog": true, "ready": true,
		"triage": true, "in_review": true, "stale": true, "migrated": true,
		"slug": true, "story": true, "epic": true, "blocked": true,
		// the ONLY two prose fields (INV-469):
		"goal": true, "title": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prime string-field set drifted (INV-469 guard):\n got %v\nwant %v", got, want)
	}
}

// stringJSONFields walks a struct type graph and returns the set of json tag
// names of every string-typed (or []string-typed) leaf field. Recurses into
// nested structs and slices of structs. Used to keep INV-469's "only two
// prose fields" a mechanically-checked property.
func stringJSONFields(t reflect.Type) map[string]bool {
	out := map[string]bool{}
	var walk func(reflect.Type, string)
	walk = func(rt reflect.Type, tag string) {
		//nolint:exhaustive // only String/Slice/Struct matter; default skips the rest
		switch rt.Kind() {
		case reflect.String:
			if tag != "" {
				out[tag] = true
			}
		case reflect.Slice:
			walk(rt.Elem(), tag)
		case reflect.Struct:
			for i := range rt.NumField() {
				f := rt.Field(i)
				name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
				walk(f.Type, name)
			}
		default:
		}
	}
	walk(t, "")
	return out
}

// TestPrimeDumpDivergenceReadOnly is INV-361/462: prime reports
// dump_divergence:true when the tracked dump moved under the DB, and it
// modifies neither the DB nor the dump file (read-only).
func TestPrimeDumpDivergenceReadOnly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)

	// A consistent tracker: no divergence.
	if out := buildFromDB(t, dir); out.DumpDivergence {
		t.Fatal("a consistent tracker must report dump_divergence:false")
	}

	// Move the tracked dump out from under the DB (a pull): append a line so
	// it differs from both the sidecar and the DB's serialization.
	dumpP := filepath.Join(dir, instanceDir, dumpFile)
	orig, err := os.ReadFile(dumpP)
	if err != nil {
		t.Fatal(err)
	}
	moved := append([]byte("-- pulled a newer dump\n"), orig...)
	if err := os.WriteFile(dumpP, moved, 0o644); err != nil {
		t.Fatal(err)
	}
	before := digest(t, dumpP)

	out := buildFromDB(t, dir)
	if !out.DumpDivergence {
		t.Fatal("a moved tracked dump must report dump_divergence:true")
	}
	// Read-only: prime touched neither the dump file nor the DB events.
	if after := digest(t, dumpP); after != before {
		t.Fatal("prime rewrote the tracked dump — it must be read-only (INV-361)")
	}
	if n := eventCount(t, dir); n != 0 {
		t.Fatalf("prime wrote %d events — it must not mutate the DB", n)
	}
}

// TestPrimeDumpRequiresNewerBinary is INV-463: a tracked dump whose header
// schema_version is ahead of this binary sets dump_requires_newer_binary,
// derived by reading the header — no migration engine needed at S8c.
func TestPrimeDumpRequiresNewerBinary(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)

	if out := buildFromDB(t, dir); out.DumpRequiresNewerBinary {
		t.Fatalf("a v%d dump against a v%d binary must not require a newer binary",
			schema.Version, schema.Version)
	}

	// Forge a header one version ahead. divergenceReport will read the same
	// forged dump: we only assert the newer-binary flag here.
	dumpP := filepath.Join(dir, instanceDir, dumpFile)
	forgeHeaderVersion(t, dumpP, schema.Version+1)

	out := buildFromDB(t, dir)
	if !out.DumpRequiresNewerBinary {
		t.Fatal("a header schema_version ahead of the binary must set dump_requires_newer_binary")
	}
}

// TestPrimeEmptyTrackerShape: an empty tracker marshals every list as [], not
// null, so consumers see a stable shape.
func TestPrimeEmptyTrackerShape(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)
	blob := marshal(t, buildFromDB(t, dir))
	for _, list := range []string{
		`"epics_active":[]`, `"epics_paused":[]`, `"epics_backlog":[]`,
		`"ready":[]`, `"triage":[]`, `"in_review":[]`, `"stale":[]`, `"sprint_goals":[]`,
	} {
		if !strings.Contains(blob, list) {
			t.Fatalf("empty list not rendered as []: missing %s in\n%s", list, blob)
		}
	}
	// migrated is omitempty and absent at S8c (nothing migrates).
	if strings.Contains(blob, "migrated") {
		t.Fatalf("migrated must be absent at S8c:\n%s", blob)
	}
}

// TestPrimeStaleList is INV-460/467: stale[] is the git-changed files
// intersected with active non-ephemeral artifact links, path ASC. Needs a
// real git repo with a commit, since fillStale diffs against HEAD.
func TestPrimeStaleList(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	git(t, dir, "init")
	seedInstance(t, dir)

	// A committed artifact file under a non-ephemeral class.
	if err := os.MkdirAll(filepath.Join(dir, "docs", "research"), 0o750); err != nil {
		t.Fatal(err)
	}
	fpath := filepath.Join(dir, "docs", "research", "x.md")
	if err := os.WriteFile(fpath, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "seed")

	// Link the file to a (non-terminal) task via the dictionary + artifact.
	primeExec(t, dir,
		`INSERT INTO path_dictionary (class, scope, root, ephemeral) VALUES ('research','','docs/research',0)`,
		`INSERT INTO artifacts (class, scope, relpath) VALUES ('research','','x.md')`,
		`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('own it','OPEN','d','d')`,
		`INSERT INTO task_artifacts (task, artifact, role) VALUES (1, 1, 'home')`,
	)
	// Modify the committed file so git diff HEAD surfaces it.
	if err := os.WriteFile(fpath, []byte("v2 changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := schema.OpenRead(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var out primeOutput
	if err := fillStale(context.Background(), db, &out, 20); err != nil {
		t.Fatalf("fillStale: %v", err)
	}
	if !reflect.DeepEqual(out.Stale, []string{"docs/research/x.md"}) {
		t.Fatalf("stale = %v, want [docs/research/x.md]", out.Stale)
	}
	if out.Totals.Stale != 1 {
		t.Fatalf("totals.stale = %d, want 1", out.Totals.Stale)
	}
}

// TestPrimeStaleDegradesWithoutGit is the robustness assertion: with no git
// HEAD to diff against, stale degrades to empty and prime does NOT sink — a
// git failure must not make the whole contract unemittable (§11.1 healthy).
func TestPrimeStaleDegradesWithoutGit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	git(t, dir, "init") // a repo, but no commit → HEAD is unborn
	seedInstance(t, dir)

	db, err := schema.OpenRead(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var out primeOutput
	if err := fillStale(context.Background(), db, &out, 20); err != nil {
		t.Fatalf("fillStale must not error on unborn HEAD: %v", err)
	}
	if len(out.Stale) != 0 {
		t.Fatalf("stale = %v, want empty on unborn HEAD", out.Stale)
	}
}

// TestPrimeStaleDegradesOutsideGitRepo covers the other git-failure path a
// critic flagged as untested: a directory that is not a git repository at all
// (not merely a repo with no commits). fillStale must still degrade to empty
// rather than sink prime — the SessionStart surface must emit whenever the DB
// is readable, even where git cannot answer.
func TestPrimeStaleDegradesOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // no git init: `git diff HEAD` fails with "not a git repository"
	seedInstance(t, dir)

	db, err := schema.OpenRead(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var out primeOutput
	if err := fillStale(context.Background(), db, &out, 20); err != nil {
		t.Fatalf("fillStale must not error outside a git repo: %v", err)
	}
	if len(out.Stale) != 0 {
		t.Fatalf("stale = %v, want empty outside a git repo", out.Stale)
	}
}

// ---- helpers ----

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func marshal(t *testing.T, out primeOutput) string {
	t.Helper()
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func digest(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// forgeHeaderVersion rewrites the dump's first-line schema_version to v, then
// heals the sidecar so the ONLY divergence under test is the header version.
func forgeHeaderVersion(t *testing.T, dumpPath string, v int) {
	t.Helper()
	b, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitN(string(b), "\n", 2)
	if len(lines) != 2 {
		t.Fatal("dump has no header line")
	}
	lines[0] = replaceSchemaVersion(t, lines[0], v)
	forged := []byte(strings.Join(lines, "\n"))
	if err := os.WriteFile(dumpPath, forged, 0o644); err != nil {
		t.Fatal(err)
	}
	// Heal the sidecar to the forged bytes so divergenceReport sees a matching
	// hash and the newer-binary flag is isolated from dump_divergence.
	if err := dump.WriteSidecar(filepath.Dir(dumpPath), forged); err != nil {
		t.Fatal(err)
	}
}

func replaceSchemaVersion(t *testing.T, header string, v int) string {
	t.Helper()
	const key = "schema_version="
	before, rest, ok := strings.Cut(header, key)
	if !ok {
		t.Fatalf("header has no %s: %q", key, header)
	}
	_, after, ok := strings.Cut(rest, " ")
	if !ok {
		t.Fatalf("header schema_version not space-terminated: %q", header)
	}
	return before + key + strconv.Itoa(v) + " " + after
}
