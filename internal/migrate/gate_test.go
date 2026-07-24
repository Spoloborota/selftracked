package migrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/load"
	"github.com/Spoloborota/selftracked/internal/schema"
	"github.com/Spoloborota/selftracked/internal/verb"
)

// The tests in this file mutate the package's white-box seams
// (currentVersion, load.Steps, verb.GateVersion) and therefore never run
// parallel: Go finishes every sequential test — cleanups included —
// before the parallel batch starts, so the seams are always restored
// before anything else can observe them.

const primeVerbName = "prime"

// newInstance builds a real, current-version instance directory: a
// created database plus its serialized dump and matching sidecar — the
// state init leaves behind.
func newInstance(t *testing.T) string {
	t.Helper()
	inst := filepath.Join(t.TempDir(), ".selftracked")
	if err := os.MkdirAll(inst, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db, err := schema.Open(filepath.Join(inst, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Create(ctx, db); err != nil {
		t.Fatal(err)
	}
	text, err := dump.Serialize(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dump.WriteDumpFile(inst, text); err != nil {
		t.Fatal(err)
	}
	if err := dump.WriteSidecar(inst, text); err != nil {
		t.Fatal(err)
	}
	return inst
}

// syntheticBump is the one way to exercise migration while a single real
// schema version exists (§8.6: the first transform is written when v2
// is): the binary's target becomes 2 and version 1 gains an identity
// step whose specs mirror the v1 serializer.
func syntheticBump(t *testing.T) {
	t.Helper()
	currentVersion = 2
	verb.GateVersion = 2
	load.Steps[1] = v1Step()
	t.Cleanup(func() {
		currentVersion = schema.Version
		verb.GateVersion = schema.Version
		delete(load.Steps, 1)
	})
}

func v1Step() load.Step {
	return load.Step{
		DDL: schema.DDL(),
		Tables: []load.TableSpec{
			{Name: "meta", Columns: "key, value", OrderBy: "key", Where: "key <> 'active_verb'"},
			{Name: "path_dictionary", Columns: "class, scope, root, ephemeral, note", OrderBy: "class, scope"},
			{Name: "epics", Columns: "slug, goal, status, status_note, close_sweep, created_at", OrderBy: "slug"},
			{
				Name:    "epic_criteria",
				Columns: "epic, seq, criterion, met, evidence", OrderBy: "epic, seq",
			},
			{
				Name:    "tasks",
				Columns: "id, title, status, status_note, parked, dup_of, epic, created_at, updated_at",
				OrderBy: "id",
			},
			{Name: "task_links", Columns: "from_task, to_task, type, note", OrderBy: "from_task, to_task, type"},
			{Name: "stories", Columns: "epic, id, title, status, dod, consumes, produces, blocked", OrderBy: "epic, id"},
			{
				Name:    "worklog",
				Columns: "epic, seq, story, date, state, commits, gate, review, corrects, note",
				OrderBy: "epic, seq",
			},
			{Name: "artifacts", Columns: "id, class, scope, relpath, archived, note", OrderBy: "id"},
			{Name: "task_artifacts", Columns: "task, artifact, role, note", OrderBy: "task, artifact"},
			{Name: "epic_artifacts", Columns: "epic, artifact, role, note", OrderBy: "epic, artifact"},
			{Name: "events", Columns: "seq, at, entity, event, detail", OrderBy: "seq"},
		},
		Transform: func(*load.Corpus) error { return nil },
	}
}

func execOnDB(t *testing.T, inst, stmt string) {
	t.Helper()
	db, err := schema.Open(filepath.Join(inst, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

func gateErrCode(t *testing.T, err error) (string, int) {
	t.Helper()
	var coded *cli.CodedError
	if !errors.As(err, &coded) {
		t.Fatalf("expected a CodedError, got %v", err)
	}
	return coded.Code, coded.Status
}

func firstLine(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return line
}

func TestGatePassesACurrentInstance(t *testing.T) {
	inst := newInstance(t)
	res, err := Gate(context.Background(), inst, Mode{})
	if err != nil || res != (Result{}) {
		t.Fatalf("gate on a current instance: res=%+v err=%v, want zero/nil", res, err)
	}
}

func TestGateRefusesANewerDump(t *testing.T) {
	inst := newInstance(t)
	header := "-- selftracked dump schema_version=99 tasks=0 artifacts=0\n"
	if err := os.WriteFile(filepath.Join(inst, dumpFile), []byte(header), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Gate(context.Background(), inst, Mode{})
	code, status := gateErrCode(t, err)
	if code != codeNeedsNewer || status != 2 {
		t.Fatalf("newer dump: code=%s status=%d, want %s/2", code, status, codeNeedsNewer)
	}

	// prime's mode surfaces the same condition non-fatally (§8.6/§11.1):
	// the gate passes and the verb's own report carries the flag.
	res, err := Gate(context.Background(), inst, Mode{SkipNewerRefusal: true})
	if err != nil || res != (Result{}) {
		t.Fatalf("prime mode on a newer dump: res=%+v err=%v, want zero/nil", res, err)
	}
}

func TestGateMigratesABehindDatabase(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)

	res, err := Gate(context.Background(), inst, Mode{})
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if !res.Migrated || res.From != 1 || res.To != 2 || res.Healed {
		t.Fatalf("result = %+v, want Migrated From=1 To=2", res)
	}
	uv, err := readUserVersion(context.Background(), filepath.Join(inst, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	if uv != 2 {
		t.Fatalf("user_version after migration = %d, want 2", uv)
	}
	// The §8.6 tail ran (no external change): header and sidecar follow.
	if got := firstLine(t, filepath.Join(inst, dumpFile)); !strings.Contains(got, "schema_version=2") {
		t.Fatalf("dump header after re-dump: %q, want schema_version=2", got)
	}
	if !sidecarMatchesTracked(inst) {
		t.Fatal("sidecar does not match the re-dumped tracked dump")
	}
	// The migrated database still carries its data (the seeded meta
	// config rows survive the identity transform).
	db, err := schema.OpenRead(filepath.Join(inst, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var v string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key = 'prime_cap'`).Scan(&v); err != nil || v != "20" {
		t.Fatalf("prime_cap after migration = %q err=%v, want 20", v, err)
	}
	if err := db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v); err != nil || v != "2" {
		t.Fatalf("meta schema_version after migration = %q err=%v, want 2", v, err)
	}
}

func TestGateStaysDBSideWhenDiverged(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)
	// An external change: the tracked dump moved under this DB (§8.4).
	tampered := []byte("-- selftracked dump schema_version=1 tasks=0 artifacts=0\n-- pulled change\n")
	if err := os.WriteFile(filepath.Join(inst, dumpFile), tampered, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Gate(context.Background(), inst, Mode{})
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if !res.Migrated || res.From != 1 || res.To != 2 {
		t.Fatalf("result = %+v, want a v1→v2 migration", res)
	}
	// Branch (6): DB-side only — nothing may overwrite a pulled dump.
	b, err := os.ReadFile(filepath.Join(inst, dumpFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(tampered) {
		t.Fatal("the migration rewrote a tracked dump it saw as externally changed")
	}
	uv, err := readUserVersion(context.Background(), filepath.Join(inst, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	if uv != 2 {
		t.Fatalf("user_version = %d, want 2 (migration itself must land)", uv)
	}
}

func TestGateHealsBranchFiveResidue(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)
	// A migration that crashed between the DB swap and its re-dump: the
	// database is current, the tracked dump and its sidecar still agree
	// on the old version (§8.4 branch (5)).
	execOnDB(t, inst, `UPDATE meta SET value = '2' WHERE key = 'schema_version'`)
	execOnDB(t, inst, `PRAGMA user_version = 2`)

	res, err := Gate(context.Background(), inst, Mode{})
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if !res.Healed || res.To != 2 || res.Migrated {
		t.Fatalf("result = %+v, want Healed To=2", res)
	}
	if got := firstLine(t, filepath.Join(inst, dumpFile)); !strings.Contains(got, "schema_version=2") {
		t.Fatalf("dump header after heal: %q, want schema_version=2", got)
	}
	if !sidecarMatchesTracked(inst) {
		t.Fatal("sidecar does not match the healed dump")
	}

	// Idempotence: the healed state passes the gate silently.
	res, err = Gate(context.Background(), inst, Mode{})
	if err != nil || res != (Result{}) {
		t.Fatalf("gate after heal: res=%+v err=%v, want zero/nil", res, err)
	}
}

func TestGateLeavesBranchFiveAloneWhenDiverged(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)
	execOnDB(t, inst, `UPDATE meta SET value = '2' WHERE key = 'schema_version'`)
	execOnDB(t, inst, `PRAGMA user_version = 2`)
	before := []byte("-- selftracked dump schema_version=1 tasks=0 artifacts=0\n-- pulled change\n")
	if err := os.WriteFile(filepath.Join(inst, dumpFile), before, 0o644); err != nil {
		t.Fatal(err)
	}

	// Residue plus divergence: the heal must not touch a dump the sidecar
	// does not vouch for.
	res, err := Gate(context.Background(), inst, Mode{})
	if err != nil || res != (Result{}) {
		t.Fatalf("gate: res=%+v err=%v, want zero/nil (reconcile-first state)", res, err)
	}
	b, err := os.ReadFile(filepath.Join(inst, dumpFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(before) {
		t.Fatal("the gate rewrote a diverged tracked dump")
	}
}

func TestGateRefusesWithoutATransformChain(t *testing.T) {
	inst := newInstance(t)
	// user_version 0: a database this binary cannot have created — no
	// registered chain leaves it (the registry is empty at v0).
	execOnDB(t, inst, `PRAGMA user_version = 0`)

	_, err := Gate(context.Background(), inst, Mode{})
	code, status := gateErrCode(t, err)
	if code != "no-transform" || status != 2 {
		t.Fatalf("code=%s status=%d, want no-transform/2", code, status)
	}
}

func TestGateRefusesAnInterruptedMigration(t *testing.T) {
	inst := newInstance(t)
	execOnDB(t, inst, `PRAGMA user_version = -1`)

	_, err := Gate(context.Background(), inst, Mode{})
	code, status := gateErrCode(t, err)
	if code != codeInterrupted || status != 2 {
		t.Fatalf("code=%s status=%d, want %s/2", code, status, codeInterrupted)
	}

	// load is the recovery that refusal names: its mode must pass the
	// gate so load --force can rebuild from the tracked dump.
	res, err := Gate(context.Background(), inst, Mode{SkipMigration: true})
	if err != nil || res != (Result{}) {
		t.Fatalf("load mode on a sentinel: res=%+v err=%v, want zero/nil", res, err)
	}
}

func TestGateRefusesBusy(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)
	busyTimeoutMS = 100
	t.Cleanup(func() { busyTimeoutMS = 5000 })

	// Another process's lock, held across the gate's whole attempt.
	holder, err := schema.OpenWrite(filepath.Join(inst, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close() }()
	ctx := context.Background()
	conn, err := holder.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}

	_, err = Gate(ctx, inst, Mode{})
	code, status := gateErrCode(t, err)
	if code != "busy" || status != 2 {
		t.Fatalf("code=%s status=%d, want busy/2", code, status)
	}
	if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
}

// TestGateRaceOneWinner runs the §8.6 escalation race for real: two
// concurrent gates, one instance. Exactly one migrates; the other either
// proceeds silently (it saw the winner's outcome) or refuses with the
// retryable migration-interrupted message (it woke inside the winner's
// close→rename gap). Nothing else is legal.
func TestGateRaceOneWinner(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)

	type outcome struct {
		res Result
		err error
	}
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 2 {
		wg.Go(func() {
			<-start
			res, err := Gate(context.Background(), inst, Mode{})
			results <- outcome{res, err}
		})
	}
	close(start)
	wg.Wait()
	close(results)

	migrations := 0
	for o := range results {
		switch {
		case o.err == nil && o.res.Migrated && o.res.From == 1 && o.res.To == 2:
			migrations++
		case o.err == nil && o.res == (Result{}):
			// the loser proceeded after the winner
		default:
			var coded *cli.CodedError
			if errors.As(o.err, &coded) && coded.Code == codeInterrupted {
				continue // woke inside the swap gap; the message says retry
			}
			t.Fatalf("illegal race outcome: res=%+v err=%v", o.res, o.err)
		}
	}
	if migrations != 1 {
		t.Fatalf("%d migrations ran, want exactly 1", migrations)
	}
	uv, err := readUserVersion(context.Background(), filepath.Join(inst, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	if uv != 2 {
		t.Fatalf("final user_version = %d, want 2", uv)
	}
	if got := firstLine(t, filepath.Join(inst, dumpFile)); !strings.Contains(got, "schema_version=2") {
		t.Fatalf("final dump header: %q, want schema_version=2", got)
	}
}

func TestGatePassesWhereNoInstanceExists(t *testing.T) {
	res, err := Gate(context.Background(), filepath.Join(t.TempDir(), ".selftracked"), Mode{})
	if err != nil || res != (Result{}) {
		t.Fatalf("gate without an instance: res=%+v err=%v, want zero/nil", res, err)
	}
}

func TestLoserOutcomeInterpretsTheMark(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)
	ctx := context.Background()
	dbPath := filepath.Join(inst, dbFile)

	// cur == currentVersion: the winner migrated while we waited.
	if err := loserOutcome(ctx, dbPath, 2); err != nil {
		t.Fatalf("cur=current: err=%v, want nil", err)
	}
	// cur is the sentinel and a fresh open still sees it: interrupted.
	execOnDB(t, inst, `PRAGMA user_version = -1`)
	err := loserOutcome(ctx, dbPath, -1)
	code, _ := gateErrCode(t, err)
	if code != codeInterrupted {
		t.Fatalf("sentinel persisting: code=%s, want %s", code, codeInterrupted)
	}
	// cur is the sentinel but the path now shows the winner's file.
	execOnDB(t, inst, `PRAGMA user_version = 2`)
	if err := loserOutcome(ctx, dbPath, -1); err != nil {
		t.Fatalf("sentinel then renamed: err=%v, want nil", err)
	}
}

// TestCLIGateReportsMigrationThroughPrime drives the full chain a session
// start would: dispatcher → gate → migration → prime, asserting the §11.1
// contract — a migrating invocation reports migrated:vK→vN, the next one
// omits the field — and the §8.6 stderr notice.
func TestCLIGateReportsMigrationThroughPrime(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)
	t.Chdir(filepath.Dir(inst))

	reg := &cli.Registry{}
	if err := reg.Register(verb.PrimeVerb()); err != nil {
		t.Fatal(err)
	}
	old := cli.VersionGate
	cli.VersionGate = CLIGate
	t.Cleanup(func() { cli.VersionGate = old })

	var stdout, stderr strings.Builder
	env := &cli.Env{Stdout: &stdout, Stderr: &stderr}
	if code := cli.Run(reg, env, []string{primeVerbName, "--json"}); code != 0 {
		t.Fatalf("prime exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"migrated":"v1→v2"`) {
		t.Fatalf("prime JSON lacks migrated:v1→v2: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "schema migrated v1→v2") {
		t.Fatalf("stderr lacks the §8.6 notice: %q", stderr.String())
	}

	// The next invocation did not migrate: the field is absent (§11.1).
	stdout.Reset()
	stderr.Reset()
	env2 := &cli.Env{Stdout: &stdout, Stderr: &stderr}
	if code := cli.Run(reg, env2, []string{primeVerbName, "--json"}); code != 0 {
		t.Fatalf("second prime exited %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), `"migrated"`) {
		t.Fatalf("non-migrating prime still reports migrated: %s", stdout.String())
	}
}

func TestGateModeSkipsMigrationForLoad(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)

	res, err := Gate(context.Background(), inst, Mode{SkipMigration: true})
	if err != nil || res != (Result{}) {
		t.Fatalf("load mode: res=%+v err=%v, want zero/nil", res, err)
	}
	uv, err := readUserVersion(context.Background(), filepath.Join(inst, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	if uv != 1 {
		t.Fatalf("load mode migrated anyway: user_version = %d, want 1", uv)
	}
}

// A second consecutive migration attempt after a completed one must be a
// no-op — the gate is idempotent by state, not by memory.
func TestGateIsIdempotentAfterAMigration(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)
	if _, err := Gate(context.Background(), inst, Mode{}); err != nil {
		t.Fatal(err)
	}
	res, err := Gate(context.Background(), inst, Mode{})
	if err != nil || res != (Result{}) {
		t.Fatalf("second gate: res=%+v err=%v, want zero/nil", res, err)
	}
}

// The forward-only refusal must fire even when only the DB is ahead of
// the binary (fail closed on an unknown schema, §8.6's spirit).
func TestGateRefusesADatabaseAheadOfTheBinary(t *testing.T) {
	inst := newInstance(t)
	execOnDB(t, inst, `PRAGMA user_version = 9`)

	_, err := Gate(context.Background(), inst, Mode{})
	code, status := gateErrCode(t, err)
	if code != codeNeedsNewer || status != 2 {
		t.Fatalf("code=%s status=%d, want %s/2", code, status, codeNeedsNewer)
	}
}

// TestMigrationCommutesAcrossSources is §8.6's commutativity claim run
// against the synthetic chain: migrating via the live DB (the gate) and
// via the dump (the load door's engine) must produce byte-identical
// serializations, asserted at events_archived_through = 0 — the only v0
// state.
func TestMigrationCommutesAcrossSources(t *testing.T) {
	inst := newInstance(t)
	dumpText, err := os.ReadFile(filepath.Join(inst, dumpFile))
	if err != nil {
		t.Fatal(err)
	}
	syntheticBump(t)
	ctx := context.Background()

	// Source A: the live database, through the gate.
	if _, err := Gate(ctx, inst, Mode{}); err != nil {
		t.Fatal(err)
	}
	viaDB := serializeDB(t, filepath.Join(inst, dbFile))

	// Source B: the same state's dump, through the parse→chain→build
	// engine the load door runs.
	parsed, err := load.Parse(dumpText)
	if err != nil {
		t.Fatal(err)
	}
	d, err := load.MigrateCorpus(load.CorpusFromDump(parsed), 1, 2, dump.TableOrder())
	if err != nil {
		t.Fatal(err)
	}
	built, err := load.Build(ctx, t.TempDir(), d)
	if err != nil {
		t.Fatal(err)
	}
	viaDump := serializeDB(t, built)

	if viaDB != viaDump {
		t.Fatalf("migration does not commute:\nvia live DB:\n%s\nvia dump:\n%s", viaDB, viaDump)
	}
}

func serializeDB(t *testing.T, dbPath string) string {
	t.Helper()
	db, err := schema.OpenRead(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	text, err := dump.Serialize(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

// TestCLIGateHealsThroughPrime drives §8.4 branch (5) through the full
// session-start chain: a crashed migration's residue is healed by the
// gate in front of prime, the notice reaches stderr, and prime's JSON
// carries no migrated field (nothing migrated on THIS invocation).
func TestCLIGateHealsThroughPrime(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)
	execOnDB(t, inst, `UPDATE meta SET value = '2' WHERE key = 'schema_version'`)
	execOnDB(t, inst, `PRAGMA user_version = 2`)
	t.Chdir(filepath.Dir(inst))

	reg := &cli.Registry{}
	if err := reg.Register(verb.PrimeVerb()); err != nil {
		t.Fatal(err)
	}
	old := cli.VersionGate
	cli.VersionGate = CLIGate
	t.Cleanup(func() { cli.VersionGate = old })

	var stdout, stderr strings.Builder
	env := &cli.Env{Stdout: &stdout, Stderr: &stderr}
	if code := cli.Run(reg, env, []string{primeVerbName, "--json"}); code != 0 {
		t.Fatalf("prime exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "re-dump completed") {
		t.Fatalf("stderr lacks the heal notice: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), `"migrated"`) {
		t.Fatalf("a heal is not a migration; prime reported: %s", stdout.String())
	}
	if got := firstLine(t, filepath.Join(inst, dumpFile)); !strings.Contains(got, "schema_version=2") {
		t.Fatalf("dump header after the heal: %q, want schema_version=2", got)
	}
	if !sidecarMatchesTracked(inst) {
		t.Fatal("sidecar does not match the healed dump")
	}
}

// TestSwapFailureRestoresTheMark proves the non-destructive recovery: if
// the rename cannot land, the old database — still intact at its path —
// is un-marked back to its pre-migration version, so the next verb
// simply re-migrates instead of refusing forever toward a
// data-discarding load --force (an S11 close-review finding).
func TestSwapFailureRestoresTheMark(t *testing.T) {
	inst := newInstance(t)
	syntheticBump(t)
	ctx := context.Background()
	dbPath := filepath.Join(inst, dbFile)

	db, err := schema.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := acquireExclusive(ctx, conn); err != nil {
		t.Fatal(err)
	}

	// A built file that vanished makes the rename fail while the old
	// database stays intact at dbPath — the exact recovery scenario.
	missingBuilt := filepath.Join(inst, "db.sqlite.load-gone")
	err = swapAndRelease(ctx, conn, db, missingBuilt, dbPath, 1)
	if err == nil {
		t.Fatal("swapAndRelease with a missing built file must fail")
	}
	if !strings.Contains(err.Error(), "left unchanged") {
		t.Fatalf("err = %v, want the state-restored message", err)
	}
	uv, err := readUserVersion(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if uv != 1 {
		t.Fatalf("user_version = %d, want 1 (the mark restored)", uv)
	}
	// And the instance is fully operational again: the gate simply
	// migrates on the next invocation.
	res, err := Gate(ctx, inst, Mode{})
	if err != nil || !res.Migrated {
		t.Fatalf("gate after restore: res=%+v err=%v, want a clean migration", res, err)
	}
}
