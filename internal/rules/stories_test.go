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

// epicsDB is a fresh schema holding the named epics with the given
// statuses, and whatever stories each case seeds under them.
func epicsDB(t *testing.T, epics map[string]string, stories map[string]map[string]string) *sql.DB {
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
	for slug, status := range epics {
		// status_note is CHECK-required for PAUSED and DISSOLVED, and
		// close_sweep is CHECK-tied to CLOSED exactly; both are supplied to
		// the shape the schema demands rather than branched on at each seed.
		sweep := ""
		if status == "CLOSED" {
			sweep = "2026-01-01"
		}
		_, err := db.ExecContext(ctx,
			`INSERT INTO epics (slug, goal, status, status_note, close_sweep, created_at)
			 VALUES (?, 'g', ?, 'n', ?, 'd')`, slug, status, sweep)
		if err != nil {
			t.Fatalf("seed epic %s=%s: %v", slug, status, err)
		}
	}
	for slug, ss := range stories {
		for id, status := range ss {
			blocked := ""
			if status == rules.StoryBlocked {
				blocked = "why"
			}
			_, err := db.ExecContext(ctx,
				`INSERT INTO stories (epic, id, title, status, blocked) VALUES (?, ?, 't', ?, ?)`,
				slug, id, status, blocked)
			if err != nil {
				t.Fatalf("seed story %s/%s=%s: %v", slug, id, status, err)
			}
		}
	}
	return db
}

// TestEpicsWithoutWorkableStory pins the predicate R10's trigger (a) and
// `prime`'s notice both read: which epic statuses are in scope, and which
// story statuses count as a home. The set is WIDER than the
// READY/IN-PROGRESS pair R10's idle clause uses, and the PLANNED/BLOCKED
// rows below are exactly that difference.
func TestEpicsWithoutWorkableStory(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		epics   map[string]string
		stories map[string]map[string]string
		want    []string
	}{
		"an ACTIVE epic with no stories at all": {
			epics: map[string]string{"a": "ACTIVE"},
			want:  []string{"a"},
		},
		"an ACTIVE epic whose stories are all terminal": {
			epics:   map[string]string{"a": "ACTIVE"},
			stories: map[string]map[string]string{"a": {"S1": rules.StoryDone, "S2": rules.StoryDissolved}},
			want:    []string{"a"},
		},
		"a PLANNED story is a home": {
			epics:   map[string]string{"a": "ACTIVE"},
			stories: map[string]map[string]string{"a": {"S1": rules.StoryDone, "S2": rules.StoryPlanned}},
			want:    nil,
		},
		"a BLOCKED story is a home": {
			epics:   map[string]string{"a": "ACTIVE"},
			stories: map[string]map[string]string{"a": {"S1": rules.StoryBlocked}},
			want:    nil,
		},
		"a READY story is a home": {
			epics:   map[string]string{"a": "ACTIVE"},
			stories: map[string]map[string]string{"a": {"S1": rules.StoryReady}},
			want:    nil,
		},
		"an IN-PROGRESS story is a home": {
			epics:   map[string]string{"a": "ACTIVE"},
			stories: map[string]map[string]string{"a": {"S1": rules.StoryInProgress}},
			want:    nil,
		},
		"non-ACTIVE epics are out of scope whatever their stories": {
			epics: map[string]string{"b": "BACKLOG", "p": "PAUSED", "c": "CLOSED", "d": "DISSOLVED"},
			want:  nil,
		},
		"several epics come back in slug order": {
			epics:   map[string]string{"zeta": "ACTIVE", "alpha": "ACTIVE", "mid": "ACTIVE"},
			stories: map[string]map[string]string{"mid": {"S1": rules.StoryReady}},
			want:    []string{"alpha", "zeta"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := epicsDB(t, tc.epics, tc.stories)
			got, err := rules.EpicsWithoutWorkableStory(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			if !equal(got, tc.want) {
				t.Fatalf("EpicsWithoutWorkableStory = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEpicsWithoutWorkableStoryFromTx: the predicate is asked from inside a
// write transaction too (a verb composing a message before it commits), so
// *sql.Tx must satisfy RowsQuerier.
func TestEpicsWithoutWorkableStoryFromTx(t *testing.T) {
	t.Parallel()
	db := epicsDB(t, map[string]string{"a": "ACTIVE"}, nil)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	got, err := rules.EpicsWithoutWorkableStory(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	if !equal(got, []string{"a"}) {
		t.Fatalf("from a tx = %v, want [a]", got)
	}
}
