// Package migrate is §8.6's version gate: the two ordered comparisons
// every verb begins with, the escalation into a versioned rebuild when
// the database is behind the binary, and the §8.4 branch-(5) heal when a
// crashed migration left the re-dump unwritten. The rebuild itself is the
// load path (internal/load); this package decides WHEN it runs and owns
// the locking choreography around it.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	sqlite "modernc.org/sqlite"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/load"
	"github.com/Spoloborota/selftracked/internal/schema"
)

const (
	// DefaultDir is the instance directory the CLI gates.
	DefaultDir = ".selftracked"

	dbFile   = "db.sqlite"
	dumpFile = "dump.sql"

	infra = 2

	// codeNeedsNewer marks both forward-only refusals (§8.6): a tracked
	// dump, or a database, from a schema version this binary predates.
	codeNeedsNewer = "needs-newer-binary"
	// codeInterrupted marks the persisted migration sentinel: a writer
	// crashed between its commit and its rename, or is completing now.
	codeInterrupted = "migration-interrupted"
)

// currentVersion is the binary's compiled-in schema version. A variable
// only as the white-box test seam (a synthetic bump is the one way to
// exercise migration while a single real version exists); production
// never assigns it.
var currentVersion = schema.Version

// busyTimeoutMS bounds the wait for another process's migration lock.
// Variable for the same reason: the BUSY refusal fixture should not cost
// five real seconds.
var busyTimeoutMS = 5000

// Mode selects the two per-verb exemptions §8.6 names: prime surfaces a
// newer-than-binary dump non-fatally instead of refusing (§11.1), and
// load skips comparison (ii) — "(i), then rebuild".
type Mode struct {
	SkipNewerRefusal bool
	SkipMigration    bool
}

// Result reports what the gate did: a migration (Migrated, From→To), a
// branch-(5) re-dump heal (Healed, To), or nothing (the zero value — the
// common case).
type Result struct {
	From, To int
	Migrated bool
	Healed   bool
}

func refuse(code, format string, args ...any) error {
	return &cli.CodedError{Code: code, Message: fmt.Sprintf(format, args...), Status: infra}
}

const interruptedMessage = "a schema migration here was interrupted or is completing in another process — " +
	"re-run; if this persists, selftracked load --force rebuilds from the tracked dump"

// Gate runs §8.6's two ordered comparisons against dir and acts on them.
func Gate(ctx context.Context, dir string, mode Mode) (Result, error) {
	// Comparison (i): the tracked dump's header schema_version vs the
	// binary. Newer is the forward-only refusal (git's repository-format
	// rule); a missing or headerless dump is not "newer".
	headerV, headerOK := dump.TrackedHeaderVersion(dir)
	if headerOK && headerV > currentVersion && !mode.SkipNewerRefusal {
		return Result{}, refuse(codeNeedsNewer,
			"the tracked dump is schema_version=%d but this binary compiles v%d — upgrade selftracked (§8.6 forward-only)",
			headerV, currentVersion)
	}
	if mode.SkipMigration {
		// load: "(i), then rebuild" (§8.6) — comparison (ii) has no
		// subject when the verb replaces the database. This is also what
		// keeps load --force reachable as the recovery the interrupted-
		// migration refusal itself points at.
		return Result{}, nil
	}

	dbPath := filepath.Join(dir, dbFile)
	if _, statErr := os.Stat(dbPath); statErr != nil {
		//nolint:nilerr // no database to gate is a pass: init and load own that state
		return Result{}, nil
	}
	uv, err := readUserVersion(ctx, dbPath)
	if err != nil {
		return Result{}, err
	}
	return gateDatabase(ctx, dir, dbPath, uv, headerV, headerOK)
}

