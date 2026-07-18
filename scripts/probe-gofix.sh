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

out=$(cd "$work" && go fix -diff ./... 2>&1) && status=0 || status=$?

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
