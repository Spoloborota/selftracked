package dump

import (
	"strings"
	"testing"
)

// TestEveryTableDeclaresItsFullPKOrder is the white-box half of the §8.1
// row-order contract. The black-box fixtures cannot kill an ORDER BY
// removal: SQLite happens to scan rowid tables in rowid order and WITHOUT
// ROWID tables in PK order, so today's engine returns sorted rows either
// way — but that is a coincidence of storage, not a contract, and the
// serializer must not lean on it. This test makes the explicit ordering
// declaration itself load-bearing (found as a surviving mutant during the
// S3 mutation round).
func TestEveryTableDeclaresItsFullPKOrder(t *testing.T) {
	t.Parallel()

	if len(tables) == 0 {
		t.Fatal("no tables declared")
	}
	for _, tb := range tables {
		if strings.TrimSpace(tb.orderBy) == "" {
			t.Errorf("%s: no ORDER BY declared; row order would ride on scan-order luck", tb.name)
		}
		if strings.TrimSpace(tb.columns) == "" {
			t.Errorf("%s: no explicit column list", tb.name)
		}
	}
	if tables[len(tables)-1].name != "events" {
		t.Errorf("events is not the last table; the append-only tail must be the diff's tail")
	}
}
