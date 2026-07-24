package load

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/schema"
)

const metaTable = "meta"

var errBoom = errors.New("boom")

// These tests mutate the package's white-box seams (current, Steps) and
// therefore never run parallel; every mutation is restored in a Cleanup,
// which Go runs before the parallel batch starts.

func bumpTo2(t *testing.T, step Step) {
	t.Helper()
	current = 2
	Steps[1] = step
	t.Cleanup(func() {
		current = schema.Version
		delete(Steps, 1)
	})
}

// metaOnlyStep is the smallest v1 grammar a fresh instance's dump needs:
// its only data rows are meta rows.
func metaOnlyStep(ddl string) Step {
	return Step{
		DDL: ddl,
		Tables: []TableSpec{
			{Name: metaTable, Columns: "key, value", OrderBy: "key", Where: "key <> 'active_verb'"},
		},
		Transform: func(*Corpus) error { return nil },
	}
}

func freshDumpText(t *testing.T) []byte {
	t.Helper()
	ctx := context.Background()
	db, err := schema.Open(filepath.Join(t.TempDir(), "seed.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := schema.Create(ctx, db); err != nil {
		t.Fatal(err)
	}
	text, err := dump.Serialize(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestMigrateCorpusRunsTheChainAndPinsTheVersion(t *testing.T) {
	step := metaOnlyStep(schema.DDL())
	step.Transform = func(c *Corpus) error {
		c.Tables[metaTable] = append(c.Tables[metaTable], []Literal{"transform_probe", "ran"})
		return nil
	}
	bumpTo2(t, step)

	c := &Corpus{Tables: map[string][][]Literal{
		metaTable: {{metaSchemaVersionKey, "1"}, {"events_archived_through", "0"}},
	}}
	d, err := MigrateCorpus(c, 1, 2, dump.TableOrder())
	if err != nil {
		t.Fatal(err)
	}
	if d.Version != 2 {
		t.Fatalf("Version = %d, want 2", d.Version)
	}
	var sawPin, sawProbe bool
	for _, ins := range d.Inserts {
		if ins.Table != metaTable {
			t.Fatalf("unexpected table %q", ins.Table)
		}
		if ins.Values[0] == metaSchemaVersionKey && ins.Values[1] == "2" {
			sawPin = true
		}
		if ins.Values[0] == "transform_probe" {
			sawProbe = true
		}
	}
	if !sawPin || !sawProbe {
		t.Fatalf("pin=%v probe=%v, want both (inserts: %v)", sawPin, sawProbe, d.Inserts)
	}
}

func TestMigrateCorpusRefusesAMissingChain(t *testing.T) {
	c := &Corpus{Tables: map[string][][]Literal{}}
	_, err := MigrateCorpus(c, 1, 2, dump.TableOrder())
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "no transform from schema v1") {
		t.Fatalf("err = %v, want a no-transform refusal", err)
	}
}

func TestMigrateCorpusWrapsATransformError(t *testing.T) {
	step := metaOnlyStep(schema.DDL())
	step.Transform = func(*Corpus) error { return errBoom }
	bumpTo2(t, step)

	_, err := MigrateCorpus(&Corpus{Tables: map[string][][]Literal{}}, 1, 2, dump.TableOrder())
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "transform v1→v2: boom") {
		t.Fatalf("err = %v, want a wrapped transform refusal", err)
	}
}

func TestMigrateCorpusRefusesATableOutsideTheOrder(t *testing.T) {
	step := metaOnlyStep(schema.DDL())
	step.Transform = func(c *Corpus) error {
		c.Tables["alien"] = [][]Literal{{"x"}}
		return nil
	}
	bumpTo2(t, step)

	_, err := MigrateCorpus(&Corpus{Tables: map[string][][]Literal{}}, 1, 2, dump.TableOrder())
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "outside the serializer order") {
		t.Fatalf("err = %v, want the stray-table refusal", err)
	}
}

func TestCorpusFromDumpGroupsPerTable(t *testing.T) {
	d := &Dump{Version: 1, Inserts: []Insert{
		{Table: metaTable, Values: []Literal{"a", "1"}},
		{Table: "events", Values: []Literal{int64(1), "t", "e", "ev", "d"}},
		{Table: metaTable, Values: []Literal{"b", "2"}},
	}}
	c := CorpusFromDump(d)
	if len(c.Tables[metaTable]) != 2 || len(c.Tables["events"]) != 1 {
		t.Fatalf("grouping wrong: %v", c.Tables)
	}
	if c.Tables[metaTable][1][0] != "b" {
		t.Fatal("row order not preserved within a table")
	}
}

