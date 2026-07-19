//nolint:testpackage // white-box: the §6.1 order and the optimize call are internal steps a black box cannot see
package verb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// TestWritePipelineOrder proves INV-180 as an ORDER, not an
// implementation detail: gate → divergence → transaction → dump →
// STATE.md slot → sidecar → optimize. The S5a close review found the
// sidecar written before the STATE slot (a latent §6.1 inversion); this
// test is what keeps it fixed.
//
//nolint:paralleltest // serial by necessity: chdir + package-level trace hook
func TestWritePipelineOrder(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)

	var steps []string
	traceStep = func(s string) { steps = append(steps, s) }
	t.Cleanup(func() { traceStep = nil })

	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		if _, err := tx.ExecContext(context.Background(),
			`INSERT INTO tasks (title, created_at, updated_at) VALUES ('t', 'd', 'd')`); err != nil {
			return nil, fmt.Errorf("seed write: %w", err)
		}
		return []Event{{Entity: "#1", Event: "create", Detail: "t"}}, nil
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := "gate divergence transaction dump state sidecar optimize"
	if got := strings.Join(steps, " "); got != want {
		t.Fatalf("pipeline order:\n got %s\nwant %s", got, want)
	}
}

func seedInstance(t *testing.T, dir string) {
	t.Helper()
	inst := filepath.Join(dir, instanceDir)
	if err := os.MkdirAll(inst, 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := schema.Open(filepath.Join(inst, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := schema.Create(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	text, err := dump.Serialize(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err := dump.WriteFiles(inst, text); err != nil {
		t.Fatal(err)
	}
}
