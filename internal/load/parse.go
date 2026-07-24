// Package load is the §8.5 loader: the whitelist parser (the security
// boundary — dump text is never piped into a general SQL interpreter),
// the hardened temp-file build, the load guard, and the atomic rename.
package load

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Literal is one whitelisted value token: nil, int64, or string. There is
// no expression grammar at all — that absence is the security property.
type Literal any

// Insert is one whitelisted data statement.
type Insert struct {
	Table  string
	Values []Literal
}

// Dump is a parsed, whitelist-validated dump.
type Dump struct {
	Version int
	Inserts []Insert
}

// ErrRefused wraps every parse refusal: exit 2 territory (§8.5), one
// error family so the verb can classify without matching text.
var ErrRefused = errors.New("dump refused")

const headerPrefix = "-- selftracked dump schema_version="

// expectedColumns mirrors the serializer's per-table column lists — the
// parser accepts EXACTLY these, in exactly this order, nothing else.
var expectedColumns = map[string]string{
	"meta":            "key, value",
	"path_dictionary": "class, scope, root, ephemeral, note",
	"epics":           "slug, goal, status, status_note, close_sweep, created_at",
	"epic_criteria":   "epic, seq, criterion, met, evidence",
	"tasks":           "id, title, status, status_note, parked, dup_of, epic, created_at, updated_at",
	"task_links":      "from_task, to_task, type, note",
	"stories":         "epic, id, title, status, dod, consumes, produces, blocked",
	"worklog":         "epic, seq, story, date, state, commits, gate, review, corrects, note",
	"artifacts":       "id, class, scope, relpath, archived, note",
	"task_artifacts":  "task, artifact, role, note",
	"epic_artifacts":  "epic, artifact, role, note",
	"events":          "seq, at, entity, event, detail",
}

// Parse validates untrusted dump text against the exact serializer
// grammar. The header's schema_version selects DDL(k)/whitelist(k)
// BEFORE any data parsing — the meta row sits after the DDL block and
// cannot steer the parser (§8.1).
func Parse(text []byte) (*Dump, error) {
	version, body, err := parsePreamble(string(text))
	if err != nil {
		return nil, err
	}
	g, _ := grammarFor(version) // parsePreamble already refused an unknown version

	d := &Dump{Version: version}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line == "" {
			// The serializer ends every INSERT with a newline, so exactly
			// one empty element — the last — is expected. A blank line
			// anywhere else is not the serializer's grammar and is refused,
			// not silently skipped (§8.5 step 2 is "anything else is a hard
			// refusal"; a special-cased skip would be a soft spot).
			if i == len(lines)-1 {
				continue
			}
			return nil, fmt.Errorf("%w: blank data line %d", ErrRefused, i+1)
		}
		ins, err := parseInsert(line, g.columns)
		if err != nil {
			return nil, fmt.Errorf("data line %d: %w", i+1, err)
		}
		d.Inserts = append(d.Inserts, ins)
	}
	return d, nil
}

// parsePreamble validates the header line and the byte-equal DDL block,
// returning the resolved schema version and the data body that follows.
// The header's version selects the parser BEFORE any data is read (§8.1):
// the current grammar for version N, a registered historical grammar for
// an older version (§8.6 — the dump-side hydration source), a refusal for
// anything newer or unknown.
func parsePreamble(s string) (int, string, error) {
	header, rest, ok := strings.Cut(s, "\n")
	if !ok || !strings.HasPrefix(header, headerPrefix) {
		return 0, "", fmt.Errorf("%w: missing or malformed header line", ErrRefused)
	}
	fields := strings.Fields(strings.TrimPrefix(header, "-- selftracked dump "))
	version, err := headerVersion(fields)
	if err != nil {
		return 0, "", err
	}
	if version > current {
		return 0, "", fmt.Errorf("%w: dump schema_version=%d requires a newer binary (this one compiles v%d)",
			ErrRefused, version, current)
	}
	g, known := grammarFor(version)
	if !known {
		return 0, "", fmt.Errorf("%w: unknown schema_version=%d", ErrRefused, version)
	}
	// Step 1 (§8.5): the DDL block must byte-equal the compiled-in
	// canonical DDL for that version — not "look valid", byte-equal.
	if !strings.HasPrefix(rest, g.ddl) {
		return 0, "", fmt.Errorf("%w: DDL block does not byte-equal the canonical DDL for v%d", ErrRefused, version)
	}
	return version, rest[len(g.ddl):], nil
}

// HeaderVersion extracts the schema version from a dump's header line —
// the §8.6 gate's comparison (i) reads it without parsing the dump.
// Reports false for anything that is not a well-formed header.
func HeaderVersion(header string) (int, bool) {
	if !strings.HasPrefix(header, headerPrefix) {
		return 0, false
	}
	v, err := headerVersion(strings.Fields(strings.TrimPrefix(header, "-- selftracked dump ")))
	if err != nil {
		return 0, false
	}
	return v, true
}

func headerVersion(fields []string) (int, error) {
	for _, f := range fields {
		if raw, found := strings.CutPrefix(f, "schema_version="); found {
			v, err := strconv.Atoi(raw)
			if err != nil || v < 1 {
				return 0, fmt.Errorf("%w: unparseable schema_version %q", ErrRefused, raw)
			}
			return v, nil
		}
	}
	return 0, fmt.Errorf("%w: header carries no schema_version", ErrRefused)
}

