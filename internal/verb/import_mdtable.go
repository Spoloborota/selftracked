package verb

import (
	"regexp"
	"strconv"
	"strings"
)

// md-table section headers (§6.2): one H2 per entity kind, each followed by
// exactly one GFM pipe table. The nested criteria/increments arrays are
// JSON-only, so md-table omits them.
const (
	secPaths   = "paths"
	secEpics   = "epics"
	secStories = "stories"
	secTasks   = "tasks"
	secWorklog = "worklog"

	// minPipeRows is the smallest table that carries data columns: a header
	// row and its `--- | ---` separator. Fewer means no table.
	minPipeRows = 2
)

// mdTable is one parsed pipe table: the header column names and the data
// rows, cells already trimmed.
type mdTable struct {
	cols []string
	rows [][]string
}

// section is one `## <kind>` header with the raw lines that follow it, kept
// ordered so duplicate headers and unknown kinds can be refused (A9).
type section struct {
	kind  string
	block []string
}

// sepCell matches one GFM separator cell: dashes with optional leading/
// trailing alignment colons. Every cell of the separator row must match.
var sepCell = regexp.MustCompile(`^:?-+:?$`)

// parseMdTable reads a markdown corpus: `## <kind>` headers, each followed by
// one pipe table whose header row names the columns (the JSON field names).
// Whitespace is lenient. Every malformed structure is a loud refuse, never a
// silent drop (A9): an unknown or duplicate section, a missing/invalid
// separator row, a data row whose cell count does not match the header, an
// unknown column, an unparseable dup_of. A blank/omitted known section is
// allowed. GFM `\|` escaped pipes are deferred to S10 (the strict cell-count
// check turns their misalignment into a loud error meanwhile).
func parseMdTable(data []byte) (corpus, error) {
	if looksJSON(data) {
		return corpus{}, refuse("format", "--format md-table but the file begins with a JSON object")
	}
	var c corpus
	seen := map[string]bool{}
	for _, sec := range splitSections(string(data)) {
		if _, ok := knownColumns[sec.kind]; !ok {
			return corpus{}, refuse("format", "unknown section %q", sec.kind)
		}
		if seen[sec.kind] {
			return corpus{}, refuse("format", "duplicate section %q", sec.kind)
		}
		seen[sec.kind] = true
		tbl, err := parseTable(sec.kind, sec.block)
		if err != nil {
			return corpus{}, err
		}
		if err := c.absorb(sec.kind, tbl); err != nil {
			return corpus{}, err
		}
	}
	return c, nil
}

// splitSections walks the document, pairing each `## <kind>` header with the
// lines that follow it, preserving order and every occurrence.
func splitSections(doc string) []section {
	var out []section
	var kind string
	var block []string
	flush := func() {
		if kind != "" {
			out = append(out, section{kind: kind, block: block})
		}
		kind, block = "", nil
	}
	for line := range strings.SplitSeq(doc, "\n") {
		if header, ok := strings.CutPrefix(strings.TrimSpace(line), "## "); ok {
			flush()
			kind = strings.TrimSpace(header)
			continue
		}
		block = append(block, line)
	}
	flush()
	return out
}

// parseTable reads the pipe table in a section block: a header row, a GFM
// separator row, then data rows. A block with no pipe rows is a blank section
// (empty table). Any other malformation refuses, naming the section (A9).
func parseTable(kind string, block []string) (mdTable, error) {
	var pipes []string
	for _, line := range block {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			pipes = append(pipes, line)
		}
	}
	if len(pipes) == 0 {
		return mdTable{}, nil // a blank/omitted section is allowed
	}
	if len(pipes) < minPipeRows {
		return mdTable{}, refuse("format", "section %q has a header row but no separator row", kind)
	}
	cols := splitRow(pipes[0])
	if !isSeparatorRow(pipes[1], len(cols)) {
		return mdTable{}, refuse("format", "section %q row 2 is not a GFM separator (--- | ---)", kind)
	}
	var rows [][]string
	for _, line := range pipes[minPipeRows:] { // skip header + separator
		cells := splitRow(line)
		if len(cells) != len(cols) {
			return mdTable{}, refuse("format",
				"section %q data row %q has %d cells but the header has %d", kind, strings.TrimSpace(line), len(cells), len(cols))
		}
		rows = append(rows, cells)
	}
	return mdTable{cols: cols, rows: rows}, nil
}

// isSeparatorRow reports whether a pipe row is a GFM separator matching the
// header's column count, every cell a dashes-with-optional-colons run.
func isSeparatorRow(line string, ncols int) bool {
	cells := splitRow(line)
	if len(cells) != ncols {
		return false
	}
	for _, cell := range cells {
		if !sepCell.MatchString(strings.TrimSpace(cell)) {
			return false
		}
	}
	return true
}

