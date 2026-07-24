#!/bin/sh
# Docs link-check (plan S12): every relative markdown link in the tracked
# documentation must resolve to an existing file. External URLs are out of
# scope (no network in gates); anchors are stripped. Inline links only:
# reference-style links ([text][ref]) are not matched — none exist in this
# repository's docs, and adding one silently escapes this check.
set -eu
cd "$(dirname "$0")/.."
fail=0
# shellcheck disable=SC2046
for f in README.md CONTRIBUTING.md $(find docs -name '*.md'); do
  dir=$(dirname "$f")
  links=$(grep -o ']([^)]*)' "$f" | sed 's/^](//; s/)$//' \
    | grep -v '^https\?:' | grep -v '^mailto:' | grep -v '^#' || true)
  for l in $links; do
    target=${l%%#*}
    [ -z "$target" ] && continue
    # code snippets like `steps[v](...)` match the link pattern; a target
    # made only of dots is not a path
    case $target in *[!.]*) ;; *) continue ;; esac
    case $target in
      /*) path=".$target" ;;
      *) path="$dir/$target" ;;
    esac
    if [ ! -e "$path" ]; then
      echo "BROKEN: $f -> $l"
      fail=1
    fi
  done
done
if [ "$fail" -eq 0 ]; then
  echo "check-doc-links: all relative links resolve"
fi
exit $fail
