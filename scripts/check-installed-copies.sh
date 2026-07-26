#!/bin/sh
# Installed-copy drift gate — docs/v0-spec.md section 16, amendment
# openspec/changes/gates-catch-installed-copy-drift/.
#
# This repository is self-hosted on its own scaffold, so several of the
# documents it works from are copies of files in internal/scaffold/templates/.
# `init` never overwrites a file that already exists — the property that makes
# it safe to re-run on a live repository — so editing a template propagates to
# nothing already installed, this repository included. Nothing else asserts
# that the copies stay copies, and the drift that followed was filed as a task
# before this gate existed.
#
# The comparison is byte-for-byte. The failure class is a template edit that
# did not propagate, and any tolerance is a hole shaped like the next
# incident.
#
# Usage:
#   check-installed-copies.sh [ROOT]   compare every guarded pair under ROOT
#   check-installed-copies.sh --list   print the pair table, one row per line
#
# ROOT defaults to the current directory, which is the repository root when
# make runs this: make runs a recipe at the Makefile's directory, so there is
# no root to resolve and the paths below are literal. The argument exists for
# the fixture, which points this gate at a scratch tree it builds itself
# rather than mutating a tracked file to prove the gate can fail.
#
# Exits 0 when every guarded pair is byte-identical, 1 when any is not.
set -eu

# The guarded pairs, as data: the installed copy, then the template it is
# generated from. Guarding one more file is one more row here — deliberately
# not a code path, and the fixture reads this same table through --list.
pairs='PROMPT.md internal/scaffold/templates/PROMPT.md
.claude/skills/selftracked/SKILL.md internal/scaffold/templates/claude_skill.md
.claude/rules/selftracked.md internal/scaffold/templates/claude_rule.md'

# The table is validated once, before anything reads it — --list included,
# because the fixture builds its scratch tree from --list and a row with a
# '..' component would put files outside the temporary directory it cleans
# up. A '..' row is a defect in the table itself, not drift in a pair, so it
# refuses the whole run rather than one comparison.
table_rows=0
while read -r row; do
  [ -n "$row" ] || continue
  table_rows=$((table_rows + 1))
  set -f # split the row into words without expanding a path as a glob
  for word in $row; do
    case "$word" in
    .. | ../* | */.. | */../*)
      set +f
      echo "check-installed-copies: pair row contains a '..' path component: $row" >&2
      exit 1
      ;;
    esac
  done
  set +f
done <<PAIRS
$pairs
PAIRS

if [ "${1:-}" = "--list" ]; then
  printf '%s\n' "$pairs"
  exit 0
fi

# "$1 unset" and "$1 passed as an empty string" are different mistakes:
# the first is the documented default, the second is a caller that computed a
# root and got nothing. ${1:-.} cannot tell them apart, so it is not used.
if [ "$#" -ge 1 ]; then
  root="$1"
  [ -n "$root" ] || {
    echo "check-installed-copies: ROOT was passed as an empty string; pass a directory, or pass no argument for the default '.'" >&2
    exit 1
  }
else
  root=.
fi
[ -d "$root" ] || {
  echo "check-installed-copies: ROOT '$root' is not a directory" >&2
  exit 1
}

# How many files `init` installs from a template: the entries of staticFiles
# plus those of hookFiles in internal/scaffold/scaffold.go (11 + 2, read there
# 2026-07-26). Hard-coded because parsing Go from a shell gate is worse than a
# number that moves when the scaffold does; the guarded count beside it is
# derived from the table above, so adding a guarded pair stays a row and does
# not falsify this note.
installs_from_templates=13

# Printed on every run, green or red: the scaffold installs several times as
# many files as this gate guards, and "the drift gate is green" must not be
# read as "every generated file matches" — most generated files are unguarded,
# and one of them is genuinely, legitimately divergent. The note carries the
# scale as a number because "some files are not compared" reads as two or
# three, and it is not.
not_compared() {
  unguarded=$((installs_from_templates - table_rows))
  cat <<NOTE
check-installed-copies: NOT compared — $unguarded pairs. init installs
  $installs_from_templates files from templates and this gate guards
  $table_rows; the list of the rest is internal/scaffold/scaffold.go
  (staticFiles, hookFiles), not a copy of it here that would rot. One of the
  unguarded is named because it is the one that must stay unguarded:
  .claude/settings.json against
  internal/scaffold/templates/claude_settings.json. This repository's settings
  carry local configuration an adopter's scaffold has no business receiving,
  so the two are legitimately different and a byte-equality guard over them
  would be a gate red by design. Green here means the pairs above match, not
  that every generated file does.
NOTE
}

# irregular ROOT REL — true when REL under ROOT is not a plain file reached
# through plain directories: any path component that is a symlink — the
# final one or an ancestor — or a final component that exists but is not a
# regular file. Both [ -f ] and cmp follow symlinks, so a copy replaced by
# a link to an identical file elsewhere would compare equal while the
# tracked file has stopped being a copy at all — and the same swap one
# level up, a symlinked PARENT directory, was the gap filed as task #80:
# the first cut tested only the last component and passed it. The
# guarantee here is over the whole relative path. ROOT itself is the
# caller's business and is not walked: the fixture points this gate at a
# temporary directory that on macOS legitimately sits behind /var, a
# symlink. git does surface both swaps in `git status` (a typechange for
# the file, a deletion plus an untracked entry for the directory), so this
# stays a second line of defence rather than the only one. On return 0,
# $irregular_at names the offending path relative to ROOT.
irregular() {
  _walk=$1
  _rest=$2
  irregular_at=''
  while [ -n "$_rest" ]; do
    _comp=${_rest%%/*}
    if [ "$_comp" = "$_rest" ]; then _rest=''; else _rest=${_rest#*/}; fi
    [ -n "$_comp" ] || continue
    _walk="$_walk/$_comp"
    if [ -h "$_walk" ]; then
      irregular_at="${_walk#"$1"/}" # a symlink component, however it resolves
      return 0
    fi
  done
  if [ -e "$_walk" ] && [ ! -f "$_walk" ]; then
    irregular_at="${_walk#"$1"/}" # a directory, a fifo, a device
    return 0
  fi
  return 1 # absent, or a regular file behind regular directories
}

