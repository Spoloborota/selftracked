package cli_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/load"
	"github.com/Spoloborota/selftracked/internal/schema"
	"github.com/Spoloborota/selftracked/internal/verb"
)

// TestMain installs the real entrypoint under BOTH of its §6.1 names —
// selftracked and the strk alias — so every script drives the actual
// dispatcher end to end. The registry the scripts see is the fixture one:
// the shipped catalog is empty until S5a, and the scripts exercise the
// dispatcher's contract, which no future verb changes.
func TestMain(m *testing.M) {
	entry := func() {
		// Fixture verbs for the dispatcher scenarios, plus the real catalog
		// verbs as their stages land them — same wiring main.go uses.
		r := fixtureRegistry()
		base := []cli.Verb{dump.Verb(), load.Verb()}
		catalog := make([]cli.Verb, 0, len(base)+len(verb.Verbs()))
		catalog = append(catalog, base...)
		catalog = append(catalog, verb.Verbs()...)
		for _, v := range catalog {
			if err := r.Register(v); err != nil {
				panic(err)
			}
		}
		e := &cli.Env{Stdout: os.Stdout, Stderr: os.Stderr}
		os.Exit(cli.Run(r, e, os.Args[1:]))
	}
	testscript.Main(m, map[string]func(){
		"selftracked": entry,
		"strk":        entry,
	})
}

func TestScripts(t *testing.T) {
	t.Parallel()
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			// expectexit N cmd args… — the DoD demands EXACT exit codes,
			// and testscript's own ! only distinguishes zero from nonzero.
			"expectexit": func(ts *testscript.TestScript, neg bool, args []string) {
				if neg || len(args) < 2 {
					ts.Fatalf("usage: expectexit <code> <command> [args...]")
				}
				want, err := strconv.Atoi(args[0])
				if err != nil {
					ts.Fatalf("expectexit: %q is not a code", args[0])
				}
				got := 0
				if runErr := ts.Exec(args[1], args[2:]...); runErr != nil {
					var exitErr *exec.ExitError
					if !errors.As(runErr, &exitErr) {
						ts.Fatalf("expectexit: %v", runErr)
					}
					got = exitErr.ExitCode()
				}
				if got != want {
					ts.Fatalf("exit %d, want %d", got, want)
				}
			},
			// dumpdiff old new R A asserts the set-difference between two
			// dump snapshots: exactly R lines removed and A lines added —
			// the §8.2 review-surface shapes are counted, not vibed.
			"dumpdiff": func(ts *testscript.TestScript, neg bool, args []string) {
				if neg || len(args) != 4 {
					ts.Fatalf("usage: dumpdiff <old> <new> <removed> <added>")
				}
				oldLines := strings.Split(ts.ReadFile(args[0]), "\n")
				newLines := strings.Split(ts.ReadFile(args[1]), "\n")
				oldSet := map[string]int{}
				for _, l := range oldLines {
					oldSet[l]++
				}
				newSet := map[string]int{}
				for _, l := range newLines {
					newSet[l]++
				}
				var removed, added int
				for l, n := range oldSet {
					if d := n - newSet[l]; d > 0 {
						removed += d
					}
				}
				for l, n := range newSet {
					if d := n - oldSet[l]; d > 0 {
						added += d
					}
				}
				wantR, err1 := strconv.Atoi(args[2])
				wantA, err2 := strconv.Atoi(args[3])
				if err1 != nil || err2 != nil {
					ts.Fatalf("dumpdiff: numeric args required")
				}
				if removed != wantR || added != wantA {
					ts.Fatalf("dump diff: removed=%d added=%d; want removed=%d added=%d",
						removed, added, wantR, wantA)
				}
			},
			// seeddb builds .selftracked/db.sqlite with one of everything —
			// there are no creating verbs until S5a, and the dump scenarios
			// need data to serialize.
			"seeddb": func(ts *testscript.TestScript, neg bool, args []string) {
				if neg || len(args) != 0 {
					ts.Fatalf("usage: seeddb")
				}
				dir := ts.MkAbs(".selftracked")
				ts.Check(os.MkdirAll(dir, 0o750))
				db, err := schema.Open(filepath.Join(dir, "db.sqlite"))
				ts.Check(err)
				defer func() { _ = db.Close() }()
				ctx := context.Background()
				ts.Check(schema.Create(ctx, db))
				for _, stmt := range []string{
					`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'goal', 'ACTIVE', 'd')`,
					`INSERT INTO tasks (title, epic, created_at, updated_at) VALUES ('t', 'e', 'd', 'd')`,
					`INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`,
				} {
					_, err := db.ExecContext(ctx, stmt)
					ts.Check(err)
				}
				// Write verbs run the §8.4 divergence core, so a seeded
				// instance carries its dump and sidecar like init's would.
				text, err := dump.Serialize(ctx, db)
				ts.Check(err)
				ts.Check(dump.WriteFiles(dir, text))
			},
		},
	})
}
