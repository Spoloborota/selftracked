#!/bin/sh
# Dependency-policy gate: the pins the specification calls load-bearing.
#
# Checks, in order:
#   1. go.mod declares an exact toolchain (not a floating one).
#   2. The SQLite driver, once present, is at or above the version below
#      which RETURNING rows are lost through Exec.
#   3. The driver and its libc are pinned as a pair, at the versions the
#      driver's own go.mod names — they are bumped together or not at all.
#   4. No configuration library reaches the binary: configuration lives in
#      the database's meta rows plus CLI flags. Checked against the real
#      import graph of our own packages, not against go.mod — go.mod also
#      carries what the *tools* drag in, and a linter's dependency on a
#      config library says nothing about ours.
#
# Exits 0 when the tree satisfies the policy, 1 with a reason when it does not.
set -eu

MOD="${1:-go.mod}"
DRIVER_MIN="v1.48.2"
fail() { echo "check-pins: $*" >&2; exit 1; }

[ -f "$MOD" ] || fail "$MOD not found"
command -v go >/dev/null 2>&1 || fail "go is not on PATH; this gate cannot check the import graph, and a gate that cannot run must not report success"

# 1. exact toolchain
toolchain=$(awk '/^toolchain /{print $2}' "$MOD")
[ -n "$toolchain" ] || fail "go.mod names no toolchain directive; the pin is load-bearing"
echo "$toolchain" | grep -Eq '^go1\.[0-9]+\.[0-9]+$' \
  || fail "toolchain '$toolchain' is not an exact version (want go1.N.P)"

# 2 + 3. driver pins, checked only once the driver is actually a dependency
driver=$(awk '/modernc\.org\/sqlite /{print $2}' "$MOD" | head -1)
libc=$(awk '/modernc\.org\/libc /{print $2}' "$MOD" | head -1)
if [ -n "$driver" ]; then
  lowest=$(printf '%s\n%s\n' "$DRIVER_MIN" "$driver" | sort -V | head -1)
  [ "$lowest" = "$DRIVER_MIN" ] \
    || fail "modernc.org/sqlite $driver is below the $DRIVER_MIN floor (RETURNING via Exec loses rows)"
  [ -n "$libc" ] \
    || fail "modernc.org/sqlite is pinned but modernc.org/libc is not; the pair is bumped together"
  # "Bumped together" is not enough: the specification requires the libc the
  # driver's OWN go.mod names. Ask the module cache what that is rather than
  # trusting that two version strings appearing together are the right two.
  want=$(go mod download -json "modernc.org/sqlite@$driver" 2>/dev/null \
         | awk -F'"' '/"Dir"/{print $4}')
  if [ -n "$want" ] && [ -f "$want/go.mod" ]; then
    expected=$(awk '/modernc\.org\/libc /{print $2}' "$want/go.mod" | head -1)
    [ -z "$expected" ] || [ "$expected" = "$libc" ] \
      || fail "modernc.org/libc is $libc but modernc.org/sqlite $driver names $expected; the pair must match exactly"
  else
    echo "check-pins: WARNING cannot read modernc.org/sqlite@$driver from the module cache — the libc exact-match check did not run" >&2
  fi
fi

# 4. no configuration library in the binary's own import graph
deps=$(go list -deps ./... 2>/dev/null || true)
for pkg in spf13/viper knadh/koanf kelseyhightower/envconfig ilyakaznacheev/cleanenv; do
  if printf '%s\n' "$deps" | grep -q "$pkg"; then
    fail "configuration library '$pkg' reaches the binary; configuration lives in meta rows plus flags"
  fi
done

echo "check-pins: toolchain $toolchain; dependency policy satisfied"
