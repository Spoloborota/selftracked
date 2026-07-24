package verb

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// The date-source codes the per-epic source map speaks (§6.2): git-derived,
// explicit field, import-time synthesized. One code per worklog row.
const (
	srcGit      = "g"
	srcExplicit = "e"
	srcImport   = "i"

	// dayLen is the length of a canonical date prefix "2006-01-02" — the
	// calendar-day granularity INV-261's disagreement test compares at.
	dayLen = 10

	// canonTime is the one stored timestamp shape (matches now()): ISO-8601
	// UTC. Every derived date is normalised to it so R10's lexical compare
	// and the dump grammar see one format (the §5 write-time guarantee).
	canonTime = "2006-01-02T15:04:05Z"
	dateOnly  = "2006-01-02"

	legacyPrefix = "legacy: "
)

// shaShape matches a bare git object name: hex, 7–40 chars — the abbreviated
// and full forms both resolve. A token outside this shape is prose, never a
// commit. A range token is split first, so this only tests a single endpoint.
var shaShape = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// commitClass is what a commits cell is, once classified (A4): the four
// mutually exclusive shapes that decide both git resolution and what is
// stored. The distinction defeats a `legacy:` value masquerading as a
// citation (its hex-shaped remainder would otherwise resolve) and keeps
// prose out of the commits column (so R5 never runs `git cat-file` on a
// stray word).
type commitClass int

const (
	ccEmpty       commitClass = iota // blank after trim
	ccLegacy                         // begins with `legacy:` — a relaxation, not a citation
	ccCitation                       // every token is a sha-shape or an a..b range of sha-shapes
	ccPlaceholder                    // ≥1 token but not a citation (prose, a stray number, mixed)
)

// classifyCommits sorts a commits cell into its shape (A4). A citation is
// the ONLY shape git resolution touches and the ONLY shape stored as tokens;
// everything else is legacy:, a dropped placeholder, or empty.
func classifyCommits(cell string) commitClass {
	trimmed := strings.TrimSpace(cell)
	if trimmed == "" {
		return ccEmpty
	}
	if strings.HasPrefix(trimmed, "legacy:") {
		return ccLegacy
	}
	toks := commitTokens(cell)
	if len(toks) == 0 {
		return ccEmpty
	}
	for _, tok := range toks {
		if !isCitationToken(tok) {
			return ccPlaceholder
		}
	}
	return ccCitation
}

// isCitationToken reports whether one token is a bare sha-shape or an `a..b`
// range whose BOTH endpoints are sha-shapes. A range with a non-sha endpoint
// is not a citation token (so the whole cell falls to placeholder).
func isCitationToken(tok string) bool {
	before, after, isRange := strings.Cut(tok, "..")
	if !isRange {
		return shaShape.MatchString(tok)
	}
	return shaShape.MatchString(strings.Trim(before, ".")) &&
		shaShape.MatchString(strings.Trim(after, "."))
}

// gitDate resolves a citation cell to its git author date (INV-260, INV-262):
// the newest resolvable author date wins — a multi-commit or `a..b` citation
// dates the increment by its finish. author date, never committer, so a
// rebase does not rewrite worklog chronology. A legacy/placeholder/empty cell
// resolves no date.
// It also reports the sha-shaped endpoints that resolved to nothing. Those
// are stored verbatim by design (§6.2: rewriting a typo to `legacy:` would
// mask it), and R5 flags them in full verify — but a corpus of a hundred rows
// makes "run verify afterwards" a weak promise, so the importer names them as
// it goes.
func (im *importer) gitDate(ctx context.Context, cell string, kind commitClass) (string, []string) {
	if kind != ccCitation {
		return "", nil
	}
	var date string
	var unresolved []string
	for _, tok := range commitTokens(cell) {
		endpoint := tok
		if _, after, isRange := strings.Cut(tok, ".."); isRange {
			endpoint = strings.Trim(after, ".")
		}
		d, ok := im.authorDate(ctx, endpoint)
		switch {
		case !ok:
			unresolved = append(unresolved, endpoint)
		case d > date:
			date = d
		}
	}
	return date, unresolved
}

