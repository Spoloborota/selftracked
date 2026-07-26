package instance

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var hex12 = regexp.MustCompile(`^[0-9a-f]{12}$`)

// mkTracker creates <root>/.selftracked and returns root.
func mkTracker(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, instanceDirName), 0o750); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestDigestShapeStabilityAndSalt: the §11.1 contract in one arc — first
// use creates the salt (owner-only, one hex line), the digest is 12 hex
// characters, and a second run reads the same salt and reports the same
// identity.
func TestDigestShapeStabilityAndSalt(t *testing.T) {
	root := mkTracker(t)
	t.Chdir(root)

	d1, ok := Digest(instanceDirName)
	if !ok {
		t.Fatal("digest not computed on a healthy tracker")
	}
	if !hex12.MatchString(d1) {
		t.Fatalf("digest %q is not 12 hex characters", d1)
	}
	saltPath := filepath.Join(instanceDirName, saltFile)
	st, err := os.Stat(saltPath)
	if err != nil {
		t.Fatalf("first use must create the salt: %v", err)
	}
	if st.Mode().Perm() != saltMode {
		t.Fatalf("salt mode %o, want %o", st.Mode().Perm(), saltMode)
	}
	b, err := os.ReadFile(saltPath)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSuffix(string(b), "\n")
	if len(line) != saltBytes*2 || !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(line) {
		t.Fatalf("salt content is not one hex line: %q", string(b))
	}

	d2, ok := Digest(instanceDirName)
	if !ok || d2 != d1 {
		t.Fatalf("digest changed between runs: %q then %q (ok=%v)", d1, d2, ok)
	}
}

// TestDigestDistinguishesACopy: the motivating case — a cp -a copy carries
// the salt with it and still reports a different identity, because the
// path half differs (§11.1: copying the salt is harmless precisely because
// the salt is not the identity).
func TestDigestDistinguishesACopy(t *testing.T) {
	src := mkTracker(t)
	t.Chdir(src)
	d1, ok := Digest(instanceDirName)
	if !ok {
		t.Fatal("source digest not computed")
	}

	copyRoot := mkTracker(t)
	salt, err := os.ReadFile(filepath.Join(src, instanceDirName, saltFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(copyRoot, instanceDirName, saltFile), salt, saltMode); err != nil {
		t.Fatal(err)
	}
	t.Chdir(copyRoot)
	d2, ok := Digest(instanceDirName)
	if !ok {
		t.Fatal("copy digest not computed")
	}
	if d1 == d2 {
		t.Fatalf("a copy carrying the source's salt reported the source's identity %q", d1)
	}
}

// TestDigestIsSaltedPathDigest pins the preimage: SHA-256(salt || path)
// truncated to 12 hex, with the path physically resolved — the test
// tracker lives under a symlinked temp root on macOS, so computing the
// expectation from the physical path is what proves the basis.
func TestDigestIsSaltedPathDigest(t *testing.T) {
	root := mkTracker(t)
	seed := []byte("known-salt")
	if err := os.WriteFile(filepath.Join(root, instanceDirName, saltFile), append(seed, '\n'), saltMode); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	phys, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(append(seed, phys...))
	want := hex.EncodeToString(sum[:])[:12]
	got, ok := Digest(instanceDirName)
	if !ok || got != want {
		t.Fatalf("digest %q (ok=%v), want SHA-256(salt||physical path)[:12] = %q", got, ok, want)
	}
}

// TestDigestOmittedNeverDowngraded: the two decided failure modes — an
// uncreatable salt (read-only instance dir) and an empty salt file — both
// resolve to ok=false, never to an unsalted or empty-salted digest.
func TestDigestOmittedNeverDowngraded(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure modes are invisible to root")
	}
	t.Run("uncreatable salt", func(t *testing.T) {
		root := mkTracker(t)
		dir := filepath.Join(root, instanceDirName)
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })
		t.Chdir(root)
		if d, ok := Digest(instanceDirName); ok {
			t.Fatalf("digest %q computed with an uncreatable salt", d)
		}
	})
	t.Run("empty salt", func(t *testing.T) {
		root := mkTracker(t)
		if err := os.WriteFile(filepath.Join(root, instanceDirName, saltFile), []byte("\n"), saltMode); err != nil {
			t.Fatal(err)
		}
		t.Chdir(root)
		if d, ok := Digest(instanceDirName); ok {
			t.Fatalf("digest %q computed from an empty salt — the oracle is back", d)
		}
	})
}

// TestAncestorTracker: the walk names the nearest ancestor root relative
// to the working directory, skips a FILE named .selftracked, and returns
// nothing from the root itself (the walk looks only upward).
func TestAncestorTracker(t *testing.T) {
	root := mkTracker(t)
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}

	t.Chdir(deep)
	if got := AncestorTracker(); got != filepath.Join("..", "..") {
		t.Fatalf("from two levels down: %q, want ../..", got)
	}
	t.Chdir(filepath.Join(root, "a"))
	if got := AncestorTracker(); got != ".." {
		t.Fatalf("from one level down: %q, want ..", got)
	}
	t.Chdir(root)
	if got := AncestorTracker(); got != "" {
		t.Fatalf("from the tracker root itself the walk looks only upward, got %q", got)
	}

	// A plain FILE named .selftracked is not a tracker: skipped, and the
	// real root above it is still found.
	between := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(between, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "b", instanceDirName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(between)
	if got := AncestorTracker(); got != filepath.Join("..", "..", "..") {
		t.Fatalf("a file named .selftracked must be skipped: %q, want ../../..", got)
	}
}

// TestAncestorTrackerUnreadableStopsSilently reproduces the amendment's
// permission case: the first candidate the walk cannot read ends it, so a
// tracker sitting above the opaque directory is NOT reported and the
// caller falls back to the historical message.
func TestAncestorTrackerUnreadableStopsSilently(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure modes are invisible to root")
	}
	root := mkTracker(t)
	deep := filepath.Join(root, "outer", "inner", "deep")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(deep)
	inner := filepath.Join(root, "outer", "inner")
	if err := os.Chmod(inner, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(inner, 0o750) })
	if got := AncestorTracker(); got != "" {
		t.Fatalf("an unreadable candidate must end the walk, got %q", got)
	}
}

// TestNotFoundMessage: the two branches of §6.1's refusal — the ancestor
// form never mentions init, the nothing-anywhere form is the historical
// message verbatim.
func TestNotFoundMessage(t *testing.T) {
	root := mkTracker(t)
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	got := NotFoundMessage(".selftracked/db.sqlite")
	if strings.Contains(got, "init") {
		t.Fatalf("the ancestor refusal advises init: %q", got)
	}
	if !strings.Contains(got, "a tracker exists at ..") {
		t.Fatalf("the ancestor refusal does not name the root: %q", got)
	}

	bare := t.TempDir()
	t.Chdir(bare)
	got = NotFoundMessage(".selftracked/db.sqlite")
	if got != "no .selftracked/db.sqlite here; run selftracked init first" {
		t.Fatalf("the nothing-anywhere message must stand verbatim, got %q", got)
	}
}