func TestHydrateCorpusReadsInOrderAndFilters(t *testing.T) {
	ctx := context.Background()
	db, err := schema.Open(filepath.Join(t.TempDir(), "h.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := schema.Create(ctx, db); err != nil {
		t.Fatal(err)
	}
	// The serializer's own exclusion: a leaked active_verb row must not
	// resurrect through hydration.
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('active_verb', 'crash-leak')`); err != nil {
		t.Fatal(err)
	}

	c, err := HydrateCorpus(ctx, db, metaOnlyStep(schema.DDL()).Tables)
	if err != nil {
		t.Fatal(err)
	}
	rows := c.Tables[metaTable]
	if len(rows) == 0 {
		t.Fatal("no meta rows hydrated")
	}
	prev := ""
	for _, r := range rows {
		key, ok := r[0].(string)
		if !ok {
			t.Fatalf("meta key hydrated as %T, want string", r[0])
		}
		if key == "active_verb" {
			t.Fatal("active_verb resurrected through hydration")
		}
		if key < prev {
			t.Fatalf("rows out of ORDER BY: %q after %q", key, prev)
		}
		prev = key
	}
}

func TestParseSelectsAHistoricalGrammar(t *testing.T) {
	// A v1 dialect whose DDL deliberately differs from the canonical
	// text: parse succeeds only if the registry's grammar — not the
	// compiled-in current one — judged the block. (The current version's
	// grammar is resolved compiled-in-first by design; the registry is
	// consulted only for genuinely historical versions.)
	oldDDL := schema.DDL() + "-- v1 historical tail\n"
	bumpTo2(t, metaOnlyStep(oldDDL))

	text := "-- selftracked dump schema_version=1 tasks=0 artifacts=0\n" + oldDDL +
		"INSERT INTO meta (key, value) VALUES ('schema_version', '1');\n"
	d, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("historical grammar not honored: %v", err)
	}
	if d.Version != 1 || len(d.Inserts) != 1 {
		t.Fatalf("parsed %+v, want one v1 insert", d)
	}

	// The canonical DDL no longer matches the v1 grammar — byte-equality
	// is judged against the REGISTERED text.
	wrong := "-- selftracked dump schema_version=1 tasks=0 artifacts=0\n" + schema.DDL()
	if _, err := Parse([]byte(wrong)); !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v, want a DDL refusal against the v1 grammar", err)
	}
}

func TestLoadDoorMigratesAnOldDump(t *testing.T) {
	text := freshDumpText(t)
	bumpTo2(t, metaOnlyStep(schema.DDL()))

	dir := t.TempDir()
	inst := filepath.Join(dir, instanceDir)
	if err := os.MkdirAll(inst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inst, dumpFile), text, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	var stdout, stderr strings.Builder
	env := &cli.Env{Stdout: &stdout, Stderr: &stderr}
	if err := run(env, false); err != nil {
		t.Fatalf("load of a v1 dump under a v2 binary: %v", err)
	}
	if !strings.Contains(stderr.String(), "schema migrated v1→v2") {
		t.Fatalf("stderr lacks the migration notice: %q", stderr.String())
	}

	db, err := schema.OpenRead(filepath.Join(inst, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var uv int
	if err := db.QueryRow("PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != 2 {
		t.Fatalf("user_version = %d, want 2", uv)
	}
	var v string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v); err != nil || v != "2" {
		t.Fatalf("meta schema_version = %q err=%v, want 2", v, err)
	}
	// The tracked dump was deliberately left at v1 with a matching
	// sidecar — the §8.4 branch-(5) state the next gated verb re-dumps.
	if !dump.SidecarMatches(inst, text) {
		t.Fatal("sidecar does not record the loaded (old) dump bytes")
	}
}

// TestBuildAbortsOnNonContiguousWorklog proves §8.6's hydration
// obligation end to end: the INSERT-firing gates bind transform output,
// so a corpus whose worklog seq is not contiguous per epic aborts the
// rebuild on the schema's own worklog_seq_contiguous trigger.
func TestBuildAbortsOnNonContiguousWorklog(t *testing.T) {
	bumpTo2(t, metaOnlyStep(schema.DDL()))
	c := &Corpus{Tables: map[string][][]Literal{
		metaTable: {
			{metaSchemaVersionKey, "1"},
			{"events_archived_through", "0"},
		},
		"epics": {
			{"e1", "a goal", "BACKLOG", "", "", "2026-07-24T00:00:00Z"},
		},
		// seq 2 with no seq 1: a renumbering transform's mistake.
		"worklog": {
			{"e1", int64(2), "S1", "2026-07-24", "DONE", "", "", "", nil, ""},
		},
	}}
	d, err := MigrateCorpus(c, 1, 2, dump.TableOrder())
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	_, err = Build(context.Background(), t.TempDir(), d)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "contiguous") {
		t.Fatalf("err = %v, want the worklog_seq_contiguous abort as a refusal", err)
	}
}
