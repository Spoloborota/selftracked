#!/bin/sh
# Empirical re-verification of an undocumented toolchain behaviour.
#
# The build gate rests on `go fix -diff` reporting pending modernisations by
# BOTH an empty-output contract and a zero exit status. The exit-status half
# is not documented anywhere, so it is proven here against a case that must
# produce a diff, on whatever toolchain is pinned — and re-proven whenever the
# pin moves. Traceability: this is the §16 re-verification item for `go fix`.
#
# Exits 0 when the behaviour holds; 1 with a reason when it does not.
set -eu

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cat > "$work/go.mod" <<'MOD'
module probe

go 1.25.0
MOD

# A loop `go fix` modernises into `for range n`, so a diff is guaranteed.
cat > "$work/probe.go" <<'GO'
package probe

func Count(n int) int {
	total := 0
	for i := 0; i < n; i++ {
		total += i
	}
	return total
}
GO

command -v go >/dev/null 2>&1 || {
  echo "probe-gofix: go is not on PATH — the probe cannot run, and a gate" >&2
  echo "  that cannot run must not report success" >&2
  exit 1
}

out=$(cd "$work" && go fix -diff ./... 2>&1) && status=0 || status=$?

# A non-zero status with no diff in it is a failure of the toolchain, not a
# pending fix. Without this the probe reported success on a 127 from a
# missing binary: output was non-empty and the status non-zero, which is
# exactly the shape it was looking for.
case "$out" in
  *"---"*|*"+++"*|*"@@"*) : ;;
  *)
    echo "probe-gofix: output is not a diff — go fix did not run as expected:" >&2
    printf '  %s\n' "$out" >&2
    exit 1
    ;;
esac

if [ -z "$out" ]; then
  echo "probe-gofix: expected a diff for a modernisable file, got none —" >&2
  echo "  the probe no longer produces a case this toolchain fixes" >&2
  exit 1
fi
if [ "$status" -eq 0 ]; then
  echo "probe-gofix: FAILED — a pending fix produced output but exit status 0." >&2
  echo "  The gate must not rely on the exit status alone on this toolchain." >&2
  exit 1
fi

echo "probe-gofix: pending fixes produce output and exit status $status (both halves of the gate hold)"
