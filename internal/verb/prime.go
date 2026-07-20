package verb

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/ref"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// defaultPrimeCap is the fallback when the meta row is somehow absent; the
// seeded value is 20 (§11.1), and config validates edits as positive ints.
const defaultPrimeCap = 20

// primeOutput is the §11.1 stable JSON contract. Field order and JSON tags
// are the contract; the two naming fields (epic goal, story title) are the
// only prose-class payload — nothing else carries free text (INV-469/470).
type primeOutput struct {
	EpicsActive  []activeEpic `json:"epics_active"`
	EpicsPaused  []string     `json:"epics_paused"`
	EpicsBacklog []string     `json:"epics_backlog"`
	Ready        []string     `json:"ready"`
	Triage       []string     `json:"triage"`
	InReview     []string     `json:"in_review"`
	Stale        []string     `json:"stale"`
	SprintGoals  []sprintGoal `json:"sprint_goals"`
	Totals       primeTotals  `json:"totals"`

	DumpDivergence          bool `json:"dump_divergence"`
	DumpRequiresNewerBinary bool `json:"dump_requires_newer_binary"`
	// Migrated is present only when this invocation migrated (§8.6). At v0
	// nothing migrates; the field's value and its verification ride to S11
	// with the migration engine (amendment migrated-field-rides-migration-at-s11).
	Migrated string `json:"migrated,omitempty"`
}

// activeEpic is one ACTIVE epic. criteria_unmet is a count, never criterion
// text (INV-458).
type activeEpic struct {
	Slug          string      `json:"slug"`
	Goal          string      `json:"goal"`
	Stories       epicStories `json:"stories"`
	CriteriaUnmet int         `json:"criteria_unmet"`
}

// epicStories tallies a single epic's stories: done/in_progress as counts,
// ready/blocked as story-id lists (the attention-worthy ones). Uncapped —
// bounded by the epic's own story count (INV-458/468).
type epicStories struct {
	Done       int      `json:"done"`
	InProgress int      `json:"in_progress"`
	Ready      []string `json:"ready"`
	Blocked    []string `json:"blocked"`
}

// sprintGoal is one IN-PROGRESS story: the (epic, story, title) tuple, one of
// the contract's two naming fields (INV-465/469).
type sprintGoal struct {
	Epic  string `json:"epic"`
	Story string `json:"story"`
	Title string `json:"title"`
}

// primeTotals is the full count of every capped list plus parked — the one
// authoritative representation of parked, no separate scalar (INV-461).
type primeTotals struct {
	Ready        int `json:"ready"`
	Triage       int `json:"triage"`
	InReview     int `json:"in_review"`
	Stale        int `json:"stale"`
	EpicsPaused  int `json:"epics_paused"`
	EpicsBacklog int `json:"epics_backlog"`
	Parked       int `json:"parked"`
}

// PrimeVerb returns §6.2/§11.1's `prime`: the session-start read that reports
// the whole active surface as one stable JSON object. Read-only — it never
// touches the DB or the dump file, including its divergence report (INV-361).
func PrimeVerb() cli.Verb {
	return cli.Verb{Name: "prime", Subs: []cli.Sub{{
		Arity: 0,
		Usage: "prime",
		Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
			return primeRun(e)
		},
	}}}
}

func primeRun(e *cli.Env) error {
	ctx := context.Background()
	var out primeOutput
	err := Read(ctx, func(ctx context.Context, db *sql.DB) error {
		return buildPrime(ctx, db, &out)
	})
	if err != nil {
		return err
	}
	if e.JSON {
		return printJSON(e, out)
	}
	printPrimeDigest(e, &out)
	return nil
}

// buildPrime assembles the whole contract on one read-only connection.
func buildPrime(ctx context.Context, db *sql.DB, out *primeOutput) error {
	capN := primeCap(ctx, db)
	steps := []func(context.Context, *sql.DB, *primeOutput, int) error{
		fillActiveEpics, fillEpicLists, fillTaskLists, fillStale, fillSprintGoals, fillDivergence,
	}
	for _, step := range steps {
		if err := step(ctx, db, out, capN); err != nil {
			return err
		}
	}
	// Never-nil slices: an empty contract list marshals as [], not null, so
	// the JSON shape is stable for consumers (INV-458–460).
	ensureEmpty(out)
	return nil
}

