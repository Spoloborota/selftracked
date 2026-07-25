package verb

import (
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/Spoloborota/selftracked/internal/rules"
)

// TestStoryRouteClause pins the composed text for every status
// StoryToOffer can return. The whole string is compared, not a substring:
// the clause's value is that an agent can read the state and the command
// in one line, and an assertion on fragments would pass a reordering that
// destroys exactly that.
func TestStoryRouteClause(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		story rules.Story
		want  string
	}{
		"PLANNED names ready then start": {
			story: rules.Story{ID: "S1", Status: rules.StoryPlanned},
			want: `; story "S1" of epic "wl" is PLANNED` +
				` — story ready "wl" "S1", then story start "wl" "S1"`,
		},
		"READY names start alone": {
			story: rules.Story{ID: "S2", Status: rules.StoryReady},
			want:  `; story "S2" of epic "wl" is READY — story start "wl" "S2"`,
		},
		"BLOCKED names unblock, with its required flag, then start": {
			story: rules.Story{ID: "S3", Status: rules.StoryBlocked},
			want: `; story "S3" of epic "wl" is BLOCKED` +
				` — story unblock "wl" "S3" --resolution TEXT, then story start "wl" "S3"`,
		},
		"IN-PROGRESS names done, not another start": {
			story: rules.Story{ID: "S4", Status: rules.StoryInProgress},
			want: `; story "S4" of epic "wl" is IN-PROGRESS and already holds the WIP slot` +
				` — story done "wl" "S4" --commits RANGE --gate G writes the episode`,
		},
		// The default branch, unreachable through StoryToOffer. Its comment
		// claims it degrades to the head; this is that claim under test.
		"an unknown status degrades to the head with no route": {
			story: rules.Story{ID: "S5", Status: "TELEPORTED"},
			want:  `; story "S5" of epic "wl" is TELEPORTED`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := storyRouteClause("wl", tc.story); got != tc.want {
				t.Errorf("storyRouteClause:\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

// TestStoryRouteClauseNeverPromisesStartToItself: an IN-PROGRESS story
// already holds the WIP slot, so naming `story start` for it would be a
// command that cannot succeed. The other three branches may promise it
// only because StoryToOffer sorts IN-PROGRESS first (see its doc comment),
// which is what makes the slot free whenever they are reached.
func TestStoryRouteClauseNeverPromisesStartToItself(t *testing.T) {
	t.Parallel()
	got := storyRouteClause("wl", rules.Story{ID: "S1", Status: rules.StoryInProgress})
	if strings.Contains(got, "story start") {
		t.Errorf("the IN-PROGRESS clause names story start, which the WIP index refuses: %s", got)
	}
	for _, st := range []string{rules.StoryPlanned, rules.StoryReady, rules.StoryBlocked} {
		clause := storyRouteClause("wl", rules.Story{ID: "S1", Status: st})
		if !strings.Contains(clause, `story start "wl" "S1"`) {
			t.Errorf("the %s clause must reach story start: %s", st, clause)
		}
	}
}

// TestDeadZoneClause pins the one branch that involves the owner: both
// sanctioned routes named, and the story route marked a scope change that
// is the owner's call (§6.2).
func TestDeadZoneClause(t *testing.T) {
	t.Parallel()
	want := `; epic "wl" has no story in a non-terminal status` +
		` — a new story (story add "wl" --title T, then story ready and story start)` +
		` is a scope change and the owner's call;` +
		` work that does not advance this epic's goal is a standalone task (create --title T)`
	if got := deadZoneClause("wl"); got != want {
		t.Errorf("deadZoneClause:\n got %s\nwant %s", got, want)
	}
}

// TestRouteClausesEscapeHostileIdentifiers: neither a slug nor a story id
// is shape-constrained enough to trust. `epics.slug` carries no CHECK,
// `stories.id`'s CHECK is `GLOB 'S[0-9]*'` (unconstrained after the first
// digit), and `import` screens text only for control runes -- so both can
// carry bidi and format characters into a message another agent reads.
// Every interpolation is %q, which renders them as escapes.
//
// The property asserted is the one that matters to the reader of the
// message: no unprintable rune survives into it. Testing for the literal
// escape text instead would need the raw runes in this file, and a source
// file must not carry the characters it exists to prove are removed --
// hence rune(0x202E) rather than a string literal.
func TestRouteClausesEscapeHostileIdentifiers(t *testing.T) {
	t.Parallel()
	// U+202E RIGHT-TO-LEFT OVERRIDE reverses the display of everything
	// after it; U+200B is an invisible separator. Both are schema-legal in
	// a slug, and in a story id after the first digit.
	rtlOverride, zeroWidth := string(rune(0x202E)), string(rune(0x200B))
	hostileSlug := "ev" + rtlOverride + "il"
	hostileID := "S1" + zeroWidth + "-x"
	clauses := map[string]string{
		"story clause": storyRouteClause(hostileSlug,
			rules.Story{ID: hostileID, Status: rules.StoryReady}),
		"dead zone clause": deadZoneClause(hostileSlug),
		"story id in a benign epic": storyRouteClause("wl",
			rules.Story{ID: hostileID, Status: rules.StoryReady}),
	}
	for name, got := range clauses {
		for _, r := range got {
			// The em-dash the clause joins on is printable; a format or
			// control rune is not, and is what %q had to remove.
			if !unicode.IsPrint(r) {
				t.Errorf("%s carries the unprintable rune %U: %+q", name, r, got)
			}
		}
		// Escaped, not dropped: the identifier is still legible.
		if !strings.Contains(got, strconv.Quote(hostileSlug)) &&
			!strings.Contains(got, strconv.Quote(hostileID)) {
			t.Errorf("%s contains neither identifier in its quoted form: %+q", name, got)
		}
	}
}