// gateDatabase is comparison (ii): the database's user_version vs the
// binary's compiled-in version.
func gateDatabase(ctx context.Context, dir, dbPath string, uv, headerV int, headerOK bool) (Result, error) {
	switch {
	case uv < 0:
		// The sentinel a migration writer leaves on the old inode before
		// releasing its lock (see swapAndRelease).
		return Result{}, refuse(codeInterrupted, interruptedMessage)
	case uv > currentVersion:
		// Unspecified by §8.6 (it names the dump header as the refusal
		// trigger) and unreachable without a crashed newer-binary
		// migration plus a downgrade; fail closed rather than operate a
		// schema this binary has never seen.
		return Result{}, refuse(codeNeedsNewer,
			"the database is schema v%d but this binary compiles v%d — upgrade selftracked (§8.6 forward-only)",
			uv, currentVersion)
	case uv == currentVersion:
		// §8.4 branch (5): the dump matches its sidecar but the DB is
		// ahead of the dump's header — a crash between the migration's
		// DB swap and its re-dump. Healed by re-running the re-dump.
		if headerOK && headerV < uv && sidecarMatchesTracked(dir) {
			if err := redump(ctx, dir, dbPath); err != nil {
				return Result{}, err
			}
			return Result{Healed: true, To: uv}, nil
		}
		return Result{}, nil
	default: // uv < currentVersion
		return runMigration(ctx, dir, dbPath, uv)
	}
}

