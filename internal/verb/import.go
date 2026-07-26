package verb

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
)

// evImport is the events name every import row carries (§5.9). R12 accepts
// it as the terminal-state trail, and the per-epic row's detail is the
// date-source map.
const evImport = "import"

const (
	formatJSON    = "json"
	formatMdTable = "md-table"

	wlDone     = statusDone
	vRowPrefix = "V-"
)

// ImportVerbs returns the S9 `import` catalog entry — the one sanctioned
// backfill door (§6.2, §10). It does not open a second write path: it reuses
// the shared Write pipeline, doing raw INSERTs in one transaction so every
// schema trigger fires exactly as for a live write. `--legacy` relaxes three
// inputs (synthesized timestamps, `legacy:` commit values, terminal-state
// INSERTs) and nothing else — all three gated by the single legacy bool.
func ImportVerbs() []cli.Verb {
	var file, format string
	var legacy bool
	return []cli.Verb{{Name: evImport, Subs: []cli.Sub{{
		Arity: 0,
		Usage: "import --file F [--format md-table|json] [--legacy]",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&file, "file", "", "the corpus file to import (required)")
			fs.StringVar(&format, "format", "", "md-table|json (default: inferred from the file)")
			fs.BoolVar(&legacy, "legacy", false, "relax historical inputs: synthesized dates, legacy: commits, terminal states")
		},
		Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
			return runImport(e, file, format, legacy)
		},
	}}}}
}

// ---- corpus model ----

// corpus is the one internal shape both formats parse into. Field names are
// snake_case, matching the tagliatelle json:snake rule.
type corpus struct {
	Paths   []pathRow    `json:"paths"`
	Epics   []epicRow    `json:"epics"`
	Stories []storyRow   `json:"stories"`
	Tasks   []taskRowIn  `json:"tasks"`
	Worklog []worklogRow `json:"worklog"`
}

type pathRow struct {
	Class     string `json:"class"`
	Scope     string `json:"scope"`
	Root      string `json:"root"`
	Ephemeral bool   `json:"ephemeral"`
	Note      string `json:"note"`
}

// The criterion text carries one name on both doors and one historical
// alias (§6.2 "One field, one name", amendment
// `one-name-for-the-criterion-field`): `text` is the name the `criteria
// add` flag and the corpus field share, `criterion` is the spelling the
// corpus shipped with and keeps accepting. The column stays
// `epic_criteria.criterion` under either — this is a vocabulary change,
// not a schema one.
const (
	criterionName  = "text"
	criterionAlias = "criterion"
)

// criterionRow declares BOTH spellings because the decoder runs
// `DisallowUnknownFields` (parseJSON), which accepts only declared keys.
// They are `*string` rather than `string` because §6.2 makes a row that
// carries both with DIFFERENT values a `usage` refusal, and that refusal
// has to tell an absent key from a key present with an empty value —
// `{"text":"","criterion":"x"}` is a contradiction, `{"criterion":"x"}`
// is not, and two plain strings render the two cases identically.
type criterionRow struct {
	Text      *string `json:"text"`
	Criterion *string `json:"criterion"`
	Met       bool    `json:"met"`
	Evidence  string  `json:"evidence"`
}

// text returns the criterion under whichever spelling carried it.
// `validateCriteriaSpelling` has already refused a row carrying both with
// different values, so the first non-nil pointer is the whole answer.
// Neither present yields the empty string, which the schema's non-empty
// CHECK on `epic_criteria.criterion` refuses — the same refusal an absent
// `criterion` key produced before the alias existed.
func (cr criterionRow) text() string {
	if cr.Text != nil {
		return *cr.Text
	}
	if cr.Criterion != nil {
		return *cr.Criterion
	}
	return ""
}

type epicRow struct {
	Slug       string         `json:"slug"`
	Goal       string         `json:"goal"`
	Status     string         `json:"status"`
	StatusNote string         `json:"status_note"`
	CloseSweep string         `json:"close_sweep"`
	CreatedAt  string         `json:"created_at"`
	Criteria   []criterionRow `json:"criteria"`
}

