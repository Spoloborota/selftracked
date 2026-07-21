package verb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Spoloborota/selftracked/internal/ref"
)

// taskTerminal / epicTerminal / storyTerminal are the statuses that need
// --legacy to INSERT (INV-442/259): historical records carry them, ordinary
// new records do not.
var (
	taskTerminal  = map[string]bool{statusDone: true, statusWontDo: true, statusDuplicate: true}
	epicTerminal  = map[string]bool{epicClosed: true, epicDissolved: true}
	storyTerminal = map[string]bool{storyDone: true, storyDissolved: true}
)

// validate runs the pre-write checks that do not depend on git: text
// hygiene (§8.1) on every stored string, and — without --legacy — the
// terminal-state refusal. The other two relaxations (synthesized dates,
// legacy: commits) are gated inside date resolution; all three answer to the
// single legacy bool (INV-439).
func (c *corpus) validate(legacy bool) error {
	if err := c.validateText(); err != nil {
		return err
	}
	if err := c.validateFutureIncrement(); err != nil {
		return err
	}
	if legacy {
		return nil
	}
	return c.validateTerminal()
}

// validateFutureIncrement requires a future_increment task to name the epic it
// is homed to, so the field gates a real check rather than sitting inert
// (INV-448).
func (c *corpus) validateFutureIncrement() error {
	for _, t := range c.Tasks {
		if t.FutureIncrement && t.Epic == "" {
			return refuse("usage", "task %q is a future_increment but names no epic", t.Title)
		}
	}
	return nil
}

// validateTerminal is the without-legacy relaxation gate: a terminal epic,
// task or story is a historical record only --legacy admits.
func (c *corpus) validateTerminal() error {
	for _, e := range c.Epics {
		if epicTerminal[e.Status] {
			return refuse("legacy-required", "epic %q is terminal (%s); a terminal INSERT needs --legacy", e.Slug, e.Status)
		}
	}
	for _, t := range c.Tasks {
		if taskTerminal[t.Status] {
			return refuse("legacy-required", "task %q is terminal (%s); a terminal INSERT needs --legacy", t.Title, t.Status)
		}
	}
	for _, s := range c.Stories {
		if storyTerminal[s.Status] {
			return refuse("legacy-required", "story %s/%s is terminal (%s); a terminal INSERT needs --legacy",
				s.Epic, s.ID, s.Status)
		}
	}
	return nil
}

// validateText passes every user-supplied string through §8.1's control-char
// gate, so a corpus can never smuggle a byte that breaks the one-line dump
// grammar.
func (c *corpus) validateText() error {
	for _, p := range c.Paths {
		if err := validateAll("path", p.Class, p.Scope, p.Root, p.Note); err != nil {
			return err
		}
	}
	if err := c.validateEpicsText(); err != nil {
		return err
	}
	for _, s := range c.Stories {
		if err := validateAll("story", s.Epic, s.ID, s.Title, s.Status, s.Dod, s.Consumes, s.Produces); err != nil {
			return err
		}
	}
	for _, t := range c.Tasks {
		if err := validateAll("task", t.Title, t.Status, t.Note, t.Epic, t.PointerNote, t.OwnerSteer); err != nil {
			return err
		}
	}
	return c.validateWorklogText()
}

