package load_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/load"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// FuzzWhitelistParser is the §16 mandate: the parser is the security
// boundary, so it gets fuzzed. The invariant under fuzz is total safety —
// no panic, no hang — plus one semantic guarantee: whatever parses came
// through the closed literal grammar, so every accepted statement names a
// known table and every value is nil/int64/string.
func FuzzWhitelistParser(f *testing.F) {
	// Seeds: a real dump, then mutations along the historically dangerous
	// axes — statement smuggling, quote games, truncation.
	valid := string(seedDump(f))
	f.Add(valid)
	f.Add(strings.Replace(valid, "schema_version=1", "schema_version=99", 1))
	f.Add(valid + "PRAGMA journal_mode=WAL;\n")
	f.Add(valid + "INSERT INTO tasks (id) VALUES (1);\n")
	f.Add(strings.TrimSuffix(valid, "\n"))
	f.Add(valid[:len(valid)/2])
	f.Add("-- selftracked dump schema_version=1 tasks=0 artifacts=0\n")
	f.Add("")
	f.Add("''")
	f.Add(valid +
		"INSERT INTO events (seq, at, entity, event, detail)" +
		" VALUES (2, 'd''); DROP TABLE tasks; --', '#1', 'create', '');\n")

	f.Fuzz(func(t *testing.T, input string) {
		d, err := load.Parse([]byte(input))
		if err != nil {
			return // refusal is always a legal outcome
		}
		for _, ins := range d.Inserts {
			switch ins.Table {
			case "meta", "path_dictionary", "epics", "epic_criteria", "tasks",
				"task_links", "stories", "worklog", "artifacts",
				"task_artifacts", "epic_artifacts", "events":
			default:
				t.Fatalf("parser accepted unknown table %q", ins.Table)
			}
			for _, v := range ins.Values {
				switch v.(type) {
				case nil, int64, string:
				default:
					t.Fatalf("parser produced a value outside the closed grammar: %T", v)
				}
			}
		}
	})
}

// seedDump builds one valid dump for the fuzz corpus without depending on
// *testing.T helpers.
func seedDump(f *testing.F) []byte {
	f.Helper()
	dir := f.TempDir()
	text := buildSeedDump(dir)
	if text == nil {
		f.Fatal("could not build the seed dump")
	}
	return text
}

// buildSeedDump is goodDump without *testing.T, for the fuzz harness.
func buildSeedDump(dir string) []byte {
	db, err := schema.Open(filepath.Join(dir, "seed.sqlite"))
	if err != nil {
		return nil
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	if err := schema.Create(ctx, db); err != nil {
		return nil
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO events (at, entity, event) VALUES ('d', 'epic:e', 'create')`); err != nil {
		return nil
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO epics (slug, goal, created_at) VALUES ('e', 'g', 'd')`); err != nil {
		return nil
	}
	text, err := dump.Serialize(ctx, db)
	if err != nil {
		return nil
	}
	return text
}