type storyRow struct {
	Epic     string `json:"epic"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Dod      string `json:"dod"`
	Consumes string `json:"consumes"`
	Produces string `json:"produces"`
}

type taskRowIn struct {
	Title           string `json:"title"`
	Status          string `json:"status"`
	Note            string `json:"note"`
	Epic            string `json:"epic"`
	Date            string `json:"date"`
	DupOf           int64  `json:"dup_of"`
	FutureIncrement bool   `json:"future_increment"`
	PointerNote     string `json:"pointer_note"`
	OwnerSteer      string `json:"owner_steer"`
}

type incrementRow struct {
	Commits string `json:"commits"`
	Gate    string `json:"gate"`
	Note    string `json:"note"`
}

type worklogRow struct {
	Epic         string         `json:"epic"`
	Story        string         `json:"story"`
	State        string         `json:"state"`
	Commits      string         `json:"commits"`
	Gate         string         `json:"gate"`
	Review       string         `json:"review"`
	Date         string         `json:"date"`
	Note         string         `json:"note"`
	LegacyReason string         `json:"legacy_reason"`
	Increments   []incrementRow `json:"increments"`
}

// importer carries the resolution context: the single import moment (used
// for import-time dates and the future bound), the legacy switch, the source
// file's earliest-commit bound, and the accumulating stderr reports.
type importer struct {
	moment      string
	legacy      bool
	sourceFirst string
	haveBound   bool
	warnings    []string
}

// runImport parses, resolves dates, then commits the whole batch in one
// Write. Git resolution (read-only, external) happens BEFORE Write, so the
// transaction only INSERTs already-computed rows.
func runImport(e *cli.Env, file, format string, legacy bool) error {
	if file == "" {
		return refuse("usage", "import requires --file")
	}
	data, err := os.ReadFile(file) //nolint:gosec // the corpus path is an explicit operator argument
	if err != nil {
		return refuse("not-found", "cannot read %s: %v", file, err)
	}
	c, reader, err := parseCorpus(data, format)
	if err != nil {
		return err
	}
	// The empty-corpus refusal (§6.2) sits HERE, above validation and above
	// both readers: each reader can hand back an understood-but-empty corpus
	// (md-table with no recognized heading, json decoding `{}`), so a fix
	// inside one would leave the other standing.
	if c.rows() == 0 {
		return refuseEmptyCorpus(data, reader, file)
	}
	if err := c.validate(legacy); err != nil {
		return err
	}
	ctx := context.Background()
	im := &importer{moment: now(), legacy: legacy}
	im.sourceFirst, im.haveBound = firstCommitDate(ctx, file)

	episodes, err := im.resolveWorklog(ctx, c.Worklog)
	if err != nil {
		return err
	}
	tasks, err := im.resolveTasks(c.Tasks)
	if err != nil {
		return err
	}

	if err := Write(ctx, func(tx *sql.Tx) ([]Event, error) {
		return im.insert(ctx, tx, c, episodes, tasks)
	}); err != nil {
		return err
	}
	// Warnings/reports (an INV-549 pre-first-commit report, a date
	// disagreement) describe rows that DID commit, so they are flushed only
	// after Write succeeds (A8) — a failed import prints none.
	for _, w := range im.warnings {
		_, _ = fmt.Fprintln(e.Stderr, w)
	}
	// All five kinds, path-dictionary rows included: after the empty-corpus
	// refusal above, a paths-only import is the only remaining green exit,
	// and a four-counter line would report it as four zeros.
	_, _ = fmt.Fprintf(e.Stdout,
		"imported %d path(s), %d epic(s), %d story(ies), %d task(s), %d worklog row(s) from %s\n",
		len(c.Paths), len(c.Epics), len(c.Stories), len(c.Tasks), len(episodes), file)
	return nil
}

// rows counts every row the corpus would insert, across every section the
// importer writes. The four entity counters are NOT this test: a paths-only
// corpus leaves all four at zero and is a legitimate import.
func (c *corpus) rows() int {
	return len(c.Paths) + len(c.Epics) + len(c.Stories) + len(c.Tasks) + len(c.Worklog)
}

// corpusSections is the closed section vocabulary both readers share, in the
// order the importer writes them. It is quoted back by the empty-corpus
// refusal; `TestCorpusSectionsMatchColumns` pins it against `knownColumns`,
// so a section added to the readers cannot go missing from the message.
var corpusSections = []string{secPaths, secEpics, secStories, secTasks, secWorklog}

// refuseEmptyCorpus states the §6.2 rule that a batch verb's green exit is a
// claim the batch landed. It distinguishes the two reasons a corpus can be
// empty because they call for different actions: a file nothing was
// recognized in is usually a mis-cased heading or a bare table with none
// above it, and the reader's own vocabulary is what redirects its author; a
// file whose sections were all found and all empty has no syntax problem to
// hunt for.
func refuseEmptyCorpus(data []byte, reader, file string) error {
	if recognizedSections(data, reader) == 0 {
		return refuse("format",
			"%s: no %s section was recognized, so no row was imported; %s",
			file, reader, sectionVocabulary(reader))
	}
	return refuse("format",
		"%s: every %s section in it is empty, so no row was imported; "+
			"import inserts rows and refuses a corpus that carries none",
		file, reader)
}

// sectionVocabulary spells the reader's accepted section names the way that
// reader spells them, so the refusal shows the exact text a corpus needs.
func sectionVocabulary(reader string) string {
	if reader == formatMdTable {
		return `md-table reads the lowercase headings "## ` +
			strings.Join(corpusSections, `", "## `) +
			`", each followed by one pipe table`
	}
	return `json reads the object keys "` + strings.Join(corpusSections, `", "`) + `"`
}

