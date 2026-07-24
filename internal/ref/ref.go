// Package ref implements the §4 reference grammar: the one syntax every
// verb uses to name a task, an epic, a story or an artifact. It is a leaf
// package — verbs consume it, it consumes nothing of theirs — because a
// reference's meaning must not depend on which verb is reading it.
package ref

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Kind says which entity a reference names.
type Kind int

// The four reference kinds of §4.
const (
	Task Kind = iota + 1
	Epic
	Story
	Artifact
)

// Ref is one parsed reference. Exactly the fields implied by its Kind are
// set; the zero value is not a valid reference.
type Ref struct {
	Kind  Kind
	Task  int64  // Task
	Epic  string // Epic, Story
	Story string // Story
	Class string // Artifact
	Scope string // Artifact; "" is the default scope
	Rel   string // Artifact
}

// ErrInvalid wraps every parse refusal so callers can match the family
// without matching message text.
var ErrInvalid = errors.New("invalid reference")

// Parse reads one §4 reference:
//
//	#NN or bare NN            task
//	epic:SLUG                 epic
//	epic:SLUG/SID             story
//	CLASS:RELPATH             artifact, default scope
//	CLASS@SCOPE:RELPATH       artifact, named scope
//
// The grammar cannot produce the bare keywords `archive`/`unarchive` —
// §6.2 states that invariant so `link`'s keyword dispatch stays
// unambiguous — and this parser preserves it: a bare word without a colon
// is not a reference of any kind.
func Parse(s string) (Ref, error) {
	if s == "" {
		return Ref{}, fmt.Errorf("%w: empty", ErrInvalid)
	}
	if r, matched, err := parseTask(s); matched {
		return r, err
	}
	head, rest, found := strings.Cut(s, ":")
	if !found || head == "" || rest == "" {
		return Ref{}, fmt.Errorf("%w: %q", ErrInvalid, s)
	}
	if head == "epic" {
		return parseEpicOrStory(s, rest)
	}
	return parseArtifact(s, head, rest)
}

// parseTask reads #NN, or the bare integer the CLI accepts because an
// unquoted # opens a shell comment (§4). matched reports whether the token
// was task-shaped at all — a non-task token falls through to the colon
// grammar, but a broken #-token is refused here.
func parseTask(s string) (Ref, bool, error) {
	digits := strings.TrimPrefix(s, "#")
	if id, err := strconv.ParseInt(digits, 10, 64); err == nil {
		if id <= 0 {
			return Ref{}, true, fmt.Errorf("%w: task id must be positive: %q", ErrInvalid, s)
		}
		return Ref{Kind: Task, Task: id}, true, nil
	}
	if strings.HasPrefix(s, "#") {
		return Ref{}, true, fmt.Errorf("%w: %q is not #NN", ErrInvalid, s)
	}
	return Ref{}, false, nil
}

func parseArtifact(s, head, rest string) (Ref, error) {
	class, scope, scoped := strings.Cut(head, "@")
	if !scoped {
		return Ref{Kind: Artifact, Class: head, Rel: rest}, nil
	}
	if class == "" || scope == "" {
		return Ref{}, fmt.Errorf("%w: %q has an empty class or scope", ErrInvalid, s)
	}
	if class == "epic" {
		return Ref{}, fmt.Errorf("%w: %q — epic takes no scope", ErrInvalid, s)
	}
	return Ref{Kind: Artifact, Class: class, Scope: scope, Rel: rest}, nil
}

func parseEpicOrStory(s, rest string) (Ref, error) {
	slug, sid, isStory := strings.Cut(rest, "/")
	if slug == "" {
		return Ref{}, fmt.Errorf("%w: %q has an empty slug", ErrInvalid, s)
	}
	if !isStory {
		return Ref{Kind: Epic, Epic: slug}, nil
	}
	if !validStoryID(sid) {
		return Ref{}, fmt.Errorf("%w: %q — story id must be S<number>", ErrInvalid, s)
	}
	return Ref{Kind: Story, Epic: slug, Story: sid}, nil
}

// validStoryID accepts S<number> — the §4 story-id form. V-<n> rows are
// worklog records, not addressable stories, so they are not references.
func validStoryID(id string) bool {
	if len(id) < 2 || id[0] != 'S' {
		return false
	}
	for _, r := range id[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// TaskRef renders a task id the one way JSON output ever shows it: "#14"
// (§4 — output always uses the prefixed form; only input accepts bare NN).
func TaskRef(id int64) string {
	return "#" + strconv.FormatInt(id, 10)
}
