package verb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/instance"
	"github.com/Spoloborota/selftracked/internal/ref"
	"github.com/Spoloborota/selftracked/internal/rules"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// defaultPrimeCap is the fallback when the meta row is somehow absent; the
// seeded value is 20 (§11.1), and config validates edits as positive ints.
const defaultPrimeCap = 20

// primeOutput is the §11.1 stable JSON contract. Field order and JSON tags
// are the contract; the two naming fields (epic goal, story title) are the
// only prose-class payload — nothing else carries free text (INV-469/470).
type primeOutput struct {
	// Instance is the §11.1 tracker identity (amendment
	// `a-tracker-carries-a-name`): 12 hex characters, the leading 48 bits
	// of SHA-256(salt || physically-resolved path). FIRST field of the
	// payload — the subject of a payload belongs before its predicates.
	// Omitted (never downgraded to an unsalted form) when the path cannot
	// be resolved or the salt cannot be read or created; consumers treat
	// it as optional.
	Instance string `json:"instance,omitempty"`

	EpicsActive  []activeEpic `json:"epics_active"`
	EpicsPaused  []string     `json:"epics_paused"`
	EpicsBacklog []string     `json:"epics_backlog"`
	Ready        []string     `json:"ready"`
	Triage       []string     `json:"triage"`
	InReview     []string     `json:"in_review"`
	Stale        []string     `json:"stale"`
	SprintGoals  []sprintGoal `json:"sprint_goals"`
	Notices      []notice     `json:"notices"`
	Totals       primeTotals  `json:"totals"`

	DumpDivergence          bool `json:"dump_divergence"`
	DumpRequiresNewerBinary bool `json:"dump_requires_newer_binary"`
	// Migrated is present only when this invocation migrated (§8.6): the
	// dispatcher's gate hands the outcome in through Env (the slot was
	// built at S8c, its value landed at S11 with the migration engine —
	// amendment migrated-field-rides-migration-at-s11).
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

// epicStories tallies a single epic's stories: done/in_progress/planned as
// counts, ready/blocked as story-id lists (the attention-worthy ones).
// Uncapped — bounded by the epic's own story count (INV-458/468).
//
// `planned` joined the tally with amendment
// `prime-names-an-epic-that-cannot-receive-work`: without it an epic holding
// three PLANNED stories and an epic holding none render identically, and the
// ABSENCE of a no-workable-story notice becomes the only way to tell them
// apart — a contract whose signal is a field's silence.
type epicStories struct {
	Done       int      `json:"done"`
	InProgress int      `json:"in_progress"`
	Planned    int      `json:"planned"`
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

// noticeNoWorkableStory is v0's ONLY notice code (§11.1): an ACTIVE epic
// with no story in a non-terminal status, so work can be recorded nowhere
// under it. The predicate is rules.EpicsWithoutWorkableStory — the same one
// §7's R10 trigger (a) reads, never a second copy of the story-status
// vocabulary.
const noticeNoWorkableStory = "no-workable-story"

// notice is one condition prime STATES rather than leaves to be inferred
// from a counter: a fixed code token plus the identifier it is about. It is
// a typed enumeration, NOT prose — so §11.1's rule that the epic goal and
// the story title are the contract's only prose-class payload holds
// unchanged (INV-469).
//
// The condition is stated, not enforced: no verb requires the agent to act
// on it, and the interactive protocol that puts the choice to the owner is
// deliberately v0.1.
type notice struct {
	Code string `json:"code"`
	Epic string `json:"epic"`
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
// the whole active surface as one stable JSON object. prime's own body is
// read-only — its divergence report never writes the DB or the dump
// (INV-361) — but the dispatcher's §8.6 gate runs before every verb, and
// for prime it may migrate or heal (the spec-sanctioned escalation: the
// first post-upgrade prime is exactly the invocation that migrates).
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
	// The dispatcher's version gate ran before this verb (§6.1); when it
	// migrated, this invocation reports it (§11.1) — the field's value
	// arrives here, its contract slot was built at S8c.
	out.Migrated = e.Migrated
	err := Read(ctx, func(ctx context.Context, db *sql.DB) error {
		return buildPrime(ctx, db, &out)
	})
	if err != nil {
		return err
	}
	// The identity is computed only once the tracker in the working
	// directory answered — creating the salt on first use is §6.1's one
	// named read-verb exception, scoped to prime alone. A failed
	// computation omits the field (§11.1); it never sinks prime.
	if id, ok := instance.Digest(instanceDir); ok {
		out.Instance = id
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
		fillActiveEpics, fillEpicLists, fillTaskLists, fillStale, fillSprintGoals,
		fillNotices, fillDivergence,
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

// epicStoryTally counts an epic's done/in_progress/planned stories and lists
// its ready/blocked story ids (id ASC). DISSOLVED is counted nowhere, as
// before: a dissolved story is not a story the epic still holds.
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
		case storyPlanned:
			st.Planned++
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

// fillNotices fills notices[] — the §11.1 conditions prime states outright.
// Never capped: it is bounded by the ACTIVE-epic count, like epics_active[]
// itself, and capping it would hide exactly the epics it exists to name
// (§2's "nothing hides").
//
// The one v0 code comes from rules.EpicsWithoutWorkableStory, which is also
// R10's trigger (a). Calling it here rather than re-asking the question in
// SQL is the point of the shared package: two hand-written copies of the
// non-terminal story vocabulary is the drift this surface pair exists to
// avoid.
func fillNotices(ctx context.Context, db *sql.DB, out *primeOutput, _ int) error {
	homeless, err := rules.EpicsWithoutWorkableStory(ctx, db)
	if err != nil {
		return fmt.Errorf("prime: notices: %w", err)
	}
	for _, slug := range homeless {
		out.Notices = append(out.Notices, notice{Code: noticeNoWorkableStory, Epic: slug})
	}
	// The contract's order is code ASC, then epic ASC.
	//
	// IN v0 THIS SORT IS WHOLLY UNEXERCISED, and no test can make it fail:
	// there is one code, and rules.EpicsWithoutWorkableStory already ends
	// `ORDER BY e.slug`, so both legs are no-ops on every input reachable
	// today — deleting the whole block leaves the suite green. It is kept
	// because the ordering is the CONTRACT's, not the predicate's: stating
	// it at the emitting site is what stops a second code from being
	// appended into a silently wrong order, and it becomes load-bearing the
	// moment that code exists.
	slices.SortFunc(out.Notices, func(a, b notice) int {
		if c := strings.Compare(a.Code, b.Code); c != 0 {
			return c
		}
		return strings.Compare(a.Epic, b.Epic)
	})
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

	out.DumpRequiresNewerBinary = dumpNeedsNewerBinary(instanceDir)
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

// dumpNeedsNewerBinary reports whether the tracked dump's header is
// ahead of this binary's compiled-in version (§8.6 forward-only,
// surfaced non-fatally at session start). One parser for every reader —
// dump.TrackedHeaderVersion — so prime can never disagree with the gate
// about what counts as a header (an S11 close-review finding: the
// previous local copy skipped the prefix check and the v ≥ 1 guard).
// A missing/unreadable header is not "ahead": false.
func dumpNeedsNewerBinary(dir string) bool {
	v, ok := dump.TrackedHeaderVersion(dir)
	return ok && v > schema.Version
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
	if out.Notices == nil {
		out.Notices = []notice{}
	}
}

// printPrimeDigest is the human fallback when --json is absent: a terse
// count-oriented summary. The machine contract is the --json form; this is a
// convenience, not part of the stable contract.
func printPrimeDigest(e *cli.Env, out *primeOutput) {
	// The instance identity is the first token of the first line (§11.1):
	// an agent that reads the state before learning whose state it is has
	// already formed the model the field exists to correct. Absent when
	// the field is omitted — the line then opens with the counter as before.
	prefix := ""
	if out.Instance != "" {
		prefix = out.Instance + " "
	}
	_, _ = fmt.Fprintf(e.Stdout, "%sactive epics: %d\n", prefix, len(out.EpicsActive))
	for _, ep := range out.EpicsActive {
		_, _ = fmt.Fprintf(e.Stdout, "  %s — %s (unmet criteria: %d)\n", ep.Slug, ep.Goal, ep.CriteriaUnmet)
	}
	// Each notice renders as a sentence BEFORE the counter line — the whole
	// point of the field: a bare `sprint goals: 0` reads the same for a
	// healthy epic between two stories as for one nothing can be recorded
	// against.
	for _, n := range out.Notices {
		_, _ = fmt.Fprintln(e.Stdout, noticeSentence(n))
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

// noticeSentence renders one notice for the human digest. The sentence lives
// here and not in the JSON on purpose: the digest is explicitly outside the
// stable contract, and the contract carries the typed code so a consumer
// never parses prose. The fallback is not decoration — a code added without
// its sentence must still print its subject rather than vanish from the
// digest.
//
// EVERY interpolated identifier is %q-quoted, matching `worklog add`'s
// refusals. `import` does not apply the kebab-case rule `epic create`
// enforces and `epics.slug` carries no shape CHECK, so a slug can be an
// arbitrary string; dropped raw into a fluent English sentence it reads as
// part of the sentence. Quoting does not sanitize it — it marks where the
// data starts and ends, which is what a reader needs to see.
func noticeSentence(n notice) string {
	switch n.Code {
	case noticeNoWorkableStory:
		return fmt.Sprintf("notice: epic %q has no story that can receive work — "+
			"new work has no home; opening a story is the owner's call", n.Epic)
	default:
		return fmt.Sprintf("notice: %q (epic %q)", n.Code, n.Epic)
	}
}
