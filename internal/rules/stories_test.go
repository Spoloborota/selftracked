package rules_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/Spoloborota/selftracked/internal/rules"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// storyDB is a fresh tracker schema holding one ACTIVE epic and whatever
// stories a case seeds. Rows go in with raw SQL on purpose: the subject is
// the lookup, and driving the verbs would drag the whole write pipeline
// (and its transition matrix) into a query test.
func storyDB(t *testing.T, stories map[string]string) *sql.DB {
	t.Helper()
	db, err := schema.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := schema.Create(ctx, db); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', 'd')`)
	if err != nil {
		t.Fatal(err)
	}
	for id, status := range stories {
		_, err := db.ExecContext(ctx,
			`INSERT INTO stories (epic, id, title, status) VALUES ('e', ?, 't', ?)`, id, status)
		if err != nil {
			t.Fatalf("seed story %s=%s: %v", id, status, err)
		}
	}
	return db
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestStoryToOffer pins the whole selection contract: which statuses
// qualify, which story wins when several do, and what the dead zone
// returns. `want` is "" for the dead zone, otherwise "ID=STATUS".
func TestStoryToOffer(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		stories map[string]string
		want    string
	}{
		"no stories at all is the dead zone": {
			stories: map[string]string{},
			want:    "",
		},
		"every story terminal is the dead zone": {
			stories: map[string]string{"S1": rules.StoryDone, "S2": rules.StoryDissolved},
			want:    "",
		},
		"terminal stories never win": {
			stories: map[string]string{
				"S1": rules.StoryDone, "S2": rules.StoryDissolved, "S3": rules.StoryBlocked,
			},
			want: "S3=BLOCKED",
		},
		// The finding-1 bug: a lowest-id offer would name S1, send the agent
		// through a `story ready` that mutates state for nothing, and land it
		// on the WIP refusal S2 was already holding.
		"IN-PROGRESS beats a lower-id PLANNED": {
			stories: map[string]string{"S1": rules.StoryPlanned, "S2": rules.StoryInProgress},
			want:    "S2=IN-PROGRESS",
		},
		"READY beats a lower-id BLOCKED and PLANNED": {
			stories: map[string]string{
				"S1": rules.StoryPlanned, "S2": rules.StoryBlocked, "S3": rules.StoryReady,
			},
			want: "S3=READY",
		},
		"BLOCKED beats a lower-id PLANNED": {
			stories: map[string]string{"S1": rules.StoryPlanned, "S2": rules.StoryBlocked},
			want:    "S2=BLOCKED",
		},
		"PLANNED wins when it is all there is": {
			stories: map[string]string{"S1": rules.StoryDone, "S7": rules.StoryPlanned},
			want:    "S7=PLANNED",
		},
		// The id tie-break, which only ever runs within one status. A lexical
		// ORDER BY id would rank S10 first and offer the newest story.
		"equal status ties break numerically, not lexically": {
			stories: map[string]string{
				"S2": rules.StoryPlanned, "S10": rules.StoryPlanned, "S1": rules.StoryDone,
			},
			want: "S2=PLANNED",
		},
		"priority outranks the numeric tie-break": {
			stories: map[string]string{"S2": rules.StoryPlanned, "S10": rules.StoryReady},
			want:    "S10=READY",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := storyDB(t, tc.stories)
			got, ok, err := rules.StoryToOffer(context.Background(), db, "e")
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				if tc.want != "" {
					t.Fatalf("StoryToOffer found nothing, want %s", tc.want)
				}
				return
			}
			if tc.want == "" {
				t.Fatalf("StoryToOffer = %s=%s, want the dead zone", got.ID, got.Status)
			}
			if g := got.ID + "=" + got.Status; g != tc.want {
				t.Errorf("StoryToOffer = %s, want %s", g, tc.want)
			}
		})
	}
}

