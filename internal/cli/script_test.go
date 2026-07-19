package cli_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/schema"
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
		if err := r.Register(dump.Verb()); err != nil {
			panic(err)
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
			},
		},
	})
}
