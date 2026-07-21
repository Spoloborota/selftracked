package verb

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
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

type criterionRow struct {
	Criterion string `json:"criterion"`
	Met       bool   `json:"met"`
	Evidence  string `json:"evidence"`
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
	c, err := parseCorpus(data, format)
	if err != nil {
		return err
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
	_, _ = fmt.Fprintf(e.Stdout, "imported %d epic(s), %d story(ies), %d task(s), %d worklog row(s) from %s\n",
		len(c.Epics), len(c.Stories), len(c.Tasks), len(episodes), file)
	return nil
}

// parseCorpus selects a parser: an explicit --format is honoured (and a
// content mismatch refused by the parser itself); with no flag the format is
// inferred — a leading `{` means json, else md-table.
func parseCorpus(data []byte, format string) (corpus, error) {
	switch format {
	case formatJSON:
		return parseJSON(data)
	case formatMdTable:
		return parseMdTable(data)
	case "":
		if looksJSON(data) {
			return parseJSON(data)
		}
		return parseMdTable(data)
	}
	return corpus{}, refuse("usage", "unknown --format %q (want json or md-table)", format)
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
		return corpus{}, refuse("format", "invalid import JSON: %v", err)
	}
	return c, nil
}
