package cli_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
	_ "modernc.org/sqlite" // the driver whose codes the mapper reads
)

// TestExitMapper drives the §6.1 mapper with a REAL driver error (S1c
// proved the extended codes; here the mapper's reading of them is what is
// under test), a verb-classified refusal, and an unclassified error.
func TestExitMapper(t *testing.T) {
	t.Parallel()

	t.Run("constraint-family-is-refusal-exit-1", func(t *testing.T) {
		t.Parallel()
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if _, err := db.ExecContext(t.Context(),
			`CREATE TABLE x (v TEXT NOT NULL CHECK (v <> '')) STRICT`); err != nil {
			t.Fatal(err)
		}
		_, violation := db.ExecContext(t.Context(), `INSERT INTO x (v) VALUES ('')`)
		if violation == nil {
			t.Fatal("the CHECK did not fire; no error to classify")
		}
		status, code := cli.ClassifyForTest(violation)
		if status != 1 || code != "constraint" {
			t.Fatalf("classify(CHECK violation) = %d %q; want 1 constraint", status, code)
		}
	})
	t.Run("coded-refusal-keeps-its-status", func(t *testing.T) {
		t.Parallel()
		status, code := cli.ClassifyForTest(&cli.CodedError{Code: "not-found", Message: "no such file", Status: 1})
		if status != 1 || code != "not-found" {
			t.Fatalf("classify(Coded refusal) = %d %q; want 1 not-found", status, code)
		}
	})
	t.Run("unclassified-error-is-infrastructure-exit-2", func(t *testing.T) {
		t.Parallel()
		unmapped := errors.New("something nobody mapped") //nolint:err113 // the point is an unclassified error
		status, code := cli.ClassifyForTest(unmapped)
		if status != 2 || code != "error" {
			t.Fatalf("classify(unknown) = %d %q; want 2 error — it must not pass as a clean refusal", status, code)
		}
	})
}
