package load

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/schema"
)

const (
	instanceDir = ".selftracked"
	dbFile      = "db.sqlite"
	dumpFile    = "dump.sql"
)

// Verb returns the §6.2 `load [--force]` catalog entry.
func Verb() cli.Verb {
	var force bool
	return cli.Verb{
		Name: "load",
		Subs: []cli.Sub{{
			Arity: 0,
			Usage: "load [--force] [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.BoolVar(&force, "force", false, "replace an existing local database")
			},
			Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
				return run(e, force)
			},
		}},
	}
}

func run(e *cli.Env, force bool) error {
	dir := instanceDir
	dumpPath := filepath.Join(dir, dumpFile)
	//nolint:gosec // the fixed tracked-dump path inside .selftracked, not user input
	text, err := os.ReadFile(dumpPath)
	if err != nil {
		const refusal = 1
		return &cli.CodedError{Code: "not-found", Message: "no " + dumpPath + " to load", Status: refusal}
	}

	ctx := context.Background()
	dbPath := filepath.Join(dir, dbFile)
	_, statErr := os.Stat(dbPath)
	if statErr == nil {
		// §8.3: load --force is the ONLY operation that discards local DB
		// state, and it says what it is about to discard first. The full
		// §8.4 divergence matrix arrives at S8b; this is the safety floor.
		if !force {
			const refusal = 1
			return &cli.CodedError{
				Code:    "exists",
				Message: dbPath + " exists; load --force replaces the local database with the tracked dump",
				Status:  refusal,
			}
		}
		if err := printDiscardSummary(ctx, e, dbPath); err != nil {
			return err
		}
	}

	parsed, err := Parse(text)
	if err != nil {
		return classifyRefusal(err)
	}
	built, err := Build(ctx, dir, parsed)
	if err != nil {
		return classifyRefusal(err)
	}
	// Atomic rename: an interrupted load never lands (§8.3). Both paths
	// are fixed .selftracked names, not user input.
	//nolint:gosec
	if err := os.Rename(built, dbPath); err != nil {
		_ = os.Remove(built)
		return fmt.Errorf("load: rename: %w", err)
	}
	// The sidecar records the dump these bytes came from (§8.4's writer
	// list names load; the comparison matrix is S8b's).
	if err := dump.WriteSidecar(dir, text); err != nil {
		return fmt.Errorf("load: %w", err)
	}
	_, _ = fmt.Fprintln(e.Stdout, "loaded", dbPath, "from", dumpPath)
	return nil
}

// classifyRefusal maps parser/build refusals to the §6.1 contract: a
// refused dump is exit 2 (§8.5 names the exit explicitly — untrusted
// input that failed the boundary is an environment problem, not a clean
// domain refusal).
func classifyRefusal(err error) error {
	if errors.Is(err, ErrRefused) {
		const infra = 2
		return &cli.CodedError{Code: "refused", Message: err.Error(), Status: infra}
	}
	return err
}

func printDiscardSummary(ctx context.Context, e *cli.Env, dbPath string) error {
	db, err := schema.OpenRead(dbPath)
	if err != nil {
		// The file exists but does not open: nothing meaningful to
		// summarize, and --force is exactly for replacing wreckage.
		_, _ = fmt.Fprintln(e.Stderr, "replacing an unreadable local database")
		return nil
	}
	defer func() { _ = db.Close() }()
	var tasks, epics, events int64
	for _, q := range []struct {
		dst   *int64
		query string
	}{
		{&tasks, `SELECT COUNT(*) FROM tasks`},
		{&epics, `SELECT COUNT(*) FROM epics`},
		{&events, `SELECT COUNT(*) FROM events`},
	} {
		if err := db.QueryRowContext(ctx, q.query).Scan(q.dst); err != nil {
			_, _ = fmt.Fprintln(e.Stderr, "replacing a local database that resists inspection")
			return nil
		}
	}
	_, _ = fmt.Fprintf(e.Stderr, "discarding local database: %d task(s), %d epic(s), %d event(s)\n",
		tasks, epics, events)
	return nil
}
