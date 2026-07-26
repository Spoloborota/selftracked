package verb

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/rules"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// farFuture is a moment past every fixture date, so the future bound
// (INV-548) never trips on a resolution-only test that is not about it.
const farFuture = "2030-01-01T00:00:00Z"

// ---- git + instance scaffolding ----

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "importer@test.invalid")
	runGit(t, dir, "config", "user.name", "importer")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commitAt makes an empty commit with a forged author date (and a different
// committer date, so author-over-committer is provable) and returns its sha.
func commitAt(t *testing.T, dir, authorDate, committerDate string) string {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", "c")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+authorDate, "GIT_COMMITTER_DATE="+committerDate)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	return runGit(t, dir, "rev-parse", "HEAD")
}

func newImporter(t *testing.T) *importer {
	t.Helper()
	return &importer{moment: farFuture, legacy: true}
}

// importFile seeds a fresh instance, writes the corpus JSON, and runs the verb.
func importFile(t *testing.T, dir, body string, legacy bool) error {
	t.Helper()
	seedInstance(t, dir)
	return importOnce(t, dir, body, legacy)
}

// importOnce writes the corpus JSON and runs the verb against an
// already-seeded instance — for multi-import tests that build DB state across
// successive imports.
func importOnce(t *testing.T, dir, body string, legacy bool) error {
	t.Helper()
	path := filepath.Join(dir, "corpus.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &cli.Env{Stdout: &strings.Builder{}, Stderr: &strings.Builder{}}
	return runImport(e, path, "", legacy)
}

// writeAndCommit writes a tracked file and commits it with a forged author +
// committer date — for exercising firstCommitDate over real file history.
func writeAndCommit(t *testing.T, dir, name, content, authorDate string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", name)
	cmd := exec.Command("git", "commit", "-q", "-m", "c")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+authorDate, "GIT_COMMITTER_DATE="+authorDate)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
}

func openInstance(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, err := schema.Open(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func queryString(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	var s string
	if err := db.QueryRowContext(context.Background(), q).Scan(&s); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return s
}

// assertDBGreen runs ONLY the DB-only rules (R4 + DBOnly's R6–R9/R12) — not
// the full battery. The round-trip fixture would fail R2 (its path roots are
// not on disk) and R5 (its legacy rows cite no live objects), which are
// filesystem/git rules verify's own runner exercises. The full-verify-green
// evidence for import is internal/cli/testdata/import.txtar, which `mkdir`s
// the roots and cites a real in-script sha before running `selftracked verify`.
func assertDBGreen(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	vs, err := rules.DBOnly(ctx, db)
	if err != nil {
		t.Fatalf("DBOnly: %v", err)
	}
	r4, err := rules.R4(ctx, db)
	if err != nil {
		t.Fatalf("R4: %v", err)
	}
	vs = append(vs, r4...)
	if len(vs) > 0 {
		t.Fatalf("expected green, got violations: %v", vs)
	}
}

// ---- date-priority matrix ----

func TestImportGitFirstPriority(t *testing.T) {
	dir := gitRepo(t)
	early := commitAt(t, dir, "2026-01-01T00:00:00+00:00", "2026-06-01T00:00:00+00:00")
	late := commitAt(t, dir, "2026-01-03T00:00:00+00:00", "2026-06-02T00:00:00+00:00")
	im := newImporter(t)

	t.Run("newest-in-set-wins-author-not-committer", func(t *testing.T) {
		ep := resolvedEpisode{epic: "e", story: "S1", state: statusDone, commits: early + " " + late}
		got, err := im.resolveEpisode(context.Background(), ep)
		if err != nil {
			t.Fatal(err)
		}
		if got.source != srcGit || got.date[:dayLen] != "2026-01-03" {
			t.Fatalf("want git 2026-01-03, got source=%s date=%s", got.source, got.date)
		}
	})

	t.Run("range-dates-by-finish", func(t *testing.T) {
		ep := resolvedEpisode{epic: "e", story: "S1", state: statusDone, commits: early + ".." + late}
		got, err := im.resolveEpisode(context.Background(), ep)
		if err != nil {
			t.Fatal(err)
		}
		if got.date[:dayLen] != "2026-01-03" {
			t.Fatalf("range should date by finish 2026-01-03, got %s", got.date)
		}
	})

	t.Run("author-not-committer-for-single", func(t *testing.T) {
		ep := resolvedEpisode{epic: "e", story: "S1", state: statusDone, commits: early}
		got, err := im.resolveEpisode(context.Background(), ep)
		if err != nil {
			t.Fatal(err)
		}
		if got.date[:dayLen] != "2026-01-01" {
			t.Fatalf("want author 2026-01-01, got %s (committer must not win)", got.date)
		}
	})
}

func TestImportCalendarDayDisagreement(t *testing.T) {
	dir := gitRepo(t)
	sha := commitAt(t, dir, "2026-01-02T09:00:00+00:00", "2026-01-02T09:00:00+00:00")

	t.Run("disagree-warns-git-wins-both-recorded", func(t *testing.T) {
		im := newImporter(t)
		ep := resolvedEpisode{epic: "e", story: "S1", state: statusDone, commits: sha, date: "2026-01-03"}
		got, err := im.resolveEpisode(context.Background(), ep)
		if err != nil {
			t.Fatal(err)
		}
		if got.source != srcGit || got.date[:dayLen] != "2026-01-02" {
			t.Fatalf("git must win: source=%s date=%s", got.source, got.date)
		}
		if len(im.warnings) != 1 {
			t.Fatalf("want one warning, got %v", im.warnings)
		}
		if !strings.Contains(got.note, "explicit date 2026-01-03 disagreed") ||
			!strings.Contains(got.note, "git 2026-01-02 used") {
			t.Fatalf("both dates must be recorded in note, got %q", got.note)
		}
	})

	t.Run("same-day-silent", func(t *testing.T) {
		im := newImporter(t)
		ep := resolvedEpisode{epic: "e", story: "S1", state: statusDone, commits: sha, date: "2026-01-02"}
		got, err := im.resolveEpisode(context.Background(), ep)
		if err != nil {
			t.Fatal(err)
		}
		if len(im.warnings) != 0 || got.note != "" {
			t.Fatalf("same calendar day must be silent, warnings=%v note=%q", im.warnings, got.note)
		}
	})
}

func TestImportPlaceholderFallthrough(t *testing.T) {
	gitRepo(t)
	im := newImporter(t)
	// "see commit" is prose: no sha-shaped token, resolves nothing (INV-262),
	// and a DONE row without a range becomes legacy: under --legacy.
	ep := resolvedEpisode{epic: "e", story: "S1", state: statusDone, commits: "see commit", legacyReason: "no range"}
	got, err := im.resolveEpisode(context.Background(), ep)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.commits, legacyPrefix) {
		t.Fatalf("placeholder DONE row should fall through to legacy:, got %q", got.commits)
	}
	if got.source == srcGit {
		t.Fatalf("a placeholder contributes no git date, got source %s", got.source)
	}
}

// INV-266: the importer records the author date git SHOWS, which after a
// rebase/squash is the rewritten author date, not the original. This commit's
// author date (2026-02-02) and committer date (2026-09-09) differ, standing in
// for a rewrite; the importer takes the author date — proving both that
// committer time never leaks in AND that a rewritten author time is what gets
// recorded (the honest limitation: we cannot recover a pre-rewrite date git no
// longer holds).
func TestImportBestEffortAuthorDate(t *testing.T) {
	dir := gitRepo(t)
	rewritten := commitAt(t, dir, "2026-02-02T00:00:00+00:00", "2026-09-09T00:00:00+00:00")
	im := newImporter(t)
	ep := resolvedEpisode{epic: "e", story: "S1", state: statusDone, commits: rewritten}
	got, err := im.resolveEpisode(context.Background(), ep)
	if err != nil {
		t.Fatal(err)
	}
	if got.date[:dayLen] != "2026-02-02" {
		t.Fatalf("importer must record the (rewritten) author date git shows, 2026-02-02, got %s", got.date)
	}
	if got.date[:dayLen] == "2026-09-09" {
		t.Fatal("committer date must never be recorded as the worklog date")
	}
}

// ---- task dates (INV-267/268) ----

func TestImportTaskDateRegime(t *testing.T) {
	gitRepo(t)
	im := newImporter(t)

	got, err := im.resolveTasks([]taskRowIn{
		{Title: "explicit", Status: statusDone, Note: "closed: x", Date: "2026-03-03"},
		{Title: "synthesized", Status: statusOpen},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].source != srcExplicit || got[0].date[:dayLen] != "2026-03-03" {
		t.Fatalf("explicit task date: source=%s date=%s", got[0].source, got[0].date)
	}
	if got[1].source != srcImport || got[1].date != farFuture {
		t.Fatalf("dateless task takes the import moment: source=%s date=%s", got[1].source, got[1].date)
	}
}

// ---- structural obligations, driven through the full verb ----

func TestImportStructuralAndLegacy(t *testing.T) {
	roundTrip := `{
      "paths": [
        {"class":"src","scope":"web","root":"web/src"},
        {"class":"src","scope":"api","root":"api/src"}
      ],
      "epics": [
        {"slug":"done-epic","goal":"g","status":"CLOSED","close_sweep":"2026-01-05",
         "criteria":[{"criterion":"owner attests","met":true,"evidence":"noted"}]},
        {"slug":"live-epic","goal":"g2"},
        {"slug":"gone-epic","goal":"g3","status":"DISSOLVED","status_note":"abandoned"}
      ],
      "stories": [
        {"epic":"done-epic","id":"S1","title":"t","status":"DONE"},
        {"epic":"done-epic","id":"S2","title":"t","status":"DONE"}
      ],
      "tasks": [
        {"title":"a done task","status":"DONE","note":"closed: shipped","epic":"done-epic"},
        {"title":"a wontdo task","status":"WONT-DO","note":"declined","date":"2026-02-02"},
        {"title":"future inc","status":"OPEN","epic":"live-epic","future_increment":true},
        {"title":"pointered","status":"OPEN","pointer_note":"a worklog row","owner_steer":"PO said go"}
      ],
      "worklog": [
        {"epic":"done-epic","story":"S1","state":"DONE","date":"2026-01-03",
         "increments":[{"commits":"legacy: part 1","note":"p1"},{"commits":"legacy: part 2","note":"p2"}]},
        {"epic":"done-epic","story":"S2","state":"DONE","commits":"legacy: whole"},
        {"epic":"done-epic","story":"V-1","state":"IN-PROGRESS","date":"2026-01-06"},
        {"epic":"live-epic","story":"S9","state":"IN-PROGRESS","date":"2026-01-02"}
      ]
    }`

	t.Run("legacy-round-trips-green", func(t *testing.T) {
		dir := gitRepo(t)
		if err := importFile(t, dir, roundTrip, true); err != nil {
			t.Fatalf("import: %v", err)
		}
		db := openInstance(t, dir)
		assertDBGreen(t, db)
		assertStructural(t, db)
	})

	t.Run("without-legacy-refused", func(t *testing.T) {
		dir := gitRepo(t)
		err := importFile(t, dir, roundTrip, false)
		if err == nil {
			t.Fatal("a corpus of terminal + synthesized rows must be refused without --legacy")
		}
	})
}

func assertStructural(t *testing.T, db *sql.DB) {
	t.Helper()
	// INV-438: two scopes of one class → two path rows.
	if n := queryString(t, db, `SELECT COUNT(*) FROM path_dictionary WHERE class='src'`); n != "2" {
		t.Fatalf("INV-438: want 2 src path rows, got %s", n)
	}
	// INV-444: the bundled increments split into two DONE rows for S1.
	if n := queryString(t, db,
		`SELECT COUNT(*) FROM worklog WHERE epic='done-epic' AND story='S1' AND state='DONE'`); n != "2" {
		t.Fatalf("INV-444: want 2 split increment rows, got %s", n)
	}
	// INV-269/443: done-epic seq is contiguous ascending by derived date —
	// the two 2026-01-03 increments, then V-1 (2026-01-06), then S2 (dateless
	// → import-time, newest).
	order := queryString(t, db,
		`SELECT group_concat(story,',') FROM (SELECT story FROM worklog WHERE epic='done-epic' ORDER BY seq)`)
	if order != "S1,S1,V-1,S2" {
		t.Fatalf("INV-269/443: want S1,S1,V-1,S2 by date, got %s", order)
	}
	// INV-270/445: S9 materialized PLANNED; V-1 exempt (no story row).
	if got := queryString(t, db, `SELECT status FROM stories WHERE epic='live-epic' AND id='S9'`); got != storyPlanned {
		t.Fatalf("INV-270: S9 should be materialized PLANNED, got %s", got)
	}
	if n := queryString(t, db, `SELECT COUNT(*) FROM stories WHERE epic='done-epic' AND id='V-1'`); n != "0" {
		t.Fatalf("INV-445: a V-row must not materialize a story, got %s", n)
	}
	// INV-448: future increment homed AND not parked — asserted distinctly.
	if got := queryString(t, db, `SELECT epic FROM tasks WHERE title='future inc'`); got != "live-epic" {
		t.Fatalf("INV-448: future increment must be homed to its epic, got %q", got)
	}
	if got := queryString(t, db, `SELECT parked FROM tasks WHERE title='future inc'`); got != "" {
		t.Fatalf("INV-448: future increment must not be parked, got %q", got)
	}
	// INV-441/A10: legacy commits are stored in the canonical `legacy: <why>`
	// form (one space after the colon), which R5 skips and R6 accepts.
	if got := queryString(t, db,
		`SELECT commits FROM worklog WHERE epic='done-epic' AND story='S2' AND state='DONE'`); got != "legacy: whole" {
		t.Fatalf("INV-441: legacy commits must round-trip verbatim canonical, got %q", got)
	}
	if got := queryString(t, db,
		`SELECT group_concat(commits, '|') FROM (SELECT commits FROM worklog
		 WHERE epic='done-epic' AND story='S1' AND state='DONE' ORDER BY seq)`); got != "legacy: part 1|legacy: part 2" {
		t.Fatalf("INV-441: split legacy increments must store canonical legacy:, got %q", got)
	}
	// INV-451/452: pointer + owner steer folded into the note.
	note := queryString(t, db, `SELECT status_note FROM tasks WHERE title='pointered'`)
	if !strings.Contains(note, "[pointer:") || !strings.Contains(note, "[owner steer:") {
		t.Fatalf("INV-451/452: pointer and steer must land as note, got %q", note)
	}
	// INV-452: no synthesized unblock/story event.
	if n := queryString(t, db, `SELECT COUNT(*) FROM events WHERE event='story'`); n != "0" {
		t.Fatalf("INV-452: import must synthesize no story events, got %s", n)
	}
	assertEventsPerTerminal(t, db)
}

// INV-442/446/447/265/440: every imported task gets an import events row (the
// terminal ones R12 requires, plus the INV-056/440 backfill marks), and each
// worklog-bearing epic's row carries the per-row date-source map (never a flat
// batch stamp).
func assertEventsPerTerminal(t *testing.T, db *sql.DB) {
	t.Helper()
	// A5/INV-440: all four imported tasks (2 terminal, 2 backfilled OPEN) get
	// an import events row so a backdated OPEN task is distinguishable from a
	// genuine old one.
	if n := queryString(t, db,
		`SELECT COUNT(*) FROM events WHERE event='import' AND entity LIKE '#%'`); n != "4" {
		t.Fatalf("A5: want 4 imported-task import events, got %s", n)
	}
	// INV-446: the two terminal tasks (DONE, WONT-DO) are among them — the
	// R12 trail. R12 (run under assertDBGreen) enforces the terminal coverage.
	if n := queryString(t, db,
		`SELECT COUNT(DISTINCT e.entity) FROM events e JOIN tasks t ON e.entity = '#' || t.id
		 WHERE e.event='import' AND t.status IN ('DONE','WONT-DO','DUPLICATE')`); n != "2" {
		t.Fatalf("INV-446: want 2 terminal tasks carrying an import trail, got %s", n)
	}
	// INV-446: one epic import row each for done-epic (terminal + worklog),
	// gone-epic (terminal, no worklog) and live-epic (non-terminal + worklog).
	if n := queryString(t, db,
		`SELECT COUNT(*) FROM events WHERE event='import' AND entity LIKE 'epic:%'`); n != "3" {
		t.Fatalf("INV-446: want 3 epic import events, got %s", n)
	}
	// INV-447/265/440: the source map is the per-row seq:source enumeration,
	// mixing explicit (e) and synthesized (i) — proving it is not one flat
	// stamp and that synthesized timestamps are events-marked.
	if got := queryString(t, db,
		`SELECT detail FROM events WHERE event='import' AND entity='epic:done-epic'`); got != "1:e 2:e 3:e 4:i" {
		t.Fatalf("INV-447/265: done-epic source map mismatch, got %q", got)
	}
	if got := queryString(t, db,
		`SELECT detail FROM events WHERE event='import' AND entity='epic:live-epic'`); got != "1:e" {
		t.Fatalf("INV-447: live-epic source map mismatch, got %q", got)
	}
}

// ---- md-table parser ----

func TestImportMdTableParser(t *testing.T) {
	doc := "## paths\n" +
		"| class | scope | root | ephemeral |\n" +
		"| --- | --- | --- | --- |\n" +
		"| src | web | web/src | false |\n" +
		"\n## tasks\n" +
		"| title | status | note |\n" +
		"| --- | --- | --- |\n" +
		"| a task | OPEN | hello |\n"
	c, err := parseMdTable([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Paths) != 1 || c.Paths[0].Class != "src" || c.Paths[0].Scope != "web" {
		t.Fatalf("md-table paths: %+v", c.Paths)
	}
	if len(c.Tasks) != 1 || c.Tasks[0].Title != "a task" || c.Tasks[0].Status != statusOpen {
		t.Fatalf("md-table tasks: %+v", c.Tasks)
	}

	bad := "## tasks\n| title | bogus |\n| --- | --- |\n| t | x |\n"
	if _, err := parseMdTable([]byte(bad)); err == nil {
		t.Fatal("an unknown column must be refused")
	}
}

// A9: every md-table malformation is a loud refuse, never a silent drop.
func TestImportMdTableRefusals(t *testing.T) {
	cases := []struct{ name, doc, want string }{
		{"missing-separator", "## tasks\n| title | status |\n", "separator"},
		{"bad-separator", "## tasks\n| title | status |\n| xxx | yyy |\n| a | OPEN |\n", "separator"},
		{"duplicate-section", "## tasks\n| title |\n| --- |\n| a |\n\n## tasks\n| title |\n| --- |\n| b |\n", "duplicate"},
		{"cell-count", "## tasks\n| title | status |\n| --- | --- |\n| only-one |\n", "cells"},
		{"unknown-section", "## bogus\n| a |\n| --- |\n| x |\n", "unknown section"},
		{"unknown-section-empty", "## bogus\n", "unknown section"},
		{
			"bad-dup-of",
			"## tasks\n| title | status | dup_of |\n| --- | --- | --- |\n| a | DUPLICATE | notanumber |\n", "dup_of",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMdTable([]byte(tc.doc))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s: want a refuse mentioning %q, got %v", tc.name, tc.want, err)
			}
		})
	}
}

// The empty-corpus refusal quotes `corpusSections` back at the author, so a
// section the readers accept and that list omits would be a message telling
// someone their correct heading is not a heading. The list is ordered (the
// message needs a stable order) and `knownColumns` is not, so the two are
// compared as sets here rather than the message deriving one from the other.
func TestCorpusSectionsMatchColumns(t *testing.T) {
	t.Parallel()
	if len(corpusSections) != len(knownColumns) {
		t.Fatalf("corpusSections has %d entries, knownColumns has %d", len(corpusSections), len(knownColumns))
	}
	for _, s := range corpusSections {
		if _, ok := knownColumns[s]; !ok {
			t.Errorf("corpusSections names %q, which no reader accepts", s)
		}
	}
}

// The two empty-corpus reasons are indistinguishable from the parsed rows —
// both corpora are zero-valued — so this pins the discriminator itself over
// both readers, at the unit level the txtar fixture cannot reach.
func TestRecognizedSections(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, reader, doc string
		want              int
	}{
		{"md-bare-table", formatMdTable, "| a | b |\n| --- | --- |\n| 1 | 2 |\n", 0},
		{"md-prose-only", formatMdTable, "just prose, no heading and no table\n", 0},
		{"md-empty-section", formatMdTable, "## tasks\n| title |\n| --- |\n", 1},
		{"md-two-sections", formatMdTable, "## tasks\n\n## paths\n", 2},
		{"json-empty-object", formatJSON, "{}", 0},
		{"json-empty-array", formatJSON, `{"tasks":[]}`, 1},
		{"json-two-keys", formatJSON, `{"tasks":[],"paths":[]}`, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := recognizedSections([]byte(tc.doc), tc.reader); got != tc.want {
				t.Fatalf("recognizedSections(%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// ---- determinism (INV-264) ----

func TestImportSourceMapDeterministic(t *testing.T) {
	dir := gitRepo(t)
	sha := commitAt(t, dir, "2026-01-02T00:00:00+00:00", "2026-01-02T00:00:00+00:00")
	body := `{
      "epics":[{"slug":"e","goal":"g","status":"CLOSED","close_sweep":"2026-01-09"}],
      "stories":[{"epic":"e","id":"S1","title":"t","status":"DONE"}],
      "worklog":[
        {"epic":"e","story":"S1","state":"IN-PROGRESS","date":"2026-01-01"},
        {"epic":"e","story":"S1","state":"DONE","commits":"` + sha + `"}
      ]
    }`
	detail1 := importForDetail(t, dir, body)

	// A second repo with the identical forged commit yields the identical git
	// object id (empty commit, same identity/date/message), so the cited sha
	// resolves there too and the second import derives the same map.
	dir2 := gitRepo(t)
	sha2 := commitAt(t, dir2, "2026-01-02T00:00:00+00:00", "2026-01-02T00:00:00+00:00")
	if sha2 != sha {
		t.Fatalf("forged empty commit should be reproducible: %s vs %s", sha, sha2)
	}
	detail2 := importForDetail(t, dir2, body)
	if detail1 != detail2 {
		t.Fatalf("source map must be deterministic: %q vs %q", detail1, detail2)
	}
	if detail1 != "1:e 2:g" {
		t.Fatalf("INV-265: want the per-row map 1:e 2:g, got %q", detail1)
	}
}

func importForDetail(t *testing.T, dir, body string) string {
	t.Helper()
	if err := importFile(t, dir, body, true); err != nil {
		t.Fatalf("import: %v", err)
	}
	db := openInstance(t, dir)
	return queryString(t, db, `SELECT detail FROM events WHERE event='import' AND entity='epic:e'`)
}

// ---- future bound (INV-548) at the Go level ----

func TestImportFutureDateRefused(t *testing.T) {
	dir := gitRepo(t)
	body := `{"tasks":[{"title":"tomorrow","status":"OPEN","date":"2999-01-01"}]}`
	err := importFile(t, dir, body, true)
	if err == nil || !strings.Contains(err.Error(), "future") {
		t.Fatalf("a future-dated row must be refused naming the future bound, got %v", err)
	}
}

// ---- A4/T1: a sha-shaped typo is stored verbatim (R5 flags it end-to-end
// in internal/cli/testdata/import-typo.txtar — a cross-package verify call
// here would cycle verify→load→verb) ----

func TestImportShaTypoStoredVerbatim(t *testing.T) {
	dir := gitRepo(t)
	commitAt(t, dir, "2026-01-01T00:00:00+00:00", "2026-01-01T00:00:00+00:00")
	body := `{
      "epics":[{"slug":"e","goal":"g"}],
      "stories":[{"epic":"e","id":"S1","title":"t"}],
      "worklog":[{"epic":"e","story":"S1","state":"IN-PROGRESS","commits":"abcdef1","date":"2026-01-02"}]
    }`
	if err := importFile(t, dir, body, true); err != nil {
		t.Fatalf("import: %v", err)
	}
	db := openInstance(t, dir)
	// A lone unresolvable sha-shaped token is a typo kept verbatim (not
	// dropped, not rewritten) so the commit rule R5 can flag it (§6.2, INV-262).
	if got := queryString(t, db, `SELECT commits FROM worklog WHERE epic='e'`); got != "abcdef1" {
		t.Fatalf("T1: a sha-shaped typo must be stored verbatim, got %q", got)
	}
}

// ---- A10/T4: legacy: is stored in one canonical space-normalized form ----

func TestImportLegacyCommitsNormalized(t *testing.T) {
	gitRepo(t)
	im := newImporter(t)
	// No space after the colon → normalized to the canonical "legacy: ".
	ep := resolvedEpisode{epic: "e", story: "S1", state: statusDone, commits: "legacy:no-space"}
	got, err := im.resolveEpisode(context.Background(), ep)
	if err != nil {
		t.Fatal(err)
	}
	if got.commits != "legacy: no-space" {
		t.Fatalf("A10: legacy: prefix must normalize to one space, got %q", got.commits)
	}
}

// ---- T3: each without-legacy relaxation is refused on its own ----

func TestImportWithoutLegacyPerRelaxation(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{
			"terminal-task",
			`{"tasks":[{"title":"t","status":"DONE","note":"closed: x"}]}`, "terminal",
		},
		{
			"terminal-epic",
			`{"epics":[{"slug":"e","goal":"g","status":"DISSOLVED","status_note":"x"}]}`, "terminal",
		},
		{
			"terminal-story",
			`{"epics":[{"slug":"e","goal":"g"}],"stories":[{"epic":"e","id":"S1","title":"t","status":"DONE"}]}`, "terminal",
		},
		{
			"legacy-commits",
			`{"epics":[{"slug":"e","goal":"g"}],"stories":[{"epic":"e","id":"S1","title":"t"}],` +
				`"worklog":[{"epic":"e","story":"S1","state":"IN-PROGRESS","commits":"legacy: foo"}]}`, "legacy: commits",
		},
		{
			"synthesized-date",
			`{"epics":[{"slug":"e","goal":"g"}],"stories":[{"epic":"e","id":"S1","title":"t"}],` +
				`"worklog":[{"epic":"e","story":"S1","state":"IN-PROGRESS"}]}`, "synthesized import-time date",
		},
		// An EXPLICIT date is NOT a --legacy relaxation (§6.2/RC-1); its
		// admission is proven by TestImportExplicitDateWithoutLegacy below.
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := gitRepo(t)
			err := importFile(t, dir, tc.body, false)
			if err == nil {
				t.Fatalf("%s: must be refused without --legacy", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s: refusal should mention %q, got %v", tc.name, tc.want, err)
			}
		})
	}
}

// ---- RC-1: an explicit date is admitted without --legacy (only a
// synthesized import-time date is a relaxation), and it is still
// events-marked so it cannot be a hidden forgery ----

func TestImportExplicitDateWithoutLegacy(t *testing.T) {
	dir := gitRepo(t)
	if err := importFile(t, dir,
		`{"tasks":[{"title":"explicit","status":"OPEN","date":"2026-01-02"}]}`, false); err != nil {
		t.Fatalf("an explicit-dated task must import without --legacy, got %v", err)
	}
	db := openInstance(t, dir)
	if got := queryString(t, db, `SELECT created_at FROM tasks WHERE title='explicit'`); got[:dayLen] != "2026-01-02" {
		t.Fatalf("RC-1: the explicit date must be recorded, got %s", got)
	}
	// The backfill mark (A5) survives: an explicit-dated task still carries an
	// import events row, which is what keeps it from being a hidden forgery.
	if n := queryString(t, db,
		`SELECT COUNT(*) FROM events e JOIN tasks t ON e.entity='#'||t.id
		 WHERE e.event='import' AND t.title='explicit'`); n != "1" {
		t.Fatalf("RC-1: an explicit-dated task must still be events-marked, got %s", n)
	}
}

func TestImportDatelessTaskNeedsLegacy(t *testing.T) {
	dir := gitRepo(t)
	// A dateless task falls to the synthesized import moment, which needs --legacy.
	err := importFile(t, dir, `{"tasks":[{"title":"dateless","status":"OPEN"}]}`, false)
	if err == nil || !strings.Contains(err.Error(), "synthesized import-time date") {
		t.Fatalf("RC-1: a dateless task must be refused without --legacy, got %v", err)
	}
}

// ---- RC-3: a re-imported epic slug or (class,scope) path refuses cleanly ----

func TestImportExistingEpicAndPathRefuse(t *testing.T) {
	dir := gitRepo(t)
	seedInstance(t, dir)
	if err := importOnce(t, dir,
		`{"paths":[{"class":"src","scope":"web","root":"web/src"}],"epics":[{"slug":"e","goal":"g"}]}`,
		false); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if err := importOnce(t, dir, `{"epics":[{"slug":"e","goal":"g2"}]}`, false); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("RC-3: re-importing an epic slug must refuse cleanly, got %v", err)
	}
	if err := importOnce(t, dir,
		`{"paths":[{"class":"src","scope":"web","root":"web/src"}]}`, false); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("RC-3: re-importing a (class,scope) path must refuse cleanly, got %v", err)
	}
}

// ---- T2: a single summary events row leaves other terminal tasks R12-red ----

func TestImportEventsPerTerminalNegative(t *testing.T) {
	dir := gitRepo(t)
	seedInstance(t, dir)
	db := openInstance(t, dir)
	ctx := context.Background()
	for _, ins := range []string{
		`INSERT INTO tasks (title,status,status_note,created_at,updated_at)
		   VALUES ('a','DONE','done x','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`,
		`INSERT INTO tasks (title,status,status_note,created_at,updated_at)
		   VALUES ('b','DONE','done y','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`,
		`INSERT INTO events (at,entity,event,detail)
		   VALUES ('2026-01-01T00:00:00Z','#1','import','import DONE (date:i)')`,
	} {
		if _, err := db.ExecContext(ctx, ins); err != nil {
			t.Fatal(err)
		}
	}
	vs, err := rules.DBOnly(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	var r12 []string
	for _, v := range vs {
		if v.Rule == "R12" {
			r12 = append(r12, v.Message)
		}
	}
	// INV-446 negative: the lone summary row covers #1; #2 stays red — a
	// single aggregated events row is NOT sufficient for a batch of terminals.
	if len(r12) != 1 || !strings.Contains(r12[0], "#2") {
		t.Fatalf("R12 must report the uncovered terminal task #2 and only it, got %v", r12)
	}
}

// ---- A1/T7: a DUPLICATE import writes the duplicates link (R7 green) ----

func TestImportDuplicateRoundTrip(t *testing.T) {
	dir := gitRepo(t)
	seedInstance(t, dir)
	// The canonical task first (id 1); its backfilled import date needs --legacy.
	if err := importOnce(t, dir, `{"tasks":[{"title":"canonical","status":"OPEN"}]}`, true); err != nil {
		t.Fatalf("seed canonical: %v", err)
	}
	// A DUPLICATE pointing at it (id 2).
	if err := importOnce(t, dir,
		`{"tasks":[{"title":"dupe","status":"DUPLICATE","note":"dup of 1","dup_of":1}]}`, true); err != nil {
		t.Fatalf("import duplicate: %v", err)
	}
	db := openInstance(t, dir)
	assertDBGreen(t, db) // R7 green only because the duplicates link was written
	if n := queryString(t, db,
		`SELECT COUNT(*) FROM task_links WHERE type='duplicates' AND from_task=2 AND to_task=1`); n != "1" {
		t.Fatalf("A1: DUPLICATE import must write the paired duplicates link, got %s", n)
	}
	// A dup_of at a nonexistent task, and at a DUPLICATE (a chain), are clean refusals.
	if err := importOnce(t, dir,
		`{"tasks":[{"title":"x","status":"DUPLICATE","note":"n","dup_of":99}]}`, true); err == nil ||
		!strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("A1: dup_of at a missing task must refuse cleanly, got %v", err)
	}
	if err := importOnce(t, dir,
		`{"tasks":[{"title":"y","status":"DUPLICATE","note":"n","dup_of":2}]}`, true); err == nil ||
		!strings.Contains(err.Error(), "DUPLICATE") {
		t.Fatalf("A1: dup_of at a DUPLICATE (chain) must refuse cleanly, got %v", err)
	}
}

// ---- A6/T8: epics.created_at is deterministic and never postdates close_sweep ----

func TestImportEpicCreatedAtDeterministic(t *testing.T) {
	body := `{
      "epics":[{"slug":"e","goal":"g","status":"CLOSED","close_sweep":"2026-01-05"}],
      "stories":[{"epic":"e","id":"S1","title":"t","status":"DONE"}],
      "worklog":[
        {"epic":"e","story":"S1","state":"IN-PROGRESS","date":"2026-01-02"},
        {"epic":"e","story":"S1","state":"DONE","commits":"legacy: whole","date":"2026-01-03"}
      ]
    }`
	dir1 := gitRepo(t)
	if err := importFile(t, dir1, body, true); err != nil {
		t.Fatalf("import: %v", err)
	}
	db1 := openInstance(t, dir1)
	created1 := queryString(t, db1, `SELECT created_at FROM epics WHERE slug='e'`)
	sweep := queryString(t, db1, `SELECT close_sweep FROM epics WHERE slug='e'`)
	if created1[:dayLen] > sweep {
		t.Fatalf("A6: created_at %s must not postdate close_sweep %s", created1, sweep)
	}
	if created1[:dayLen] != "2026-01-02" {
		t.Fatalf("A6: created_at should be the earliest worklog date 2026-01-02, got %s", created1)
	}
	dir2 := gitRepo(t)
	if err := importFile(t, dir2, body, true); err != nil {
		t.Fatalf("import: %v", err)
	}
	db2 := openInstance(t, dir2)
	if created2 := queryString(t, db2, `SELECT created_at FROM epics WHERE slug='e'`); created1 != created2 {
		t.Fatalf("A6: created_at must be deterministic across imports, got %q vs %q", created1, created2)
	}
}

// RC-2: a CLOSED epic with NO worklog anchors created_at to close_sweep (not
// the import moment), so it never postdates its own close — deterministically.
func TestImportClosedEpicNoWorklogCreatedAt(t *testing.T) {
	body := `{"epics":[{"slug":"e","goal":"g","status":"CLOSED","close_sweep":"2026-01-05"}]}`
	dir1 := gitRepo(t)
	if err := importFile(t, dir1, body, true); err != nil {
		t.Fatalf("import: %v", err)
	}
	db1 := openInstance(t, dir1)
	created1 := queryString(t, db1, `SELECT created_at FROM epics WHERE slug='e'`)
	sweep := queryString(t, db1, `SELECT close_sweep FROM epics WHERE slug='e'`)
	if created1[:dayLen] != sweep {
		t.Fatalf("RC-2: created_at must come from close_sweep %s, got %s", sweep, created1)
	}
	dir2 := gitRepo(t)
	if err := importFile(t, dir2, body, true); err != nil {
		t.Fatalf("import: %v", err)
	}
	db2 := openInstance(t, dir2)
	if created2 := queryString(t, db2, `SELECT created_at FROM epics WHERE slug='e'`); created1 != created2 {
		t.Fatalf("RC-2: created_at must be deterministic, got %q vs %q", created1, created2)
	}
}

// ---- A3/T9: firstCommitDate takes the MIN over non-monotonic history ----

func TestImportFirstCommitNonMonotonic(t *testing.T) {
	dir := gitRepo(t)
	// A later (HEAD) commit carries an EARLIER forged author date than its
	// parent — git log lists it first, so a last-line reading is wrong.
	writeAndCommit(t, dir, "f", "one", "2026-03-03T00:00:00+00:00")
	writeAndCommit(t, dir, "f", "two", "2026-01-01T00:00:00+00:00")
	got, ok := firstCommitDate(context.Background(), "f")
	if !ok {
		t.Fatal("expected a bound for a tracked file")
	}
	if got[:dayLen] != "2026-01-01" {
		t.Fatalf("A3: the bound must be the MIN author date 2026-01-01, got %s", got)
	}
}

// ---- A7/T10: re-import of a worklog row onto an already-live story ----

func TestImportReimportOntoExistingStory(t *testing.T) {
	dir := gitRepo(t)
	seedInstance(t, dir)
	// Seed a live epic + story through a first (non-legacy) import.
	if err := importOnce(t, dir,
		`{"epics":[{"slug":"e","goal":"g"}],"stories":[{"epic":"e","id":"S1","title":"t","status":"READY"}]}`,
		false); err != nil {
		t.Fatalf("seed epic+story: %v", err)
	}
	// Backfilling a worklog episode onto the existing story must not crash.
	if err := importOnce(t, dir,
		`{"worklog":[{"epic":"e","story":"S1","state":"IN-PROGRESS","date":"2026-01-02"}]}`, true); err != nil {
		t.Fatalf("A7: re-import onto an existing story must succeed, got %v", err)
	}
	db := openInstance(t, dir)
	if n := queryString(t, db, `SELECT COUNT(*) FROM worklog WHERE epic='e' AND story='S1'`); n != "1" {
		t.Fatalf("A7: the backfilled episode must land, got %s row(s)", n)
	}
	if n := queryString(t, db, `SELECT COUNT(*) FROM stories WHERE epic='e' AND id='S1'`); n != "1" {
		t.Fatalf("A7: the existing story must not be duplicated, got %s", n)
	}
	// An EXPLICIT stories[] row that collides with the live story refuses cleanly.
	if err := importOnce(t, dir,
		`{"stories":[{"epic":"e","id":"S1","title":"again"}]}`, false); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("A7: an explicit story collision must refuse cleanly, got %v", err)
	}
}