// primeCap reads the validated prime_cap meta row, falling back to the
// seeded default if it is somehow unreadable (a read verb never fails on a
// missing config knob).
func primeCap(ctx context.Context, db *sql.DB) int {
	var v string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'prime_cap'`).Scan(&v); err != nil {
		return defaultPrimeCap
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return defaultPrimeCap
	}
	return n
}

// fillActiveEpics lists ACTIVE epics (slug ASC, uncapped) with their story
// tallies and unmet-criteria count.
func fillActiveEpics(ctx context.Context, db *sql.DB, out *primeOutput, _ int) error {
	rows, err := db.QueryContext(ctx,
		`SELECT slug, goal FROM epics WHERE status = ? ORDER BY slug`, epicActive)
	if err != nil {
		return fmt.Errorf("prime: active epics: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var slugs []string
	goals := map[string]string{}
	for rows.Next() {
		var slug, goal string
		if err := rows.Scan(&slug, &goal); err != nil {
			return fmt.Errorf("prime: active epics: %w", err)
		}
		slugs = append(slugs, slug)
		goals[slug] = goal
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("prime: active epics: %w", err)
	}
	for _, slug := range slugs {
		st, err := epicStoryTally(ctx, db, slug)
		if err != nil {
			return err
		}
		unmet, err := epicUnmetCount(ctx, db, slug)
		if err != nil {
			return err
		}
		out.EpicsActive = append(out.EpicsActive, activeEpic{
			Slug: slug, Goal: goals[slug], Stories: st, CriteriaUnmet: unmet,
		})
	}
	return nil
}

// epicStoryTally counts an epic's done/in_progress stories and lists its
// ready/blocked story ids (id ASC).
func epicStoryTally(ctx context.Context, db *sql.DB, slug string) (epicStories, error) {
	st := epicStories{Ready: []string{}, Blocked: []string{}}
	rows, err := db.QueryContext(ctx,
		`SELECT id, status FROM stories WHERE epic = ? ORDER BY id`, slug)
	if err != nil {
		return st, fmt.Errorf("prime: story tally: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return st, fmt.Errorf("prime: story tally: %w", err)
		}
		switch status {
		case storyDone:
			st.Done++
		case storyInProgress:
			st.InProgress++
		case storyReady:
			st.Ready = append(st.Ready, id)
		case storyBlocked:
			st.Blocked = append(st.Blocked, id)
		}
	}
	if err := rows.Err(); err != nil {
		return st, fmt.Errorf("prime: story tally: %w", err)
	}
	return st, nil
}

func epicUnmetCount(ctx context.Context, db *sql.DB, slug string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM epic_criteria WHERE epic = ? AND met = 0`, slug).Scan(&n); err != nil {
		return 0, fmt.Errorf("prime: unmet criteria: %w", err)
	}
	return n, nil
}

// fillEpicLists fills the slug-only paused/backlog epic lists (slug ASC,
// capped) and their totals.
func fillEpicLists(ctx context.Context, db *sql.DB, out *primeOutput, capN int) error {
	paused, nPaused, err := cappedSlugs(ctx, db, epicPaused, capN)
	if err != nil {
		return err
	}
	backlog, nBacklog, err := cappedSlugs(ctx, db, epicBacklog, capN)
	if err != nil {
		return err
	}
	out.EpicsPaused, out.Totals.EpicsPaused = paused, nPaused
	out.EpicsBacklog, out.Totals.EpicsBacklog = backlog, nBacklog
	return nil
}

func cappedSlugs(ctx context.Context, db *sql.DB, status string, capN int) ([]string, int, error) {
	var total int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM epics WHERE status = ?`, status).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("prime: epic count: %w", err)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT slug FROM epics WHERE status = ? ORDER BY slug LIMIT ?`, status, capN)
	if err != nil {
		return nil, 0, fmt.Errorf("prime: epic list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var slugs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, 0, fmt.Errorf("prime: epic list: %w", err)
		}
		slugs = append(slugs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("prime: epic list: %w", err)
	}
	return slugs, total, nil
}