// splitRow splits one `| a | b |` line into trimmed cells, dropping the
// empty edges the outer pipes create.
func splitRow(line string) []string {
	parts := strings.Split(strings.TrimSpace(line), "|")
	if len(parts) > 0 && strings.TrimSpace(parts[0]) == "" {
		parts = parts[1:]
	}
	if len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// absorb turns one parsed table into corpus rows for its kind, rejecting an
// unknown column name.
func (c *corpus) absorb(kind string, tbl mdTable) error {
	for _, r := range tbl.rows {
		cells, err := zip(kind, tbl.cols, r)
		if err != nil {
			return err
		}
		if len(cells) == 0 {
			continue
		}
		if err := c.appendRow(kind, cells); err != nil {
			return err
		}
	}
	return nil
}

func (c *corpus) appendRow(kind string, cells map[string]string) error {
	switch kind {
	case secPaths:
		c.Paths = append(c.Paths, pathRow{
			Class: cells["class"], Scope: cells["scope"], Root: cells["root"],
			Ephemeral: truthy(cells["ephemeral"]), Note: cells["note"],
		})
	case secEpics:
		c.Epics = append(c.Epics, epicRow{
			Slug: cells["slug"], Goal: cells["goal"], Status: cells["status"],
			StatusNote: cells["status_note"], CloseSweep: cells["close_sweep"],
			CreatedAt: cells["created_at"],
		})
	case secStories:
		c.Stories = append(c.Stories, storyRow{
			Epic: cells["epic"], ID: cells["id"], Title: cells["title"], Status: cells["status"],
			Dod: cells["dod"], Consumes: cells["consumes"], Produces: cells["produces"],
		})
	case secTasks:
		return c.appendTask(cells)
	case secWorklog:
		c.appendWorklogRow(cells)
	default:
		return refuse("format", "unknown md-table section %q", kind)
	}
	return nil
}

func (c *corpus) appendTask(cells map[string]string) error {
	var dup int64
	if raw := strings.TrimSpace(cells["dup_of"]); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return refuse("format", "task %q has an unparseable dup_of %q", cells["title"], raw)
		}
		dup = v
	}
	c.Tasks = append(c.Tasks, taskRowIn{
		Title: cells["title"], Status: cells["status"], Note: cells["note"], Epic: cells["epic"],
		Date: cells["date"], DupOf: dup, FutureIncrement: truthy(cells["future_increment"]),
		PointerNote: cells["pointer_note"], OwnerSteer: cells["owner_steer"],
	})
	return nil
}

func (c *corpus) appendWorklogRow(cells map[string]string) {
	c.Worklog = append(c.Worklog, worklogRow{
		Epic: cells["epic"], Story: cells["story"], State: cells["state"], Commits: cells["commits"],
		Gate: cells["gate"], Review: cells["review"], Date: cells["date"], Note: cells["note"],
		LegacyReason: cells["legacy_reason"],
	})
}

// zip pairs a data row's cells with the header columns, rejecting a column
// name that is not a known field for the kind. A row of only empty cells is
// treated as blank (returns an empty map).
func zip(kind string, cols, cells []string) (map[string]string, error) {
	known := knownColumns[kind]
	out := map[string]string{}
	filled := false
	for i, col := range cols {
		if _, ok := known[col]; !ok {
			return nil, refuse("format", "%s table has unknown column %q", kind, col)
		}
		var v string
		if i < len(cells) {
			v = cells[i]
		}
		if v != "" {
			filled = true
		}
		out[col] = v
	}
	if !filled {
		return map[string]string{}, nil
	}
	return out, nil
}

func truthy(s string) bool { return strings.EqualFold(strings.TrimSpace(s), "true") }

// knownColumns is the closed per-kind column vocabulary the md-table header
// is checked against — same names as the JSON fields.
var knownColumns = map[string]map[string]struct{}{
	secPaths:   columnSet("class", "scope", "root", "ephemeral", "note"),
	secEpics:   columnSet("slug", "goal", "status", "status_note", "close_sweep", "created_at"),
	secStories: columnSet("epic", "id", "title", "status", "dod", "consumes", "produces"),
	secTasks: columnSet("title", "status", "note", "epic", "date", "dup_of",
		"future_increment", "pointer_note", "owner_steer"),
	secWorklog: columnSet("epic", "story", "state", "commits", "gate", "review", "date", "note", "legacy_reason"),
}

func columnSet(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, n := range names {
		out[n] = struct{}{}
	}
	return out
}