// validateEpicsText gates each epic's strings and — the A2 fix — its criteria
// (criterion/evidence), matching the sibling `criteria add`/`criteria met`.
func (c *corpus) validateEpicsText() error {
	for _, e := range c.Epics {
		if err := validateAll("epic", e.Slug, e.Goal, e.Status, e.StatusNote, e.CloseSweep); err != nil {
			return err
		}
		for _, cr := range e.Criteria {
			if err := validateAll("criterion", cr.Criterion, cr.Evidence); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *corpus) validateWorklogText() error {
	for _, w := range c.Worklog {
		if err := validateAll("worklog",
			w.Epic, w.Story, w.State, w.Commits, w.Gate, w.Review, w.Note, w.LegacyReason); err != nil {
			return err
		}
		for _, inc := range w.Increments {
			if err := validateAll("increment", inc.Commits, inc.Gate, inc.Note); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAll(field string, values ...string) error {
	for _, v := range values {
		if err := validateText(field, v); err != nil {
			return err
		}
	}
	return nil
}

// insert is the mutate closure: it runs every INSERT in the one Write
// transaction (so the contiguity trigger, terminal-INSERT allowance and the
// CHECKs bind exactly as for a live write) and returns the events rows — one
// per imported task (the terminal ones R12 requires, plus the INV-056/440
// backfill marks), one per epic that is terminal or received worklog rows,
// the latter carrying the date-source map (INV-264/265/447).
func (im *importer) insert(ctx context.Context, tx *sql.Tx,
	c corpus, episodes []resolvedEpisode, tasks []resolvedTask,
) ([]Event, error) {
	if err := im.insertPaths(ctx, tx, c.Paths); err != nil {
		return nil, err
	}
	if err := im.insertEpics(ctx, tx, c.Epics, earliestWorklog(episodes)); err != nil {
		return nil, err
	}
	if err := im.insertStories(ctx, tx, c.Stories, episodes); err != nil {
		return nil, err
	}
	taskEvents, err := im.insertTasks(ctx, tx, tasks)
	if err != nil {
		return nil, err
	}
	sourceMap, err := im.insertWorklog(ctx, tx, episodes)
	if err != nil {
		return nil, err
	}
	return append(taskEvents, epicEvents(c.Epics, sourceMap)...), nil
}

func (im *importer) insertPaths(ctx context.Context, tx *sql.Tx, paths []pathRow) error {
	for _, p := range paths {
		// A (class, scope) already in the dictionary is a clean refusal, not a
		// raw UNIQUE driver string (RC-3).
		exists, err := pathExists(ctx, tx, p.Class, p.Scope)
		if err != nil {
			return err
		}
		if exists {
			return refuse("path-exists", "path (%s, %s) already exists in the instance", p.Class, p.Scope)
		}
		eph := int64(0)
		if p.Ephemeral {
			eph = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO path_dictionary (class, scope, root, ephemeral, note) VALUES (?, ?, ?, ?, ?)`,
			p.Class, p.Scope, p.Root, eph, p.Note); err != nil {
			return err //nolint:wrapcheck // constraint codes ride to the mapper
		}
	}
	return nil
}

// pathExists reports whether the instance already holds a (class, scope) path.
func pathExists(ctx context.Context, tx *sql.Tx, class, scope string) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx,
		`SELECT 1 FROM path_dictionary WHERE class = ? AND scope = ?`, class, scope).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("path lookup: %w", err)
	}
	return true, nil
}

func (im *importer) insertEpics(ctx context.Context, tx *sql.Tx, epics []epicRow, earliest map[string]string) error {
	for _, e := range epics {
		// An epic slug already in the DB is a clean refusal, not a raw UNIQUE
		// driver string (RC-3).
		exists, err := epicExists(ctx, tx, e.Slug)
		if err != nil {
			return err
		}
		if exists {
			return refuse("epic-exists", "epic %q already exists in the instance", e.Slug)
		}
		status := e.Status
		if status == "" {
			status = epicBacklog
		}
		created, err := im.epicCreatedAt(e, earliest)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO epics (slug, goal, status, status_note, close_sweep, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			e.Slug, e.Goal, status, e.StatusNote, e.CloseSweep, created); err != nil {
			return err //nolint:wrapcheck // the epic CHECKs speak through the mapper
		}
		if err := im.insertCriteria(ctx, tx, e); err != nil {
			return err
		}
	}
	return nil
}

// epicExists reports whether the instance already holds an epic with this slug.
func epicExists(ctx context.Context, tx *sql.Tx, slug string) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM epics WHERE slug = ?`, slug).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("epic lookup: %w", err)
	}
	return true, nil
}

// epicCreatedAt derives a deterministic epics.created_at (A6/RC-2): an
// explicit corpus field wins; else the EARLIEST available anchor — the min of
// the epic's earliest imported worklog date and (for a CLOSED epic) its
// close_sweep — so created_at never postdates close_sweep; else — an epic with
// no worklog and no close_sweep — the import moment. The old always-now()
// value re-derived on every import and could postdate a CLOSED epic's own
// close_sweep.
func (im *importer) epicCreatedAt(e epicRow, earliest map[string]string) (string, error) {
	if e.CreatedAt != "" {
		norm, err := normalizeExplicit(e.CreatedAt)
		if err != nil {
			return "", err
		}
		return norm, nil
	}
	anchor := earliest[e.Slug] // "" when the epic has no worklog
	if e.Status == epicClosed && e.CloseSweep != "" {
		if sweep, err := normalizeExplicit(e.CloseSweep); err == nil && (anchor == "" || sweep < anchor) {
			anchor = sweep
		}
	}
	if anchor != "" {
		return anchor, nil
	}
	return im.moment, nil
}

// earliestWorklog maps each epic to the earliest date among its resolved
// episodes — the deterministic seed for epicCreatedAt.
func earliestWorklog(episodes []resolvedEpisode) map[string]string {
	out := map[string]string{}
	for _, ep := range episodes {
		if cur, ok := out[ep.epic]; !ok || ep.date < cur {
			out[ep.epic] = ep.date
		}
	}
	return out
}

func (im *importer) insertCriteria(ctx context.Context, tx *sql.Tx, e epicRow) error {
	for i, cr := range e.Criteria {
		met := int64(0)
		if cr.Met {
			met = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO epic_criteria (epic, seq, criterion, met, evidence) VALUES (?, ?, ?, ?, ?)`,
			e.Slug, i+1, cr.Criterion, met, cr.Evidence); err != nil {
			return err //nolint:wrapcheck // the met/evidence CHECK speaks through the mapper
		}
	}
	return nil
}

// insertStories writes the explicit stories, then materialises a minimal
// PLANNED story for every worklog-referenced id that has none (INV-270/445);
// a `V-<n>` reference is exempt (it is a post-close validation row, not a
// story).
func (im *importer) insertStories(ctx context.Context, tx *sql.Tx,
	stories []storyRow, episodes []resolvedEpisode,
) error {
	present := map[string]bool{}
	if err := im.insertExplicitStories(ctx, tx, stories, present); err != nil {
		return err
	}
	return im.materializeStories(ctx, tx, episodes, present)
}

// insertExplicitStories writes the corpus stories[]. A row colliding with a
// live DB story is a clean refusal, not a raw UNIQUE driver string (A7).
func (im *importer) insertExplicitStories(ctx context.Context, tx *sql.Tx,
	stories []storyRow, present map[string]bool,
) error {
	for _, s := range stories {
		exists, err := storyExists(ctx, tx, s.Epic, s.ID)
		if err != nil {
			return err
		}
		if exists {
			return refuse("story-exists", "story %s/%s already exists in the instance", s.Epic, s.ID)
		}
		present[s.Epic+"\x00"+s.ID] = true
		status := s.Status
		if status == "" {
			status = storyPlanned
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO stories (epic, id, title, status, dod, consumes, produces) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			s.Epic, s.ID, s.Title, status, s.Dod, s.Consumes, s.Produces); err != nil {
			return err //nolint:wrapcheck // the story id-shape and status CHECKs speak through the mapper
		}
	}
	return nil
}

// materializeStories writes a minimal PLANNED story for every worklog-
// referenced id that has none (INV-270/445); a `V-<n>` reference is exempt.
// Backfilling episodes onto a story that already exists in the DB is
// legitimate (§10 partial migration), so an existing story is skipped
// silently rather than crashing on the UNIQUE constraint (A7).
func (im *importer) materializeStories(ctx context.Context, tx *sql.Tx,
	episodes []resolvedEpisode, present map[string]bool,
) error {
	for _, ep := range episodes {
		key := ep.epic + "\x00" + ep.story
		if strings.HasPrefix(ep.story, vRowPrefix) || present[key] {
			continue
		}
		present[key] = true
		exists, err := storyExists(ctx, tx, ep.epic, ep.story)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO stories (epic, id, title, status) VALUES (?, ?, ?, ?)`,
			ep.epic, ep.story, ep.story, storyPlanned); err != nil {
			return err //nolint:wrapcheck // the story id-shape CHECK speaks through the mapper
		}
	}
	return nil
}

// storyExists reports whether the instance already holds a (epic, id) story.
func storyExists(ctx context.Context, tx *sql.Tx, epic, id string) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM stories WHERE epic = ? AND id = ?`, epic, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("story lookup: %w", err)
	}
	return true, nil
}

// insertTasks writes each task homed (INV-448: a future increment sets its
// epic and is never parked), pairs a DUPLICATE's dup_of with its duplicates
// link (A1), and returns one import events row per imported task. R12 requires
// only the terminal ones; the extra rows are the INV-056/440 backfill marking
// (a backdated OPEN task is otherwise indistinguishable from a genuine old
// one), and R8 resolves each `#id` (A5).
func (im *importer) insertTasks(ctx context.Context, tx *sql.Tx, tasks []resolvedTask) ([]Event, error) {
	var events []Event
	for _, t := range tasks {
		status := t.in.Status
		if status == "" {
			status = statusTriage
		}
		var epicVal, dupVal any
		if t.in.Epic != "" {
			epicVal = t.in.Epic
		}
		if t.in.DupOf != 0 {
			if err := validateDupTarget(ctx, tx, t.in); err != nil {
				return nil, err
			}
			dupVal = t.in.DupOf
		}
		var id int64
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO tasks (title, status, status_note, epic, dup_of, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			t.in.Title, status, t.note, epicVal, dupVal, t.date, t.date).Scan(&id); err != nil {
			return nil, err //nolint:wrapcheck // the terminal-state CHECKs speak through the mapper
		}
		if dupVal != nil {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO task_links (from_task, to_task, type) VALUES (?, ?, 'duplicates')`,
				id, dupVal); err != nil {
				return nil, err //nolint:wrapcheck // the link CHECKs speak through the mapper
			}
		}
		events = append(events, Event{
			Entity: ref.TaskRef(id), Event: evImport,
			Detail: fmt.Sprintf("import %s (date:%s)", status, t.source),
		})
	}
	return events, nil
}

// validateDupTarget checks a DUPLICATE task's dup_of BEFORE insert: the target
// must resolve to an existing task (a corpus author cannot know a batch
// AUTOINCREMENT id, so it points at an already-present task) and must not
// itself be DUPLICATE (R7 forbids chains; this refuses one up front rather
// than surfacing a raw driver error).
func validateDupTarget(ctx context.Context, tx *sql.Tx, t taskRowIn) error {
	var targetStatus string
	err := tx.QueryRowContext(ctx, `SELECT status FROM tasks WHERE id = ?`, t.DupOf).Scan(&targetStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse("dup-target", "task %q is dup_of #%d, which does not exist", t.Title, t.DupOf)
	}
	if err != nil {
		return fmt.Errorf("dup_of lookup: %w", err)
	}
	if targetStatus == statusDuplicate {
		return refuse("dup-target", "task %q is dup_of #%d, itself DUPLICATE (no chains)", t.Title, t.DupOf)
	}
	return nil
}

// insertWorklog orders each epic's episodes chronologically (by derived
// date, stable tiebreak by input order — INV-269/443), assigns contiguous
// seq, INSERTs in that order so the contiguity trigger sees them ascending,
// and returns each epic's date-source map in seq order.
func (im *importer) insertWorklog(ctx context.Context, tx *sql.Tx,
	episodes []resolvedEpisode,
) (map[string][]string, error) {
	byEpic := map[string][]resolvedEpisode{}
	var epicOrder []string
	for _, ep := range episodes {
		if _, seen := byEpic[ep.epic]; !seen {
			epicOrder = append(epicOrder, ep.epic)
		}
		byEpic[ep.epic] = append(byEpic[ep.epic], ep)
	}
	sourceMap := map[string][]string{}
	for _, epic := range epicOrder {
		rows := byEpic[epic]
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].date != rows[j].date {
				return rows[i].date < rows[j].date
			}
			return rows[i].order < rows[j].order
		})
		base, err := maxSeq(ctx, tx, epic)
		if err != nil {
			return nil, err
		}
		for i, ep := range rows {
			seq := base + int64(i) + 1
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, note)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				epic, seq, ep.story, ep.date, ep.state, ep.commits, ep.gate, ep.review, ep.note); err != nil {
				return nil, err //nolint:wrapcheck // the contiguity trigger and CHECKs speak through the mapper
			}
			sourceMap[epic] = append(sourceMap[epic], fmt.Sprintf("%d:%s", seq, ep.source))
		}
	}
	return sourceMap, nil
}

func maxSeq(ctx context.Context, tx *sql.Tx, epic string) (int64, error) {
	var base int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM worklog WHERE epic = ?`, epic).Scan(&base); err != nil {
		return 0, fmt.Errorf("worklog seq: %w", err)
	}
	return base, nil
}

// epicEvents emits one import events row per epic that is terminal (R12's
// per-entity trail) or received worklog rows (its source map). A terminal
// epic that also received rows gets a single row carrying the map; a
// terminal epic with no rows gets an empty-detail marker; the union is
// walked in slug order so the batch is deterministic.
func epicEvents(epics []epicRow, sourceMap map[string][]string) []Event {
	need := map[string]bool{}
	for _, e := range epics {
		if epicTerminal[e.Status] {
			need[e.Slug] = true
		}
	}
	for slug := range sourceMap {
		need[slug] = true
	}
	slugs := make([]string, 0, len(need))
	for slug := range need {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	events := make([]Event, 0, len(slugs))
	for _, slug := range slugs {
		events = append(events, Event{
			Entity: epicPrefix + slug, Event: evImport,
			Detail: strings.Join(sourceMap[slug], " "),
		})
	}
	return events
}
