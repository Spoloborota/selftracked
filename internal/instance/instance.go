// Package instance answers the two identity questions §6.1 and §11.1 pose
// about the tracker in the working directory: which one it is (a salted,
// path-derived digest — amendment `a-tracker-carries-a-name`) and, when
// there is none here, where the nearest one lives (an upward walk that
// improves a refusal and never chooses a target — amendment
// `resolution-names-the-root-it-found`). Both surfaces resolve the working
// directory PHYSICALLY, so they can never disagree about which directory
// they describe.
package instance

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// saltFile is §9's gitignored per-tracker identity salt (§11.1).
	saltFile = "instance.salt"
	// saltMode matches the sidecar's owner-only posture: the salt is a
	// per-machine scratch value that must never be readable more widely
	// than the tracker's own database.
	saltMode = 0o600
	// saltBytes of cryptographic randomness; stored hex-encoded, one line.
	saltBytes = 32

	instanceDirName = ".selftracked"
)

// Digest returns the tracker identity §11.1 specifies: the leading 48 bits
// of SHA-256(salt || path) as 12 hex characters. dir is the instance
// directory as the verb resolves it (".selftracked"). The path half is the
// physically-resolved working directory — the directory containing dir —
// and the salt half is the tracker's instance.salt, created on first use
// (§6.1's one read-verb exception; only prime calls this).
//
// ok is false when the path cannot be resolved or the salt cannot be read
// or created. Per §11.1 the caller then OMITS the field: the digest is
// never computed from an unresolved path and never downgraded to an
// unsalted form, because a silent downgrade would appear exactly where the
// environment is unusual, be indistinguishable from the salted value, and
// get published.
func Digest(dir string) (string, bool) {
	path, ok := physicalWD()
	if !ok {
		return "", false
	}
	salt, ok := ensureSalt(dir)
	if !ok {
		return "", false
	}
	sum := sha256.Sum256(append(salt, path...))
	return hex.EncodeToString(sum[:])[:12], true
}

// physicalWD is the digest's path half and the walk's base: the working
// directory with symbolic links evaluated and no trailing separator.
// os.Getwd may return any one of several logical routes to a directory, so
// without EvalSymlinks one tracker would report two identities depending
// on how the session reached it (§11.1).
func physicalWD() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	phys, err := filepath.EvalSymlinks(wd)
	if err != nil {
		return "", false
	}
	return phys, true
}

// ensureSalt reads the tracker's salt, creating it on first use. The three
// mechanics task #76 required to be decided rather than invented:
//
//   - Creation is EXCLUSIVE and symlink-safe: the salt is written to a
//     temp file and hard-linked into place. link(2) never follows a
//     pre-existing name at the target, so a symlink planted at
//     instance.salt cannot redirect the write; it surfaces as EEXIST.
//   - Mode is 0600 via os.CreateTemp, the same owner-only posture as the
//     dump sidecar.
//   - Two processes racing to create it: both write temps, the first link
//     wins, the loser reads the winner's fully-written file — the link is
//     atomic and happens only after the content is closed, so no reader
//     ever sees a partial salt and the digest never changes between runs.
//
// An existing-but-empty file reads as absent-and-uncreatable (ok=false):
// an empty salt would silently restore the preimage oracle the salt
// exists to close.
func ensureSalt(dir string) ([]byte, bool) {
	path := filepath.Join(dir, saltFile)
	if salt, ok, decided := readSalt(path); decided {
		return salt, ok
	}

	raw := make([]byte, saltBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, false
	}
	content := []byte(hex.EncodeToString(raw) + "\n")
	tmp, err := os.CreateTemp(dir, saltFile+".tmp-*")
	if err != nil {
		return nil, false
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(content)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmpName)
		return nil, false
	}
	linkErr := os.Link(tmpName, path)
	_ = os.Remove(tmpName)
	if linkErr == nil {
		return bytes.TrimRight(content, "\n"), true
	}
	if errors.Is(linkErr, fs.ErrExist) {
		// Lost the creation race: the winner's salt is this tracker's.
		salt, ok, _ := readSalt(path)
		return salt, ok
	}
	return nil, false
}

// readSalt reads an existing salt. The third value (decided) is false only
// when the file does not exist (the caller may create it); any other
// failure — including an empty file — is a decided ok=false, which the
// digest reports as an omitted field rather than working around.
func readSalt(path string) ([]byte, bool, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // fixed .selftracked path
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, false
		}
		return nil, false, true
	}
	s := bytes.TrimRight(b, "\n")
	if len(s) == 0 {
		return nil, false, true
	}
	return s, true, true
}

// AncestorTracker names the nearest ancestor directory holding a
// `.selftracked/` directory, as a path RELATIVE to the working directory
// ("..", "../.."), never absolute per §14. It returns "" when no ancestor
// holds one, when the walk meets a candidate it cannot read (§6.1: an
// unreadable candidate is treated as absent and the walk stops — a refusal
// that varied with an unrelated ancestor's permissions would depend on the
// machine, not the tracker), or when the working directory cannot be
// physically resolved.
//
// The walk exists ONLY to improve a refusal: it stats candidates and stops
// at the filesystem root — it opens no database, parses no dump, reads no
// file content and writes nothing. A verb never operates on a tracker it
// found above (§6.1). The bound is the filesystem root and nothing
// narrower — not a git boundary, not a mount: task #77 adjudicated the
// stranger-ancestor case as message-quality, the root the walk names is
// factually where a tracker sits whoever owns it, and a narrower bound
// chosen later would change a shipped refusal's behaviour where one chosen
// now is just the spec's.
func AncestorTracker() string {
	dir, ok := physicalWD()
	if !ok {
		return ""
	}
	rel := ""
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // filesystem root: nothing anywhere up the tree
		}
		rel = filepath.Join(rel, "..")
		st, err := os.Stat(filepath.Join(parent, instanceDirName))
		if err == nil && st.IsDir() {
			return rel
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "" // unreadable candidate: treated as absent, walk stops
		}
		dir = parent
	}
}

// NotFoundMessage renders §6.1's resolution refusal for a missing tracker
// artifact, in the three-case form the amendment specifies: with a tracker
// in an ancestor the message names that root and never mentions `init` —
// a reader following that advice would create a second, nested tracker —
// and with none anywhere up the tree the historical message stands
// verbatim, because there `init` is correct advice.
func NotFoundMessage(missing string) string {
	if root := AncestorTracker(); root != "" {
		return "no " + missing + " here; a tracker exists at " + root +
			" — run selftracked from that directory"
	}
	return "no " + missing + " here; run selftracked init first"
}