// TestStoryToOfferIdsTheSchemaAdmits: `stories.id`'s CHECK is only
// `GLOB 'S[0-9]*'`, so ids sharing a numeric key are legal. The raw-id
// tie-break must still pick one of them deterministically rather than
// leave the choice to the query planner.
func TestStoryToOfferIdsTheSchemaAdmits(t *testing.T) {
	t.Parallel()
	db := storyDB(t, map[string]string{
		"S1a": rules.StoryPlanned, "S1b": rules.StoryPlanned, "S01": rules.StoryPlanned,
	})
	first, ok, err := rules.StoryToOffer(context.Background(), db, "e")
	if err != nil || !ok {
		t.Fatalf("StoryToOffer: %v (found=%v)", err, ok)
	}
	for range 5 {
		again, _, err := rules.StoryToOffer(context.Background(), db, "e")
		if err != nil {
			t.Fatal(err)
		}
		if again.ID != first.ID {
			t.Fatalf("the same state offered %s then %s — the order is not total", first.ID, again.ID)
		}
	}
	// All three share numeric key 1, so the raw id decides: "S01" < "S1a".
	if first.ID != "S01" {
		t.Errorf("tie-break = %s, want S01 (lowest raw id among equal numeric keys)", first.ID)
	}
}

// TestStoryToOfferUnknownEpic: an epic nobody created reads as the dead
// zone rather than an error — the callers distinguish "does not exist"
// from "has no workable story" themselves, and an error here would force
// each of them to special-case it.
func TestStoryToOfferUnknownEpic(t *testing.T) {
	t.Parallel()
	db := storyDB(t, map[string]string{"S1": rules.StoryReady})
	got, ok, err := rules.StoryToOffer(context.Background(), db, "nope")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("StoryToOffer on an unknown epic = %s=%s, want nothing", got.ID, got.Status)
	}
}

// TestStoryToOfferFromTx proves the Querier interface's whole reason for
// existing: `worklog add` composes its refusal INSIDE the write
// transaction, so the same lookup must serve a *sql.Tx unchanged.
func TestStoryToOfferFromTx(t *testing.T) {
	t.Parallel()
	db := storyDB(t, map[string]string{"S1": rules.StoryDone, "S2": rules.StoryBlocked})
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	got, ok, err := rules.StoryToOffer(ctx, tx, "e")
	if err != nil || !ok {
		t.Fatalf("from a tx: %v (found=%v)", err, ok)
	}
	if got.ID != "S2" || got.Status != rules.StoryBlocked {
		t.Errorf("from a tx = %s=%s, want S2=BLOCKED", got.ID, got.Status)
	}
}

// TestNonTerminalStoryStatuses pins both halves of the contract: the
// membership (four statuses, DONE and DISSOLVED excluded) and the order,
// which StoryToOffer's correctness argument rests on — IN-PROGRESS must
// sort first, or the clause can promise a `story start` the WIP index
// refuses.
func TestNonTerminalStoryStatuses(t *testing.T) {
	t.Parallel()
	want := []string{"IN-PROGRESS", "READY", "BLOCKED", "PLANNED"}
	if got := rules.NonTerminalStoryStatuses(); !equal(got, want) {
		t.Fatalf("NonTerminalStoryStatuses = %v, want %v", got, want)
	}
	for _, terminal := range []string{rules.StoryDone, rules.StoryDissolved} {
		for _, s := range rules.NonTerminalStoryStatuses() {
			if s == terminal {
				t.Errorf("%s is terminal and must not be in the set", terminal)
			}
		}
	}
}

// TestNonTerminalStoryStatusesIsNotShared: the slice is the definition
// every reader consults, so a caller that sorts or truncates the returned
// value must not redefine it for the next caller.
func TestNonTerminalStoryStatusesIsNotShared(t *testing.T) {
	t.Parallel()
	want := rules.NonTerminalStoryStatuses()
	first := rules.NonTerminalStoryStatuses()
	for i := range first {
		first[i] = "MANGLED"
	}
	if second := rules.NonTerminalStoryStatuses(); !equal(second, want) {
		t.Errorf("after mutating the first result, got %v, want %v", second, want)
	}
}
