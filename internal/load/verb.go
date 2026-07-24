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
	"github.com/Spoloborota/selftracked/internal/verb"
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
		// state, and it says what it is about to discard first. The §8.4
		// divergence matrix lives in the write pipeline; this is only the
		// §8.3 discard floor.
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

	parsed, err := parseAndMigrate(e, text)
	if err != nil {
		return err
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
	// list names load).
	if err := dump.WriteSidecar(dir, text); err != nil {
		return fmt.Errorf("load: %w", err)
	}
	// A gate skip recorded before this load (§6.2) is converted now: load
	// is "what runs after a divergence", and INV-277 names it as the marker
	// converter alongside the next write verb. The conversion re-dumps, so
	// the freshly loaded state gains one local unsynced event — exactly the
	// skip that had not yet reached the tracked history.
	if err := verb.ConvertSkipMarker(ctx); err != nil {
		return fmt.Errorf("load: %w", err)
	}
	_, _ = fmt.Fprintln(e.Stdout, "loaded", dbPath, "from", dumpPath)
	return nil
}

// parseAndMigrate parses the tracked dump and, for an old version, runs
// §8.6's dump-side hydration source: the dump parsed with its own grammar
// migrates through the same transform chain the live-DB path runs, and
// builds at the current version. The tracked dump file is deliberately
// NOT rewritten here — load's sidecar records the bytes it loaded, which
// leaves exactly the §8.4 branch-(5) state, and the next gated verb
// re-dumps.
func parseAndMigrate(e *cli.Env, text []byte) (*Dump, error) {
	parsed, err := Parse(text)
	if err != nil {
		return nil, classifyRefusal(err)
	}
	if parsed.Version >= current {
		return parsed, nil
	}
	from := parsed.Version
	parsed, err = MigrateCorpus(CorpusFromDump(parsed), from, current, dump.TableOrder())
	if err != nil {
		return nil, classifyRefusal(err)
	}
	_, _ = fmt.Fprintf(e.Stderr, "selftracked: schema migrated v%d→v%d\n", from, current)
	return parsed, nil
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
