#!/bin/sh
# Red fixture for the installed-copy drift gate — docs/v0-spec.md section 16,
# property 3: a planted one-character divergence exits non-zero and identity
# exits 0, because a gate that cannot fail is decoration.
#
# The gate compares files that live in the working tree, so proving it fails
# must not mean corrupting one: a fixture killed between planting a divergence
# and restoring it would strand a mutilated PROMPT.md, and the repository
# would carry the damage into its next commit. This fixture therefore never
# reads, writes or diffs a tracked file. It asks the gate for its pair table
# (--list), builds a scratch tree with the same relative paths under a
# temporary directory, and points the gate there through its ROOT argument.
# Nothing in the working tree is touched, so nothing has to be cleaned up for
# it to stay pristine — removing the temporary tree on exit is tidiness, not
# correctness.
#
# Every row of the table is exercised rather than one representative row: a
# row the gate silently ignored — a typo in a path, a loop that stops at the
# first pair — would show up here as a green run after a planted divergence.
#
# Exits 0 when the gate behaves as specified; 1 with the failing assertion.
set -eu

gate="$(dirname "$0")/check-installed-copies.sh"
[ -f "$gate" ] || {
  echo "check-installed-copies-fixture: $gate not found" >&2
  exit 1
}

fail() {
  echo "check-installed-copies-fixture: FAILED — $*" >&2
  exit 1
}

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT INT TERM

out=""
status=0
# The gate is run with stdin closed. It does not need to be: read 2026-07-26,
# every loop in check-installed-copies.sh is fed by its own here-document and
# nothing there reads inherited stdin, so today this redirect changes no
# behaviour. It is kept as insulation, because run_gate is called from inside
# a `while read` loop fed by a here-document: were the gate ever to grow a
# read from stdin, it would consume the rows this fixture has not processed
# yet and the fixture would silently stop exercising them.
run_gate() {
  out=$(sh "$gate" "$work" 2>&1 </dev/null) && status=0 || status=$?
}

rows=$(sh "$gate" --list)
[ -n "$rows" ] || fail "the gate lists no pairs to guard"

# A row is two whitespace-separated paths. The gate refuses a row of any other
# shape, so this fixture must recognise the same shape: reading two fields
# where the gate reads three would turn a malformed row into a green gate the
# fixture then reports as "identity must exit 0, got 1" — blaming the gate for
# refusing correctly. Called from both loops below.
check_row() {
  if [ -n "$3" ] || [ -z "$2" ]; then
    fail "malformed pair row from the gate's --list (expected 'installed template'): $1 $2 $3"
  fi
}

# Build the scratch tree from the gate's own table, so adding a row to the
# gate needs no change here.
count=0
while read -r installed template extra; do
  [ -n "$installed" ] || continue
  check_row "$installed" "$template" "$extra"
  count=$((count + 1))
  mkdir -p "$work/$(dirname "$installed")" "$work/$(dirname "$template")"
  printf 'generated document for %s\nsecond line\n' "$installed" >"$work/$template"
  cp "$work/$template" "$work/$installed"
done <<ROWS
$rows
ROWS

# Green: identity exits 0.
run_gate
if [ "$status" -ne 0 ]; then
  fail "identity must exit 0, got $status:
$out"
fi

# The step names what it does not compare (spec section 16, property 2).
case "$out" in
*".claude/settings.json"*) ;;
*) fail "the gate must name .claude/settings.json as not compared; it printed:
$out" ;;
esac

# Red, row by row.
while read -r installed template extra; do
  [ -n "$installed" ] || continue
  check_row "$installed" "$template" "$extra"

  printf 'x' >>"$work/$installed" # a one-character divergence
  run_gate
  if [ "$status" -eq 0 ]; then
    fail "a one-character divergence in $installed did not fail the gate"
  fi
  # The gate's drift report for this row, matched as a whole line. A bare
  # substring search over the combined output would also be satisfied by the
  # not-compared note, which the gate prints on every run and which names a
  # path of its own — so a row whose path happened to be a substring of that
  # fixed boilerplate would pass this assertion with the gate having flagged
  # nothing.
  if ! printf '%s\n' "$out" |
    grep -qxF "check-installed-copies: DRIFT $installed differs from $template"; then
    fail "the gate failed without reporting drift for $installed:
$out"
  fi
  cp "$work/$template" "$work/$installed"

  rm "$work/$installed" # a missing installed copy is drift too
  run_gate
  if [ "$status" -eq 0 ]; then
    fail "a missing $installed did not fail the gate"
  fi
  cp "$work/$template" "$work/$installed"

  run_gate
  if [ "$status" -ne 0 ]; then
    fail "restoring $installed left the gate red ($status):
$out"
  fi
done <<ROWS
$rows
ROWS

echo "check-installed-copies-fixture: identity exits 0; a one-character divergence and a missing copy each exit non-zero, on each of the $count guarded rows"
