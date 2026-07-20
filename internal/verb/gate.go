package verb

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// skipFile is the gitignored, per-machine gate-skip marker (§6.2, §9). It
// is written mid-commit by the pre-commit hook's SELFTRACKED_SKIP=1 path
// (via `gate skip-mark`, which performs no DB write) and converted into a
// gate-skip events row by the next write verb or by `load`.
const skipFile = "skip-pending"

// gateSkipEntity is the fixed instance token a gate-skip event carries in
// its entity column. A gate skip names no §4 subject — it is a
// machine-level fact, not tracked work — so, like `paths`/`config` events,
// it is instance-scoped and skipped by R8 (amendment
// gate-skip-joins-the-r8-carve-out). The skip's moment lives in detail.
const gateSkipEntity = "skip-pending"

// markerMode: the marker is a gitignored, per-machine scratch file.
const markerMode = 0o600

// GateVerb returns §6.2's `gate skip-mark`: it writes the marker the
// pre-commit skip path leaves behind, and — the contract that matters
// mid-commit — it touches no database (INV-276). The marker is transient
// and per-machine; a skip followed by no later write is visible only here
// (INV-279).
func GateVerb() cli.Verb {
	return cli.Verb{Name: "gate", Subs: []cli.Sub{{
		Name:  "skip-mark",
		Arity: 0,
		Usage: "gate skip-mark [--json]",
		Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
			return gateSkipMark(e)
		},
	}}}
}

// gateSkipMark writes .selftracked/skip-pending with the skip's moment. It
// requires the instance directory to exist (a marker outside a tracker is
// meaningless) but never opens the database: the point of the marker is to
// defer the DB write out of the commit boundary.
func gateSkipMark(e *cli.Env) error {
	if _, err := os.Stat(instanceDir); err != nil {
		return &cli.CodedError{
			Code:    codeNotFound,
			Message: "no " + instanceDir + " here; run selftracked init first",
			Status:  refusal,
		}
	}
	path := filepath.Join(instanceDir, skipFile)
	if err := os.WriteFile(path, []byte(now()+"\n"), markerMode); err != nil {
		return fmt.Errorf("gate skip-mark: write marker: %w", err)
	}
	_, _ = fmt.Fprintln(e.Stdout, "gate skip recorded; the next write converts it to a gate-skip event")
	return nil
}

// readSkipMarker reports whether a pending marker exists and returns the
// skip moment it recorded (empty if the marker carries none). A read error
// other than absence is surfaced: a marker that exists but cannot be read
// must not be silently ignored, or the skip trail is lost.
func readSkipMarker() (bool, string, error) {
	b, err := os.ReadFile(filepath.Join(instanceDir, skipFile))
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("read skip marker: %w", err)
	}
	return true, strings.TrimSpace(string(b)), nil
}

// clearSkipMarker removes the marker after its conversion. Absence is not
// an error (a concurrent same-machine write may have cleared it already).
func clearSkipMarker() error {
	if err := os.Remove(filepath.Join(instanceDir, skipFile)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear skip marker: %w", err)
	}
	return nil
}

// gateSkipEvent builds the events row a pending marker converts into.
func gateSkipEvent(moment string) Event {
	detail := "pre-commit gate skipped (SELFTRACKED_SKIP=1)"
	if moment != "" {
		detail = "pre-commit gate skipped at " + moment + " (SELFTRACKED_SKIP=1)"
	}
	return Event{Entity: gateSkipEntity, Event: "gate-skip", Detail: detail}
}

// ConvertSkipMarker converts a pending marker into a gate-skip events row
// standalone — one transaction, then the §6.1 derived-file tail — and
// clears the marker. It is what `load` calls after a rebuild (INV-277's
// "or load" clause); the write-verb pipeline instead folds the same event
// into the verb's own transaction (see Write), so no write verb pays for a
// second serialization pass. A no-op when no marker is pending.
func ConvertSkipMarker(ctx context.Context) error {
	present, moment, err := readSkipMarker()
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	db, err := schema.OpenWrite(filepath.Join(instanceDir, dbFile))
	if err != nil {
		return fmt.Errorf("convert skip marker: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := mutateInTx(ctx, db, func(_ *sql.Tx) ([]Event, error) {
		return []Event{gateSkipEvent(moment)}, nil
	}); err != nil {
		return err
	}
	if err := regenerateDerived(ctx, db); err != nil {
		return err
	}
	return clearSkipMarker()
}
