// This file is the §8.6 migration machinery: the per-version registry
// (DDL, whitelist, transform), the rows-in-flight corpus, and the two
// hydration sources — a live database and a parsed old dump. v0 compiles
// schema version 1 and an empty registry: the machinery is the policy,
// the first Step is written when version 2 exists. Tests exercise the
// chain by installing synthetic Steps; nothing in a shipped binary's
// normal flow can reach a transform that was never compiled in.

package load

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/Spoloborota/selftracked/internal/schema"
)

// TableSpec describes one table of a historical schema version: the
// parser's exact column list (comma-space separated, the serializer
// form), the full-PK ORDER BY hydration reads in, and an optional row
// filter (the serializer's own exclusions — active_verb — must not
// resurrect through a migration).
type TableSpec struct {
	Name    string
	Columns string
	OrderBy string
	Where   string
}

// Step is one historical schema version k: the canonical DDL(k) old dumps
// are byte-checked against, the version's table specs (whitelist and
// hydration in one artifact), and the pure-Go row transform
// T_k: rows(k) → rows(k+1).
type Step struct {
	DDL       string
	Tables    []TableSpec
	Transform func(c *Corpus) error
}

// Steps is the compiled-in registry of historical versions, keyed by k.
// Empty at v0 — schema version 1 is the first there is. Every entry is
// written by hand when its successor version is born (§8.6); tests may
// install synthetic entries to drive the chain.
var Steps = map[int]Step{}

// current is the destination version everything in this package migrates
// and refuses against. schema.Version always — a variable only as the
// white-box test seam (the synthetic bump), the same reason the migrate
// package carries one; production never assigns it.
var current = schema.Version

// grammar is the dump dialect of one schema version: what parsePreamble
// byte-checks and what parseInsert whitelists.
type grammar struct {
	ddl     string
	columns map[string]string
}

// grammarFor resolves the parsing grammar for a header version: the
// compiled-in grammar for the current version FIRST — structurally, so a
// registry entry keyed by the current version (a future off-by-one in a
// hand-written Step) can never shadow the byte-equal DDL gate the §8.5
// boundary rests on — then the registered historical Steps, then unknown.
func grammarFor(version int) (grammar, bool) {
	if version == current {
		return grammar{ddl: schema.DDL(), columns: expectedColumns}, true
	}
	if st, ok := Steps[version]; ok {
		return grammar{ddl: st.DDL, columns: st.columnsMap()}, true
	}
	return grammar{}, false
}

func (s Step) columnsMap() map[string]string {
	m := make(map[string]string, len(s.Tables))
	for _, t := range s.Tables {
		m[t.Name] = t.Columns
	}
	return m
}

// Corpus is the rows-in-flight representation between transforms: every
// table's rows, in the serializer's row order, as parse/hydrate produced
// them. Transforms edit it in place — rename tables, rewrite rows — and
// MUST keep serializer PK order and re-emit any renumbered worklog epic's
// seq contiguously: the rebuild replays through the load path with the
// INSERT-firing gates live, and a violation aborts the build (§8.6).
type Corpus struct {
	Tables map[string][][]Literal
}

// querier is the one query capability hydration needs — satisfied by both
// *sql.DB and *sql.Conn, so the gate can hydrate inside its own locked
// connection's snapshot.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// HydrateCorpus reads every table of a version's spec from a live
// database, in serializer order — §8.6's live-DB source.
func HydrateCorpus(ctx context.Context, db querier, tables []TableSpec) (*Corpus, error) {
	c := &Corpus{Tables: make(map[string][][]Literal, len(tables))}
	for _, t := range tables {
		rows, err := hydrateTable(ctx, db, t)
		if err != nil {
			return nil, err
		}
		c.Tables[t.Name] = rows
	}
	return c, nil
}

func hydrateTable(ctx context.Context, db querier, t TableSpec) ([][]Literal, error) {
	q := "SELECT " + t.Columns + " FROM " + t.Name // specs are compiled in, never user input
	if t.Where != "" {
		q += " WHERE " + t.Where
	}
	q += " ORDER BY " + t.OrderBy
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("hydrate %s: %w", t.Name, err)
	}
	defer func() { _ = rows.Close() }()

	width := strings.Count(t.Columns, ",") + 1
	var out [][]Literal
	for rows.Next() {
		vals := make([]Literal, width)
		ptrs := make([]any, width)
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("hydrate %s: %w", t.Name, err)
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("hydrate %s: %w", t.Name, err)
	}
	return out, nil
}

// CorpusFromDump regroups a parsed dump's inserts per table — §8.6's
// old-dump source. Same engine, two sources.
func CorpusFromDump(d *Dump) *Corpus {
	c := &Corpus{Tables: map[string][][]Literal{}}
	for _, ins := range d.Inserts {
		c.Tables[ins.Table] = append(c.Tables[ins.Table], ins.Values)
	}
	return c
}

// MigrateCorpus runs the transform chain T_from … T_{to-1}, pins the meta
// schema_version row to the destination, and flattens the corpus into a
// Build-ready dump in the serializer's table order (parents before
// children, events last — the FK-safe order every dump already replays).
func MigrateCorpus(c *Corpus, from, to int, order []string) (*Dump, error) {
	for k := from; k < to; k++ {
		st, ok := Steps[k]
		if !ok || st.Transform == nil {
			return nil, fmt.Errorf("%w: no transform from schema v%d", ErrRefused, k)
		}
		if err := st.Transform(c); err != nil {
			return nil, fmt.Errorf("%w: transform v%d→v%d: %w", ErrRefused, k, k+1, err)
		}
	}
	pinSchemaVersion(c, to)

	d := &Dump{Version: to}
	emitted := 0
	for _, name := range order {
		for _, vals := range c.Tables[name] {
			d.Inserts = append(d.Inserts, Insert{Table: name, Values: vals})
		}
		if _, ok := c.Tables[name]; ok {
			emitted++
		}
	}
	if emitted != len(c.Tables) {
		return nil, fmt.Errorf("%w: transform left %d table(s) outside the serializer order",
			ErrRefused, len(c.Tables)-emitted)
	}
	return d, nil
}

// metaSchemaVersionKey is the meta row the engine pins after the chain.
const metaSchemaVersionKey = "schema_version"

// pinSchemaVersion forces the meta schema_version row to the destination
// version — uniformly here rather than in every transform, because a
// rebuilt database whose meta row lags would spuriously re-migrate on the
// next verb (§8.5 stamps identity from that row).
func pinSchemaVersion(c *Corpus, to int) {
	want := strconv.Itoa(to)
	for i, row := range c.Tables["meta"] {
		if len(row) == 2 && row[0] == metaSchemaVersionKey {
			c.Tables["meta"][i][1] = want
			return
		}
	}
	c.Tables["meta"] = append(c.Tables["meta"], []Literal{metaSchemaVersionKey, want})
}
