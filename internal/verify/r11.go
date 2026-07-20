package verify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Spoloborota/selftracked/internal/rules"
)

// hookNames are the two hooks §9 chains. R11 checks each independently, so
// the pre-commit-chained-but-post-commit-not state is caught.
var hookNames = []string{"pre-commit", "post-commit"}

// r11 (advisory) is §7's per-machine gate-inactive warning. For each hook,
// the effective location (core.hooksPath when set, else <git-dir>/hooks)
// must either BE .selftracked/hooks or hold a hook file that references its
// .selftracked/hooks/ counterpart OUTSIDE a comment (best-effort textual
// detection, §9). Each unchained hook is named. Advisory: it never fails
// the run — a fresh clone that has not installed hooks is a warning, not a
// broken tracker.
func r11(ctx context.Context, dir string) ([]rules.Violation, error) {
	base := filepath.Dir(dir)
	hooksDir, err := effectiveHooksDir(ctx, base)
	if err != nil {
		return nil, err
	}
	// The effective location IS our hooks dir: every hook is chained.
	if samePath(hooksDir, filepath.Join(dir, "hooks")) {
		return nil, nil
	}
	var out []rules.Violation
	for _, name := range hookNames {
		//nolint:gosec // hooksDir/name is the repo's own hook path, not user input
		content, err := os.ReadFile(filepath.Join(hooksDir, name))
		if err != nil || !hookReferences(content, name) {
			out = append(out, rules.Violation{
				Rule:    "R11",
				Message: fmt.Sprintf("%s hook does not chain .selftracked/hooks/%s (gate inactive on this machine)", name, name),
			})
		}
	}
	return out, nil
}

// hookReferences reports whether content chains the .selftracked/hooks/
// counterpart named `name`. A reference counts only OUTSIDE a comment:
// everything from the first '#' on a line is stripped before the match, so
// a whole-line `#` comment and a trailing `# ...` comment both fail to
// match — false silence is this rule's fail-open direction, so a commented
// mention must not read as a live chain (§7). Accepted, documented residual
// (INV-309): a quoted '#' inside a live string BEFORE the real reference
// defeats the naive strip — "best-effort textual detection" is the ceiling.
func hookReferences(content []byte, name string) bool {
	target := ".selftracked/hooks/" + name
	for line := range strings.SplitSeq(string(content), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		if strings.Contains(line, target) {
			return true
		}
	}
	return false
}

// effectiveHooksDir returns the directory git actually runs hooks from:
// core.hooksPath when set, else <git-dir>/hooks. A missing core.hooksPath
// key makes `git config --get` exit non-zero — that is the unset case, not
// an error.
func effectiveHooksDir(ctx context.Context, base string) (string, error) {
	// Fixed argv, no shell: base is the repo root, the rest are literals.
	cfg := exec.CommandContext(ctx, "git", "-C", base, "config", "--get", "core.hooksPath") //nolint:gosec // fixed argv
	if out, err := cfg.Output(); err == nil {
		if hp := strings.TrimSpace(string(out)); hp != "" {
			if !filepath.IsAbs(hp) {
				hp = filepath.Join(base, hp)
			}
			return hp, nil
		}
	}
	gitDirCmd := exec.CommandContext(ctx, "git", "-C", base, "rev-parse", "--git-dir") //nolint:gosec // fixed argv
	out, err := gitDirCmd.Output()
	if err != nil {
		return "", fmt.Errorf("verify R11: %s is not a git repository: %w", base, err)
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(base, gitDir)
	}
	return filepath.Join(gitDir, "hooks"), nil
}

// samePath compares two paths by their real (symlink-resolved) form, with
// a lexical fallback when either does not yet exist.
func samePath(a, b string) bool {
	ra, ea := filepath.EvalSymlinks(a)
	rb, eb := filepath.EvalSymlinks(b)
	if ea == nil && eb == nil {
		return ra == rb
	}
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return aa == bb
}