// commitTokens splits a commits cell on whitespace and commas; a `a..b`
// range stays one token (the `..` is not a separator).
func commitTokens(cell string) []string {
	fields := strings.FieldsFunc(cell, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ',' || r == '\n'
	})
	return fields
}

// authorDate runs `git show -s --format=%aI` for one object, read-only, from
// the working directory. An object that does not resolve (a typo) returns
// ok=false and contributes no date; the caller leaves it verbatim.
func (im *importer) authorDate(ctx context.Context, sha string) (string, bool) {
	// sha is a matched hex token, never interpolated into a shell.
	out, err := exec.CommandContext(ctx, //nolint:gosec // fixed argv, hex token
		"git", "show", "-s", "--format=%aI", sha+"^{commit}").Output()
	if err != nil {
		return "", false
	}
	norm, err := normalizeGit(strings.TrimSpace(string(out)))
	if err != nil {
		return "", false
	}
	return norm, true
}

// firstCommitDate is the earliest author date of a commit touching the
// source file — INV-549's lower bound. An untracked file yields ok=false and
// the bound is skipped (you cannot bound what git does not know). git-log is
// topological, not author-date sorted (a rebase/squash — the exact history
// this verb imports — reorders author dates against parentage), so the bound
// is the MIN over every line, never the last line (A3).
func firstCommitDate(ctx context.Context, file string) (string, bool) {
	// file is the operator's explicit --file argument, passed as one argv.
	out, err := exec.CommandContext(ctx, //nolint:gosec // fixed argv, operator path
		"git", "log", "--format=%aI", "--follow", "--", file).Output()
	if err != nil {
		return "", false
	}
	var earliest string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		norm, err := normalizeGit(line)
		if err != nil {
			continue
		}
		if earliest == "" || norm < earliest {
			earliest = norm
		}
	}
	if earliest == "" {
		return "", false
	}
	return earliest, true
}

// normalizeGit parses a git %aI author date (RFC3339 with offset) and
// re-renders it in canonical UTC, so every stored date is one shape.
func normalizeGit(s string) (string, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return "", refuse("date", "unparseable git author date %q", s)
	}
	return t.UTC().Format(canonTime), nil
}

// normalizeExplicit parses a corpus `date` field — a plain calendar day, or
// a full RFC3339 stamp — into canonical UTC. A day-only value dates to
// midnight UTC.
func normalizeExplicit(s string) (string, error) {
	if t, err := time.Parse(dateOnly, s); err == nil {
		return t.UTC().Format(canonTime), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(canonTime), nil
	}
	return "", refuse("date", "unparseable date %q (want YYYY-MM-DD)", s)
}

// sameDay reports whether two canonical timestamps fall on the same UTC
// calendar day — INV-261 compares days, not instants, because the modal
// contamination is an adjacent-day midnight drift.
func sameDay(a, b string) bool {
	if len(a) < dayLen || len(b) < dayLen {
		return a == b
	}
	return a[:dayLen] == b[:dayLen]
}

// resolvedEpisode is one worklog row after date derivation and increment
// splitting: the values that get INSERTed, plus the source code the per-epic
// map records and the input order that breaks a same-date tie.
type resolvedEpisode struct {
	epic, story, state          string
	commits, gate, review, note string
	legacyReason                string
	date, source                string
	order                       int
}

// resolvedTask is one task after date derivation and note folding.
type resolvedTask struct {
	in     taskRowIn
	date   string
	source string
	note   string
}