// parseInsert accepts exactly one shape:
//
//	INSERT INTO <known-table> (<exact column list>) VALUES (<literals>);
//
// Literal tokens only — integers, quoted strings with ” doubling, NULL.
func parseInsert(line string, columns map[string]string) (Insert, error) {
	rest, ok := strings.CutPrefix(line, "INSERT INTO ")
	if !ok {
		return Insert{}, errNotWhitelisted(line)
	}
	table, rest, ok := strings.Cut(rest, " (")
	if !ok {
		return Insert{}, errNotWhitelisted(line)
	}
	wantCols, known := columns[table]
	if !known {
		return Insert{}, fmt.Errorf("%w: unknown table %q", ErrRefused, table)
	}
	cols, rest, ok := strings.Cut(rest, ") VALUES (")
	if !ok {
		return Insert{}, errNotWhitelisted(line)
	}
	if cols != wantCols {
		return Insert{}, fmt.Errorf("%w: column list %q is not the canonical %q", ErrRefused, cols, wantCols)
	}
	body, ok := strings.CutSuffix(rest, ");")
	if !ok {
		return Insert{}, errNotWhitelisted(line)
	}
	values, err := parseLiterals(body)
	if err != nil {
		return Insert{}, err
	}
	if want := strings.Count(wantCols, ",") + 1; len(values) != want {
		return Insert{}, fmt.Errorf("%w: %d values for %d columns", ErrRefused, len(values), want)
	}
	return Insert{Table: table, Values: values}, nil
}

func errNotWhitelisted(line string) error {
	const show = 60
	if len(line) > show {
		line = line[:show] + "…"
	}
	return fmt.Errorf("%w: statement outside the whitelist grammar: %q", ErrRefused, line)
}

// parseLiterals tokenizes a VALUES body character-wise: NULL, decimal
// integers, and single-quoted strings where ” is the only escape. No
// token shape beyond these exists — an expression is a parse error, not
// something evaluated.
func parseLiterals(body string) ([]Literal, error) {
	var out []Literal
	i := 0
	for {
		lit, next, err := parseLiteral(body, i)
		if err != nil {
			return nil, err
		}
		out = append(out, lit)
		if next == len(body) {
			return out, nil
		}
		const sep = ", "
		if !strings.HasPrefix(body[next:], sep) {
			return nil, fmt.Errorf("%w: unexpected token at %q", ErrRefused, clip(body[next:]))
		}
		i = next + len(sep)
	}
}

//nolint:ireturn // Literal is the closed value grammar by design (nil|int64|string)
func parseLiteral(body string, i int) (Literal, int, error) {
	if i >= len(body) {
		return nil, 0, fmt.Errorf("%w: truncated VALUES body", ErrRefused)
	}
	if strings.HasPrefix(body[i:], "NULL") {
		return nil, i + len("NULL"), nil
	}
	if body[i] == '\'' {
		return parseString(body, i)
	}
	return parseInteger(body, i)
}

//nolint:ireturn // Literal is the closed value grammar by design (nil|int64|string)
func parseString(body string, i int) (Literal, int, error) {
	var sb strings.Builder
	j := i + 1
	for j < len(body) {
		c := body[j]
		if c != '\'' {
			sb.WriteByte(c)
			j++
			continue
		}
		// A quote: doubled means a literal quote, single closes.
		if j+1 < len(body) && body[j+1] == '\'' {
			sb.WriteByte('\'')
			j += 2
			continue
		}
		return sb.String(), j + 1, nil
	}
	return nil, 0, fmt.Errorf("%w: unterminated string literal", ErrRefused)
}

//nolint:ireturn // Literal is the closed value grammar by design (nil|int64|string)
func parseInteger(body string, i int) (Literal, int, error) {
	j := i
	if j < len(body) && body[j] == '-' {
		j++
	}
	start := j
	for j < len(body) && body[j] >= '0' && body[j] <= '9' {
		j++
	}
	if j == start {
		return nil, 0, fmt.Errorf("%w: token at %q is not a whitelisted literal", ErrRefused, clip(body[i:]))
	}
	tok := body[i:j]
	// The serializer emits %d — one canonical spelling per value. A loader
	// that also accepted 007 or -0 would admit a byte-shape the serializer
	// can never produce, softening the byte-exact grammar (§8.5 step 2).
	if !canonicalInteger(tok) {
		return nil, 0, fmt.Errorf("%w: non-canonical integer literal %q", ErrRefused, tok)
	}
	n, err := strconv.ParseInt(tok, 10, 64)
	if err != nil {
		//nolint:errorlint // family error is the %w; strconv detail is context
		return nil, 0, fmt.Errorf("%w: integer literal %q: %v", ErrRefused, tok, err)
	}
	return n, j, nil
}

// canonicalInteger accepts exactly the serializer's %d output: an optional
// minus on a non-zero magnitude, no leading zeros, no -0, no +.
func canonicalInteger(tok string) bool {
	mag := tok
	if neg, ok := strings.CutPrefix(tok, "-"); ok {
		mag = neg
		if mag == "0" || mag == "" {
			return false // -0, or a lone "-"
		}
	}
	if len(mag) > 1 && mag[0] == '0' {
		return false // leading zero
	}
	return true
}

func clip(s string) string {
	const show = 30
	if len(s) > show {
		return s[:show] + "…"
	}
	return s
}