// recognizedSections counts the sections the reader that ran found in the
// file, whether or not they carried rows. It re-reads the bytes rather than
// being threaded out of the parsers: it runs only on the already-refused
// empty-corpus path, and the readers stay exactly as they are.
func recognizedSections(data []byte, reader string) int {
	if reader == formatMdTable {
		n := 0
		for _, sec := range splitSections(string(data)) {
			if _, ok := knownColumns[sec.kind]; ok {
				n++
			}
		}
		return n
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return 0
	}
	n := 0
	for k := range keys {
		if _, ok := knownColumns[k]; ok {
			n++
		}
	}
	return n
}

// parseCorpus selects a parser: an explicit --format is honoured (and a
// content mismatch refused by the parser itself); with no flag the format is
// inferred — a leading `{` means json, else md-table. It returns the reader
// that actually ran: with no --format the file was read as a format nobody
// typed, and the empty-corpus refusal has to name the one it used.
func parseCorpus(data []byte, format string) (corpus, string, error) {
	switch format {
	case formatJSON:
		c, err := parseJSON(data)
		return c, formatJSON, err
	case formatMdTable:
		c, err := parseMdTable(data)
		return c, formatMdTable, err
	case "":
		if looksJSON(data) {
			c, err := parseJSON(data)
			return c, formatJSON, err
		}
		c, err := parseMdTable(data)
		return c, formatMdTable, err
	}
	return corpus{}, "", refuse("usage", "unknown --format %q (want json or md-table)", format)
}

func looksJSON(data []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(data)), "{")
}

func parseJSON(data []byte) (corpus, error) {
	if !looksJSON(data) {
		return corpus{}, refuse("format", "--format json but the file does not begin with a JSON object")
	}
	var c corpus
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		if field, ok := unknownJSONField(err); ok && criteriaOnlyKey(data, field) {
			// §6.2: the corpus's unknown-field refusal names BOTH accepted
			// spellings, for the reader who guessed a third. The generic
			// stdlib message names only the key it rejected, which is the
			// one thing the author already knows.
			//
			// The rejected key is BOUNDED before it rides %q. A JSON key has
			// no character-class restriction and no length limit, and this
			// refusal is read by an agent, not only by a person: an unbounded
			// echo lets a corpus paste an arbitrary payload into that agent's
			// context. Same helper, same reason, as the identifier refusals
			// in `refuseIdentifier`.
			return corpus{}, refuse("format",
				"invalid import JSON: unknown criteria field %q; the criterion text is %q — "+
					"the same name `criteria add --%s` writes it under — and %q, the corpus's "+
					"historical spelling, is still accepted",
				bounded(field), criterionName, criterionName, criterionAlias)
		}
		if field, ok := unknownJSONField(err); ok {
			// The redirect above did not apply — the key is not unique to
			// criteria objects — but the flood door is the same one. The
			// stdlib's message embeds the rejected key whole, so a corpus
			// carrying a 300 KB key anywhere the redirect does not reach
			// still pastes 300 KB into the reader's context; measured at
			// 300086 bytes before this branch existed. Only the
			// unknown-field shape is rebuilt: every other decode error is
			// short and names an offset, and truncating those would cost a
			// diagnostic to buy nothing.
			return corpus{}, refuse("format", "invalid import JSON: unknown field %q", bounded(field))
		}
		return corpus{}, refuse("format", "invalid import JSON: %v", err)
	}
	return c, nil
}