// runMigration is §8.6's escalation: EXCLUSIVE lock, hydrate the locked
// snapshot, transform chain, rebuild through the load path, atomic
// rename, and — only when the pre-migration state showed no external
// change — the re-dump tail. In the externally-changed state the
// migration stays DB-side (§8.4 branch (6)): nothing may overwrite a
// pulled dump before the divergence is reconciled.
func runMigration(ctx context.Context, dir, dbPath string, from int) (Result, error) {
	st, ok := load.Steps[from]
	if !ok {
		return Result{}, refuse("no-transform",
			"the database reports schema v%d and this binary (v%d) carries no transform chain from it — "+
				"not a selftracked database, or an incompatible build", from, currentVersion)
	}
	// The §8.4 decision is snapshotted BEFORE the swap: afterwards the
	// old serialization is gone, and a regenerate-and-compare against the
	// new schema would call every migration "an external change". The
	// sidecar-anchored first arm is the one computable arm mid-migration;
	// a crash-residue mismatch therefore stays DB-side too — safe (never
	// overwrites), and it reconciles through load --force plus the next
	// migration's re-dump.
	divergedBefore := !sidecarMatchesTracked(dir)

	// Base connection posture, deliberately NOT OpenWrite: under
	// locking_mode=EXCLUSIVE every lock a connection touches becomes
	// sticky-until-close, so a FAILED lock attempt still keeps its SHARED
	// half and two racing gates starve each other permanently (observed
	// empirically). Normal locking mode holds the EXCLUSIVE lock for the
	// transaction — exactly the §8.6 span that needs it; the commit→rename
	// gap is guarded by the sentinel, not by a file lock.
	db, err := schema.Open(dbPath)
	if err != nil {
		return Result{}, fmt.Errorf("migrate: open: %w", err)
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("migrate: pin connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := acquireExclusive(ctx, conn); err != nil {
		return Result{}, err
	}
	rollback := func() { _, _ = conn.ExecContext(ctx, "ROLLBACK") }

	// Re-check on THIS handle after the wait (§8.6): the winner marked
	// the old inode before releasing, so a loser that blocked sees the
	// mark here even though its handle predates the rename. The ROLLBACK
	// comes BEFORE the outcome is interpreted: loserOutcome opens a fresh
	// connection by path, and a loser still inside its own EXCLUSIVE
	// transaction would block that read against itself for the whole
	// busy_timeout (found by the S11 close review, reproduced at the
	// driver level).
	cur, err := connUserVersion(ctx, conn)
	if err != nil {
		rollback()
		return Result{}, err
	}
	if cur != from {
		rollback()
		return Result{}, loserOutcome(ctx, dbPath, cur)
	}

	built, err := rebuildCurrent(ctx, dir, conn, st, from)
	if err != nil {
		rollback()
		return Result{}, err
	}
	if err := swapAndRelease(ctx, conn, db, built, dbPath, from); err != nil {
		return Result{}, err
	}
	return finishMigration(ctx, dir, dbPath, from, divergedBefore)
}

// finishMigration is the §8.6 tail decision: re-dump only when the
// pre-migration state showed no external change (§8.4 branch (6)).
func finishMigration(ctx context.Context, dir, dbPath string, from int, divergedBefore bool) (Result, error) {
	if !divergedBefore {
		if err := redump(ctx, dir, dbPath); err != nil {
			return Result{}, err
		}
	}
	return Result{Migrated: true, From: from, To: currentVersion}, nil
}

// rebuildCurrent hydrates the locked snapshot, runs the transform chain,
// and builds the current-version database in a temp file — §8.6's
// live-DB source feeding the load path.
func rebuildCurrent(ctx context.Context, dir string, conn *sql.Conn, st load.Step, from int) (string, error) {
	corpus, err := load.HydrateCorpus(ctx, conn, st.Tables)
	if err != nil {
		return "", fmt.Errorf("migrate: %w", err)
	}
	d, err := load.MigrateCorpus(corpus, from, currentVersion, dump.TableOrder())
	if err != nil {
		return "", fmt.Errorf("migrate: %w", err)
	}
	built, err := load.Build(ctx, dir, d)
	if err != nil {
		return "", fmt.Errorf("migrate: %w", err)
	}
	return built, nil
}

// swapAndRelease marks the old inode, commits, releases the lock, THEN
// renames: the mark is what makes a blocked loser's own-handle re-check
// decisive, and renaming only after close keeps the swap portable
// (Windows refuses to replace an open file). The close→rename gap is the
// §8.6 swap gap — benign under the single-writer axiom, and the sentinel
// turns any verb that lands inside it into a retryable refusal.
func swapAndRelease(ctx context.Context, conn *sql.Conn, db *sql.DB, built, dbPath string, from int) error {
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", -from)); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		_ = os.Remove(built)
		return fmt.Errorf("migrate: mark old database: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		_ = os.Remove(built)
		return fmt.Errorf("migrate: commit mark: %w", err)
	}
	if err := conn.Close(); err != nil {
		_ = os.Remove(built)
		return fmt.Errorf("migrate: release lock: %w", err)
	}
	if err := db.Close(); err != nil {
		_ = os.Remove(built)
		return fmt.Errorf("migrate: release lock: %w", err)
	}
	if err := os.Rename(built, dbPath); err != nil {
		_ = os.Remove(built)
		// Non-destructive recovery: the old database is intact, only
		// marked. Un-mark it so the next verb simply re-migrates instead
		// of refusing forever toward a load --force that could discard
		// diverged-state data (found by the S11 close review).
		if restoreErr := restoreMark(ctx, dbPath, from); restoreErr != nil {
			return fmt.Errorf("migrate: swap failed (%w) and the mark could not be restored (%w) — %s",
				err, restoreErr, interruptedMessage)
		}
		return fmt.Errorf("migrate: swap: %w — the database was left unchanged; re-run", err)
	}
	return nil
}

// restoreMark puts the pre-migration user_version back after a failed
// swap, returning the database to the plain "behind" state.
func restoreMark(ctx context.Context, dbPath string, from int) error {
	db, err := schema.Open(dbPath)
	if err != nil {
		return fmt.Errorf("restore mark: %w", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", from)); err != nil {
		return fmt.Errorf("restore mark: %w", err)
	}
	return nil
}

// acquireExclusive takes the EXCLUSIVE lock §8.6 names, or refuses BUSY
// after busyTimeoutMS. Fail-fast attempts with sleeps at THIS level,
// deliberately not SQLite's busy handler: a handler-waiting BEGIN holds
// the SHARED lock it already acquired, so two racing gates (or a gate
// and a committer) deadlock until a timeout breaks them — observed
// empirically with both IMMEDIATE and EXCLUSIVE begins. A failed BEGIN
// releases everything, so between attempts a waiting gate holds nothing
// the winner needs.
func acquireExclusive(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout = 0"); err != nil {
		return fmt.Errorf("migrate: busy_timeout: %w", err)
	}
	deadline := time.Now().Add(time.Duration(busyTimeoutMS) * time.Millisecond)
	for {
		_, err := conn.ExecContext(ctx, "BEGIN EXCLUSIVE")
		if err == nil {
			return nil
		}
		if !isBusy(err) {
			return fmt.Errorf("migrate: acquire lock: %w", err)
		}
		// A failed BEGIN leaves no transaction; the ROLLBACK clears any
		// driver-side statement state and its "no transaction" complaint
		// is expected.
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		if time.Now().After(deadline) {
			return &cli.CodedError{
				Code:    "busy",
				Message: "the database is locked — another process may be migrating; retry",
				Status:  infra,
			}
		}
		// Jittered, so two gates that failed against each other do not
		// retry in lockstep forever.
		const retryFloorMS, retryJitterMS = 5, 30
		//nolint:gosec // lock-retry jitter, not security material
		time.Sleep(time.Duration(retryFloorMS+rand.IntN(retryJitterMS)) * time.Millisecond)
	}
}

// loserOutcome interprets what the lock's previous holder left behind.
func loserOutcome(ctx context.Context, dbPath string, cur int) error {
	if cur == currentVersion {
		return nil // the winner migrated; proceed (§8.6)
	}
	if cur < 0 {
		// The winner committed its mark; the rename may simply not have
		// landed yet. A fresh open by path sees the renamed file if it has.
		fresh, err := readUserVersion(ctx, dbPath)
		if err != nil {
			return err
		}
		if fresh == currentVersion {
			return nil
		}
		return refuse(codeInterrupted, interruptedMessage)
	}
	return refuse("migration-race",
		"the database changed to schema v%d under the migration lock — re-run", cur)
}

// redump is the §8.6 tail: re-serialize with the current serializer, then
// the sidecar. STATE.md is deliberately not here — §8.6 names exactly
// these two writes, and every write verb refreshes STATE.md in its own
// §6.1 tail.
func redump(ctx context.Context, dir, dbPath string) error {
	db, err := schema.OpenRead(dbPath)
	if err != nil {
		return fmt.Errorf("migrate: re-dump: %w", err)
	}
	defer func() { _ = db.Close() }()
	text, err := dump.Serialize(ctx, db)
	if err != nil {
		return fmt.Errorf("migrate: re-dump: %w", err)
	}
	if err := dump.WriteDumpFile(dir, text); err != nil {
		return fmt.Errorf("migrate: re-dump: %w", err)
	}
	if err := dump.WriteSidecar(dir, text); err != nil {
		return fmt.Errorf("migrate: re-dump: %w", err)
	}
	return nil
}

func isBusy(err error) bool {
	var se *sqlite.Error
	const primaryMask, busy, locked = 0xff, 5, 6
	if errors.As(err, &se) {
		c := se.Code() & primaryMask
		return c == busy || c == locked
	}
	return false
}

func readUserVersion(ctx context.Context, dbPath string) (int, error) {
	db, err := schema.OpenRead(dbPath)
	if err != nil {
		return 0, fmt.Errorf("migrate: open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()
	var v int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("migrate: user_version: %w", err)
	}
	return v, nil
}

func connUserVersion(ctx context.Context, conn *sql.Conn) (int, error) {
	var v int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("migrate: user_version: %w", err)
	}
	return v, nil
}

// sidecarMatchesTracked is §8.4's first arm: the tracked dump hashes to
// the sidecar's record. Missing either file counts as NOT matching — the
// divergent-by-default posture (§8.4).
func sidecarMatchesTracked(dir string) bool {
	tracked, err := os.ReadFile(filepath.Join(dir, dumpFile)) //nolint:gosec // fixed .selftracked path
	if err != nil {
		return false
	}
	return dump.SidecarMatches(dir, tracked)
}