failed=0
count=0
while read -r installed template extra; do
  [ -n "$installed" ] || continue
  count=$((count + 1))

  # Rows are split on whitespace, so a path containing a space would land
  # half in $template and half in $extra and be compared against the wrong
  # file. Refuse the row instead of guessing.
  if [ -n "$extra" ] || [ -z "$template" ]; then
    echo "check-installed-copies: malformed pair row (expected 'installed template'): $installed $template $extra" >&2
    failed=1
    continue
  fi

  # Before the existence tests, which follow symlinks and would report a
  # linked-away copy as present and equal.
  if irregular "$root" "$template"; then
    echo "check-installed-copies: the template side of the pair is not a regular file behind regular directories: $template (at $irregular_at)" >&2
    failed=1
    continue
  fi
  if irregular "$root" "$installed"; then
    echo "check-installed-copies: DRIFT the installed side of the pair is not a regular file behind regular directories: $installed (at $irregular_at)" >&2
    echo "  A copy is a file behind real directories, not a link to either:" >&2
    echo "  cmp would follow the link and call it identical while $installed" >&2
    echo "  is no longer the copy." >&2
    failed=1
    continue
  fi

  if [ ! -f "$root/$template" ]; then
    echo "check-installed-copies: the pair table names a template that is not there: $template" >&2
    echo "  (running from the repository root? ROOT was '$root')" >&2
    failed=1
    continue
  fi
  if [ ! -f "$root/$installed" ]; then
    echo "check-installed-copies: DRIFT $installed is missing; $template says it should exist" >&2
    failed=1
    continue
  fi

  if cmp -s "$root/$installed" "$root/$template"; then
    echo "  ok  $installed == $template"
    continue
  fi

  failed=1
  echo "check-installed-copies: DRIFT $installed differs from $template" >&2
  echo "  read the difference first:  diff $template $installed" >&2
  echo "  then reconcile deliberately: a template edit that did not propagate is" >&2
  echo "  fixed by copying the template over the installed copy, while a defect" >&2
  echo "  found in the installed copy is fixed in the template and copied down." >&2
  echo "  Either side can be the right one, which is why nothing here copies" >&2
  echo "  anything for you." >&2
done <<PAIRS
$pairs
PAIRS

# A gate that compared nothing is not green. An emptied pair table would
# otherwise report "0 guarded pairs identical to their templates" and exit 0,
# which is the failure mode this whole step exists to make impossible.
if [ "$count" -eq 0 ]; then
  echo "check-installed-copies: the pair table is empty — nothing was compared" >&2
  exit 1
fi

if [ "$failed" -eq 0 ]; then
  echo "check-installed-copies: $count guarded pairs identical to their templates"
else
  echo "check-installed-copies: guarded pairs drifted from their templates" >&2
fi
not_compared
exit "$failed"