// unknownFieldPrefix is encoding/json's wording for the error
// `DisallowUnknownFields` produces. It is matched as text because the
// stdlib returns a bare `*errors.errorString` there — no typed error, no
// field path — and `TestUnknownJSONFieldMatchesStdlib` pins the wording, so
// a toolchain that reworded it fails a test instead of silently dropping
// the criterion redirect back to the generic message.
const unknownFieldPrefix = `json: unknown field `

// unknownJSONField recovers the rejected key's name from that error.
func unknownJSONField(err error) (string, bool) {
	_, quoted, found := strings.Cut(err.Error(), unknownFieldPrefix)
	if !found {
		return "", false
	}
	name, uerr := strconv.Unquote(quoted)
	if uerr != nil {
		return "", false
	}
	return name, true
}

// criteriaKeyPath is the one place in the corpus the criterion redirect is
// about, written as the key path a generic walk reaches it by, array indices
// elided: an object at `epics[].criteria[]`.
const criteriaKeyPath = "epics.criteria"

// criteriaOnlyKey reports whether `field` — the key encoding/json rejected —
// is carried by some `epics[].criteria[]` object AND by nothing else in the
// document. That conjunction is what scopes the criterion redirect honestly.
//
// Both halves are load-bearing, because the stdlib error carries the rejected
// key's NAME and not its path (see `unknownJSONField`), so the name is the
// only evidence there is. A name that also sits on a task, a story or a
// worklog row does not say which occurrence was rejected — a `titel` typo on
// a task, in a corpus whose criteria rows happen to carry a `titel` too,
// would otherwise be answered with a lecture about the criterion's spellings.
// Where the scope cannot be established the plain refusal is the honest one.
//
// It reads the document GENERICALLY rather than into a criteria-shaped
// value. A shaped decode fails on any unrelated malformation anywhere — one
// epic whose `criteria` entry is a string rather than an object, say — and
// would then drop the redirect from a genuine criteria typo in a different
// epic, which is a defect no reader could attribute to its cause.
func criteriaOnlyKey(data []byte, field string) bool {
	// An accepted spelling that was rejected as unknown was rejected
	// somewhere the criteria vocabulary does not apply — a task row, say —
	// and "unknown criteria field `criterion` … and `criterion` is still
	// accepted" is a sentence that cannot be made true. The plain message is
	// the only correct one here whatever else the document carries.
	if field == criterionName || field == criterionAlias {
		return false
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	var inCriteria, elsewhere bool
	scanForKey(doc, "", field, &inCriteria, &elsewhere)
	return inCriteria && !elsewhere
}

// scanForKey walks a decoded JSON document and records where `field` occurs
// as an object key: inside an `epics[].criteria[]` object, or anywhere else.
// `path` is the dotted key path of the node being visited, with array levels
// transparent, so every criteria object — and only those — is visited at
// `criteriaKeyPath`.
func scanForKey(node any, path, field string, inCriteria, elsewhere *bool) {
	switch v := node.(type) {
	case map[string]any:
		if _, ok := v[field]; ok {
			if path == criteriaKeyPath {
				*inCriteria = true
			} else {
				*elsewhere = true
			}
		}
		for key, child := range v {
			next := key
			if path != "" {
				next = path + "." + key
			}
			scanForKey(child, next, field, inCriteria, elsewhere)
		}
	case []any:
		for _, child := range v {
			scanForKey(child, path, field, inCriteria, elsewhere)
		}
	}
}