// fillTaskLists fills ready[] (from v_ready, honoring blocking deps),
// triage[] (NEEDS-TRIAGE), in_review[] (IN-REVIEW), each id ASC and capped,
// plus the parked total. Entries are task refs ("#NN") — identifiers, no prose.
func fillTaskLists(ctx context.Context, db *sql.DB, out *primeOutput, capN int) error {
	ready, nReady, err := cappedTaskRefs(ctx, db, `SELECT id FROM v_ready`, `SELECT COUNT(*) FROM v_ready`, capN)
	if err != nil {
		return err
	}
	triage, nTriage, err := cappedTaskRefs(ctx, db,
		`SELECT id FROM tasks WHERE status = 'NEEDS-TRIAGE' ORDER BY id`,
		`SELECT COUNT(*) FROM tasks WHERE status = 'NEEDS-TRIAGE'`, capN)
	if err != nil {
		return err
	}
	inReview, nInReview, err := cappedTaskRefs(ctx, db,
		`SELECT id FROM tasks WHERE status = 'IN-REVIEW' ORDER BY id`,
		`SELECT COUNT(*) FROM tasks WHERE status = 'IN-REVIEW'`, capN)
	if err != nil {
		return err
	}
	out.Ready, out.Totals.Ready = ready, nReady
	out.Triage, out.Totals.Triage = triage, nTriage
	out.InReview, out.Totals.InReview = inReview, nInReview

	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks WHERE parked <> ''`).Scan(&out.Totals.Parked); err != nil {
		return fmt.Errorf("prime: parked count: %w", err)
	}
	return nil
}

// cappedTaskRefs runs listQ (which must already order id ASC or be a
// pre-ordered view) with a LIMIT, and countQ for the full total.
func cappedTaskRefs(ctx context.Context, db *sql.DB, listQ, countQ string, capN int) ([]string, int, error) {
	var total int
	if err := db.QueryRowContext(ctx, countQ).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("prime: task count: %w", err)
	}
	rows, err := db.QueryContext(ctx, listQ+` LIMIT ?`, capN)
	if err != nil {
		return nil, 0, fmt.Errorf("prime: task list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var refs []string
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("prime: task list: %w", err)
		}
		refs = append(refs, ref.TaskRef(id))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("prime: task list: %w", err)
	}
	return refs, total, nil
}

// fillStale fills stale[] (path ASC, capped): git-changed files intersected
// with active non-ephemeral artifact links — the same set the `stale` verb
// reports. Unlike `stale` (whose whole job is that list, so it refuses when
// git cannot answer), stale here is one advisory field of a broad
// session-start surface: a git failure (unborn HEAD, not a repo) degrades to
// an empty list rather than sinking the whole contract, which must stay
// emittable whenever the DB is readable (§11.1's healthy branch).
func fillStale(ctx context.Context, db *sql.DB, out *primeOutput, capN int) error {
	changed, err := gitChanged(ctx, "")
	if err != nil {
		out.Stale = []string{}
		return nil //nolint:nilerr // an advisory field degrades, it does not sink prime
	}
	linked, err := activeArtifactPaths(ctx, db)
	if err != nil {
		return err
	}
	var stale []string
	for p := range changed {
		if linked[p] {
			stale = append(stale, p)
		}
	}
	sort.Strings(stale) // path ASC (INV-467)
	out.Totals.Stale = len(stale)
	if len(stale) > capN {
		stale = stale[:capN]
	}
	out.Stale = stale
	return nil
}

// fillSprintGoals fills sprint_goals[] — every IN-PROGRESS story, uncapped
// (bounded by WIP=1 per epic), epic then story ASC.
func fillSprintGoals(ctx context.Context, db *sql.DB, out *primeOutput, _ int) error {
	rows, err := db.QueryContext(ctx,
		`SELECT epic, id, title FROM stories WHERE status = ? ORDER BY epic, id`, storyInProgress)
	if err != nil {
		return fmt.Errorf("prime: sprint goals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var g sprintGoal
		if err := rows.Scan(&g.Epic, &g.Story, &g.Title); err != nil {
			return fmt.Errorf("prime: sprint goals: %w", err)
		}
		out.SprintGoals = append(out.SprintGoals, g)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("prime: sprint goals: %w", err)
	}
	return nil
}

// fillDivergence sets the two §8-sourced booleans, both read-only:
// dump_divergence (the tracked dump moved under this DB, §8.4) and
// dump_requires_newer_binary (the tracked dump's header schema_version is
// ahead of this binary, §8.6). Neither writes the DB or the dump (INV-361).
func fillDivergence(ctx context.Context, db *sql.DB, out *primeOutput, _ int) error {
	diverged, err := divergenceReport(ctx, db, instanceDir)
	if err != nil {
		return err
	}
	out.DumpDivergence = diverged

	newer, err := dumpNeedsNewerBinary(instanceDir)
	if err != nil {
		return err
	}
	out.DumpRequiresNewerBinary = newer
	return nil
}

// divergenceReport is §8.4's comparison performed read-only: it never heals
// the sidecar or writes anything. Sidecar healing on crash-residue is every
// write verb's job (divergenceCore in the write pipeline); prime only reports.
// True means the tracked dump differs from both the sidecar hash and this
// DB's serialization — the "someone pulled a new dump.sql" case (§8.4/INV-455).
func divergenceReport(ctx context.Context, db *sql.DB, dir string) (bool, error) {
	tracked, trackedErr := os.ReadFile(filepath.Join(dir, dumpFile)) //nolint:gosec // fixed .selftracked path
	sidecar, sidecarErr := os.ReadFile(filepath.Join(dir, hashFile)) //nolint:gosec // fixed .selftracked path
	if sidecarErr != nil || trackedErr != nil {
		// A missing sidecar or tracked dump is a reconcile-first state; the
		// safe read-only report is "divergent" (§8.4's missing-sidecar
		// default). The read errors are the report's DATA, not a run failure.
		//nolint:nilerr // a missing file is a divergent report, not an error
		return true, nil
	}
	sum := sha256.Sum256(tracked)
	if hex.EncodeToString(sum[:]) == strings.TrimSpace(string(sidecar)) {
		return false, nil // tracked dump matches its sidecar: no external change
	}
	// Mismatch: regenerate from the DB (in memory) and compare. Equal ⇒ crash
	// residue (DB and dump agree, only the sidecar is stale) ⇒ not divergent.
	regen, err := dump.Serialize(ctx, db)
	if err != nil {
		return false, fmt.Errorf("prime: divergence report: %w", err)
	}
	if string(regen) == string(tracked) {
		return false, nil
	}
	return true, nil // the tracked dump changed under this DB
}

// dumpNeedsNewerBinary reads the tracked dump's header schema_version and
// reports whether it is ahead of this binary's compiled-in version (§8.6
// forward-only, surfaced non-fatally at session start). A missing/unreadable
// header is not "ahead": false.
func dumpNeedsNewerBinary(dir string) (bool, error) {
	f, err := os.Open(filepath.Join(dir, dumpFile)) //nolint:gosec // fixed .selftracked path
	if err != nil {
		return false, nil
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return false, nil
	}
	v, ok := parseDumpHeaderVersion(sc.Text())
	if !ok {
		return false, nil
	}
	return v > schema.Version, nil
}

// parseDumpHeaderVersion extracts N from the header line
// "-- selftracked dump schema_version=N tasks=… artifacts=…" (§8.1).
func parseDumpHeaderVersion(line string) (int, bool) {
	const key = "schema_version="
	_, rest, ok := strings.Cut(line, key)
	if !ok {
		return 0, false
	}
	if before, _, found := strings.Cut(rest, " "); found {
		rest = before
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return n, true
}

// ensureEmpty replaces nil contract slices with empty ones so they marshal as
// [] rather than null — a stable shape for consumers.
func ensureEmpty(out *primeOutput) {
	if out.EpicsActive == nil {
		out.EpicsActive = []activeEpic{}
	}
	if out.EpicsPaused == nil {
		out.EpicsPaused = []string{}
	}
	if out.EpicsBacklog == nil {
		out.EpicsBacklog = []string{}
	}
	if out.Ready == nil {
		out.Ready = []string{}
	}
	if out.Triage == nil {
		out.Triage = []string{}
	}
	if out.InReview == nil {
		out.InReview = []string{}
	}
	if out.Stale == nil {
		out.Stale = []string{}
	}
	if out.SprintGoals == nil {
		out.SprintGoals = []sprintGoal{}
	}
}

// printPrimeDigest is the human fallback when --json is absent: a terse
// count-oriented summary. The machine contract is the --json form; this is a
// convenience, not part of the stable contract.
func printPrimeDigest(e *cli.Env, out *primeOutput) {
	_, _ = fmt.Fprintf(e.Stdout, "active epics: %d\n", len(out.EpicsActive))
	for _, ep := range out.EpicsActive {
		_, _ = fmt.Fprintf(e.Stdout, "  %s — %s (unmet criteria: %d)\n", ep.Slug, ep.Goal, ep.CriteriaUnmet)
	}
	_, _ = fmt.Fprintf(e.Stdout, "sprint goals: %d · ready: %d · triage: %d · in review: %d · stale: %d · parked: %d\n",
		len(out.SprintGoals), out.Totals.Ready, out.Totals.Triage, out.Totals.InReview, out.Totals.Stale, out.Totals.Parked)
	if out.DumpDivergence {
		_, _ = fmt.Fprintln(e.Stdout, "dump_divergence: reconcile with `load` before working")
	}
	if out.DumpRequiresNewerBinary {
		_, _ = fmt.Fprintln(e.Stdout, "dump_requires_newer_binary: upgrade selftracked")
	}
}