// resolveWorklog splits bundled increments (INV-444) into one episode each,
// then derives each episode's date, commits and note (§6.2). A fatal error
// (future date, or a relaxation used without --legacy) aborts the whole
// import before any write.
func (im *importer) resolveWorklog(ctx context.Context, rows []worklogRow) ([]resolvedEpisode, error) {
	var out []resolvedEpisode
	order := 0
	for _, r := range rows {
		for _, ep := range splitIncrements(r) {
			ep.order = order
			order++
			resolved, err := im.resolveEpisode(ctx, ep)
			if err != nil {
				return nil, err
			}
			out = append(out, resolved)
		}
	}
	return out, nil
}

// splitIncrements turns one corpus worklog entry into its episodes: a row
// bundling increments becomes one episode per increment (each with its own
// commits/gate/note, the parent's state/story/review/date inherited); a row
// without increments is one episode.
func splitIncrements(r worklogRow) []resolvedEpisode {
	base := resolvedEpisode{
		epic: r.Epic, story: r.Story, state: r.State,
		review: r.Review, date: r.Date, note: r.Note, legacyReason: r.LegacyReason,
	}
	if len(r.Increments) == 0 {
		base.commits, base.gate = r.Commits, r.Gate
		return []resolvedEpisode{base}
	}
	out := make([]resolvedEpisode, 0, len(r.Increments))
	for _, inc := range r.Increments {
		ep := base
		ep.commits, ep.gate = inc.Commits, inc.Gate
		if inc.Note != "" {
			ep.note = inc.Note
		}
		out = append(out, ep)
	}
	return out
}

// resolveEpisode runs the git-first date priority (INV-260), records a
// calendar-day disagreement (INV-261), applies the two bounds (INV-548/549),
// and settles the stored commits value (verbatim sha / legacy: / dropped
// placeholder) under the legacy switch.
func (im *importer) resolveEpisode(ctx context.Context, ep resolvedEpisode) (resolvedEpisode, error) {
	kind := classifyCommits(ep.commits)
	gitDate, unresolved := im.gitDate(ctx, ep.commits, kind)
	for _, tok := range unresolved {
		im.warnf("warning: worklog %s/%s cites %s, which resolves to no commit; stored verbatim (verify reports it as R5)",
			ep.epic, ep.story, tok)
	}

	commits, err := im.storedCommits(ep.commits, ep.state, kind, ep.legacyReason)
	if err != nil {
		return ep, err
	}
	ep.commits = commits

	explicit, err := normalizeMaybe(ep.date)
	if err != nil {
		return ep, err
	}
	ep.date, ep.source = im.pickDate(gitDate, explicit)
	// §6.2: only a SYNTHESIZED import-time date is a --legacy relaxation. A
	// git-derived or explicit date is admitted without --legacy (an explicit
	// date field is not one of the three relaxations).
	if ep.source == srcImport && !im.legacy {
		return ep, refuse("legacy-required",
			"worklog %s/%s has no git or explicit date; a synthesized import-time date needs --legacy",
			ep.epic, ep.story)
	}
	if gitDate != "" && explicit != "" && !sameDay(gitDate, explicit) {
		im.warnf("warning: worklog %s/%s explicit date %s disagrees with git %s (git used)",
			ep.epic, ep.story, explicit[:dayLen], gitDate[:dayLen])
		ep.note = appendNote(ep.note,
			fmt.Sprintf("[explicit date %s disagreed; git %s used]", explicit[:dayLen], gitDate[:dayLen]))
	}
	if err := im.checkBounds(ep.source, ep.date, "worklog "+ep.epic+"/"+ep.story); err != nil {
		return ep, err
	}
	return ep, nil
}

