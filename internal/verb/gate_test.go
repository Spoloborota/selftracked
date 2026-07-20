package verb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/rules"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// eventCount reads how many rows are in events on a fresh read-only
// connection — the ground truth for "did a DB write happen".
func eventCount(t *testing.T, dir string) int {
	t.Helper()
	db, err := schema.OpenRead(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func markerPath(dir string) string { return filepath.Join(dir, instanceDir, skipFile) }

// TestGateSkipMarkWritesMarkerNoDBWrite is INV-276: `gate skip-mark` leaves
// the marker and touches no database mid-commit. The events count before
// and after is identical — the mark path never opens the DB.
func TestGateSkipMarkWritesMarkerNoDBWrite(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)

	before := eventCount(t, dir)
	if err := gateSkipMark(&cli.Env{Stdout: os.Stderr, Stderr: os.Stderr}); err != nil {
		t.Fatalf("gate skip-mark: %v", err)
	}
	if _, err := os.Stat(markerPath(dir)); err != nil {
		t.Fatalf("marker not written: %v", err)
	}
	if after := eventCount(t, dir); after != before {
		t.Fatalf("gate skip-mark wrote to the DB: events %d → %d", before, after)
	}
}

// TestGateSkipMarkRefusesOutsideTracker: a marker outside a tracker is
// meaningless, so the verb refuses rather than littering a stray file.
func TestGateSkipMarkRefusesOutsideTracker(t *testing.T) {
	t.Chdir(t.TempDir())
	err := gateSkipMark(&cli.Env{Stdout: os.Stderr, Stderr: os.Stderr})
	if err == nil {
		t.Fatal("expected refusal outside a tracker")
	}
	var coded *cli.CodedError
	if !errors.As(err, &coded) || coded.Code != "not-found" {
		t.Fatalf("want not-found refusal, got %v", err)
	}
}

// TestSkipMarkerConvertedByWriteVerb is INV-277's first clause: the next
// write verb folds the pending marker into its own transaction as a
// gate-skip event and clears the marker.
func TestSkipMarkerConvertedByWriteVerb(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)
	writeMarker(t, dir, "2020-01-02T03:04:05Z")

	// Any write verb: here a bare task insert through the real pipeline.
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		if _, err := tx.ExecContext(context.Background(),
			`INSERT INTO tasks (title, created_at, updated_at) VALUES ('t','d','d')`); err != nil {
			return nil, fmt.Errorf("seed write: %w", err)
		}
		return []Event{{Entity: "#1", Event: evCreate, Detail: "t"}}, nil
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	assertConverted(t, dir)
}

// TestConvertSkipMarkerStandalone is INV-277's "or load" clause: the
// standalone converter (what load calls) records the gate-skip event,
// re-dumps, and clears the marker with no other mutation.
func TestConvertSkipMarkerStandalone(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)
	writeMarker(t, dir, "2020-01-02T03:04:05Z")

	if err := ConvertSkipMarker(context.Background()); err != nil {
		t.Fatalf("ConvertSkipMarker: %v", err)
	}
	assertConverted(t, dir)

	// Idempotent: a second call with no marker is a clean no-op.
	if err := ConvertSkipMarker(context.Background()); err != nil {
		t.Fatalf("second ConvertSkipMarker: %v", err)
	}
	if n := gateSkipCount(t, dir); n != 1 {
		t.Fatalf("no-op call must not add a second gate-skip event: got %d", n)
	}
}

// TestGateSkipEventLeavesR8Green is the amendment fixture
// (gate-skip-joins-the-r8-carve-out): a gate-skip event carries a fixed
// instance token, not a §4 reference, so R8 must skip it by event type. A
// converted marker leaves verify's R8 rule green.
func TestGateSkipEventLeavesR8Green(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)
	writeMarker(t, dir, "2020-01-02T03:04:05Z")
	if err := ConvertSkipMarker(context.Background()); err != nil {
		t.Fatalf("ConvertSkipMarker: %v", err)
	}

	db, err := schema.OpenRead(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// DBOnly runs R6–R9/R12; R8 is the one that would flag an unresolvable
	// entity. The gate-skip entity ("skip-pending") is not a §4 reference,
	// so a green DBOnly proves R8 skipped it by event type rather than
	// resolving it.
	vs, err := rules.DBOnly(context.Background(), db)
	if err != nil {
		t.Fatalf("DBOnly: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("gate-skip event tripped a DB-only rule (R8 expected to skip it): %+v", vs)
	}
}

func writeMarker(t *testing.T, dir, moment string) {
	t.Helper()
	if err := os.WriteFile(markerPath(dir), []byte(moment+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gateSkipCount(t *testing.T, dir string) int {
	t.Helper()
	db, err := schema.OpenRead(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM events WHERE event = 'gate-skip'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// assertConverted checks the post-conversion invariants: exactly one
// gate-skip event exists, it carries the fixed instance token, and the
// marker is gone.
func assertConverted(t *testing.T, dir string) {
	t.Helper()
	if n := gateSkipCount(t, dir); n != 1 {
		t.Fatalf("want exactly one gate-skip event, got %d", n)
	}
	db, err := schema.OpenRead(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var entity, detail string
	if err := db.QueryRowContext(context.Background(),
		`SELECT entity, detail FROM events WHERE event = 'gate-skip'`).Scan(&entity, &detail); err != nil {
		t.Fatal(err)
	}
	if entity != gateSkipEntity {
		t.Fatalf("gate-skip entity = %q, want %q", entity, gateSkipEntity)
	}
	if _, err := os.Stat(markerPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("marker not cleared after conversion (stat err: %v)", err)
	}
	// The conversion re-dumped: the event must be in the tracked dump, and
	// the sidecar must hash the very dump on disk — no drift between DB,
	// dump, and sidecar after a gate-skip conversion (INV-351).
	dumpBytes, err := os.ReadFile(filepath.Join(dir, instanceDir, dumpFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dumpBytes), "gate-skip") {
		t.Fatal("regenerated dump.sql does not carry the gate-skip event")
	}
	sidecar, err := os.ReadFile(filepath.Join(dir, instanceDir, hashFile))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(dumpBytes)
	if got, want := strings.TrimSpace(string(sidecar)), hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("sidecar %q does not hash the post-conversion dump %q", got, want)
	}
}
