package scaffold

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// trustNote is the one-line disclosure §9 attaches to both activation forms:
// tracked hooks run code on the committer's machine (INV-419, INV-488).
const trustNote = "(repo-tracked hooks execute on your machine — enable only on repos you trust.)"

// hookActivation returns the per-machine advice init prints after
// scaffolding (§9). On a repo with no incumbent hooks it is the takeover
// command; when `core.hooksPath` is already set or an incumbent pre/post-commit
// exists, printing the takeover would silently disable the host project's
// gates, so init prints a chaining recipe instead (INV-419–424). Advice
// only — a git failure degrades to the safe takeover text rather than
// erroring init.
func hookActivation(ctx context.Context, root string) string {
	hooksPath := gitHooksPath(ctx, root)
	incumbent := filepath.Join(root, ".git", "hooks")
	if hooksPath != "" {
		incumbent = hooksPath
		if !filepath.IsAbs(incumbent) {
			incumbent = filepath.Join(root, incumbent)
		}
	}
	// A set hooksPath, or a live incumbent pre/post-commit, means takeover
	// would clobber an existing gate — chain instead (INV-420).
	if hooksPath != "" || hasLiveHook(incumbent, "pre-commit") || hasLiveHook(incumbent, "post-commit") {
		return chainingRecipe(incumbent)
	}
	return takeoverAdvice()
}

// takeoverAdvice is the clean-repo path (INV-419).
func takeoverAdvice() string {
	return "To activate the selftracked gate on this machine:\n" +
		"    git config core.hooksPath .selftracked/hooks\n" +
		trustNote
}

// chainingRecipe covers both hooks (INV-421): the incumbent pre-commit
// invokes ours with the exit status propagated (INV-422 — without `|| exit $?`
// a RED verify degrades to advisory); the incumbent post-commit invokes ours
// at its TOP (INV-423 — warn-only, safe first; appended at the bottom an
// early `exit` in the incumbent could skip it). Both must run as
// subprocesses, never `source`d (INV-424 — the scripts `exit` internally and
// would terminate the incumbent hook).
func chainingRecipe(incumbent string) string {
	pre := filepath.Join(incumbent, "pre-commit")
	post := filepath.Join(incumbent, "post-commit")
	return "This repo already runs git hooks; repointing core.hooksPath would disable them.\n" +
		"Chain selftracked instead — add each line as a SUBPROCESS call (never `source`;\n" +
		"the scripts exit internally and would abort your hook):\n" +
		"    in " + pre + " :\n" +
		"        .selftracked/hooks/pre-commit || exit $?\n" +
		"    at the TOP of " + post + " :\n" +
		"        .selftracked/hooks/post-commit\n" +
		trustNote
}

// hasLiveHook reports whether name is an active (non-.sample) hook file in
// dir. Git ships `*.sample` templates that are inert, so their presence does
// not count as an incumbent.
func hasLiveHook(dir, name string) bool {
	fi, err := os.Stat(filepath.Join(dir, name))
	return err == nil && !fi.IsDir()
}

// gitHooksPath returns core.hooksPath, or "" when unset or unavailable.
func gitHooksPath(ctx context.Context, root string) string {
	// Fixed argv, no shell: root is the repo, the rest are literals.
	cmd := exec.CommandContext(ctx, "git", "-C", root, "config", "--get", "core.hooksPath") //nolint:gosec // fixed argv
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