// storedCommits decides what lands in the commits column, by the cell's
// class (A4):
//   - legacy → the canonical `legacy: <reason>` (one space after the colon,
//     fixing A10), admitted only under --legacy.
//   - citation → its tokens re-joined with a single space (canonical
//     whitespace). A lone unresolvable sha-shaped token is a typo kept
//     verbatim so R5 flags it (§6.2, INV-262); prose never reaches here.
//   - placeholder/empty → not a citation. A DONE row becomes `legacy:
//     <reason>` under --legacy (INV-441) and is refused without it; a
//     non-DONE row records no range (empty).
func (im *importer) storedCommits(cell, state string, kind commitClass, reason string) (string, error) {
	switch kind {
	case ccLegacy:
		if !im.legacy {
			return "", refuse("legacy-required", "a legacy: commits value needs --legacy")
		}
		rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cell), "legacy:"))
		return legacyPrefix + rest, nil
	case ccCitation:
		return strings.Join(commitTokens(cell), " "), nil
	case ccPlaceholder, ccEmpty:
		if state == wlDone {
			if !im.legacy {
				return "", refuse("legacy-required",
					"DONE worklog row cites no resolvable commit range; recording it as legacy: needs --legacy")
			}
			if reason == "" {
				reason = "imported without commit range"
			}
			return legacyPrefix + reason, nil
		}
		return "", nil
	}
	return "", nil // unreachable: the switch is exhaustive over commitClass
}

// pickDate is the git-first priority: git, else explicit, else import-time.
func (im *importer) pickDate(gitDate, explicit string) (string, string) {
	switch {
	case gitDate != "":
		return gitDate, srcGit
	case explicit != "":
		return explicit, srcExplicit
	default:
		return im.moment, srcImport
	}
}

// checkBounds applies the two honest bounds to a date the engine did NOT
// derive from git (INV-548/549): a future date fails the import; a date
// before the source file's first commit is reported and the import proceeds.
func (im *importer) checkBounds(source, date, label string) error {
	if source == srcGit {
		return nil
	}
	if date > im.moment {
		return refuse("future-date", "%s dates %s, later than the import moment %s — a future date is not an event date",
			label, date[:dayLen], im.moment[:dayLen])
	}
	if im.haveBound && date < im.sourceFirst {
		im.warnf("report: %s dates %s, before the source file's first commit %s (INV-549 bound)",
			label, date[:dayLen], im.sourceFirst[:dayLen])
	}
	return nil
}

// resolveTasks derives each task's date (explicit or import-time only — a
// task has no commits cell, INV-267) and folds pointer/steer fields into the
// note (INV-451/452).
func (im *importer) resolveTasks(rows []taskRowIn) ([]resolvedTask, error) {
	out := make([]resolvedTask, 0, len(rows))
	for _, r := range rows {
		explicit, err := normalizeMaybe(r.Date)
		if err != nil {
			return nil, err
		}
		date, source := im.moment, srcImport
		if explicit != "" {
			date, source = explicit, srcExplicit
		}
		// §6.2: only a SYNTHESIZED import-time date is a --legacy relaxation.
		// A task WITH an explicit date is admitted by a plain import; a
		// dateless task falls to the import moment and needs --legacy.
		if source == srcImport && !im.legacy {
			return nil, refuse("legacy-required",
				"task %q has no explicit date; a synthesized import-time date needs --legacy", r.Title)
		}
		if err := im.checkBounds(source, date, fmt.Sprintf("task %q", r.Title)); err != nil {
			return nil, err
		}
		out = append(out, resolvedTask{in: r, date: date, source: source, note: foldTaskNote(r)})
	}
	return out, nil
}

// foldTaskNote lands a non-file pointer and an owner steer as note content,
// never as a typed link or a synthesized unblock event (INV-451/452).
func foldTaskNote(r taskRowIn) string {
	note := r.Note
	if r.PointerNote != "" {
		note = appendNote(note, "[pointer: "+r.PointerNote+"]")
	}
	if r.OwnerSteer != "" {
		note = appendNote(note, "[owner steer: "+r.OwnerSteer+"]")
	}
	return note
}

// normalizeMaybe normalises an optional date field: empty stays empty.
func normalizeMaybe(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	return normalizeExplicit(s)
}

func appendNote(note, add string) string {
	if note == "" {
		return add
	}
	return note + " " + add
}

func (im *importer) warnf(format string, args ...any) {
	im.warnings = append(im.warnings, fmt.Sprintf(format, args...))
}
