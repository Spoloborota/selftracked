# S5b opening record

Stage: S5b — relation/artifact/dictionary verbs (plan §4). Opened
2026-07-19, per D-EP13. Spec revision at open: 3.14. Plan revision at
open: 13. Rows owned at open: 32. After the open: 31 — one moved.

Scope: `rel` (add/rm/tree/cycles), `link`/`unlink` (+ archive/unarchive),
`paths` (ls/set/move `--with-files`), `config` (ls/set), `stale`, `log` —
all on the S5a write pipeline. The DoD's stated split holds: `epic:SLUG`
link targets and the epic-linked `stale` path re-verify at S6 close.
INV-153/154/155 (the ratified link-tables amendment's inverted rows)
close here with the sanctioned deleters.

**Resolved verification commands.** Every row's fixture runs as a
testscript scenario named in its row (`go test ./internal/cli -run
TestScripts/<scenario-file>`); the scenario files group per verb:
rel.txtar, link.txtar, paths-config.txtar, stale-log.txtar. Placement and
content: all 31 kept rows checked against §5.2/§5.6/§5.8 comments and
§6.2's verb rows — statements faithful, each executable by an S5b verb.

## Rows moved (placement defects)

| Row | To | Why |
|---|---|---|
| INV-247 | S8a | "A non-English crew adapts the `PO:` literal in its prompt config" — the carrying artifact is the generated PROMPT.md, S8a's deliverable. |

## Notable realizations (the rest are their row texts, one scenario each)

- INV-007/050: `paths move` one-row re-root with refs resolving before and
  after; scope `''` vs named scope resolution.
- INV-058/059: raw-write protection for meta keys is VERB-level (config
  refuses system keys; no other verb writes user keys) — the schema
  cannot distinguish writers, which is why these are verb-contract rows.
- INV-096: relates stored once, `rel tree`/`show` list it from both ends.
- INV-097/203 + INV-098/204: one cycle fixture, one target-status fixture
  (each pair states the same obligation from §5.6 and §6.2).
- INV-153: `rel rm` deletes + events row; reopen half already proven at
  S5a's scenario (cross-ref).
- INV-187: grammar invariant — a bare `archive` token never parses as a
  ref (already pinned in internal/ref's tests; the link dispatch fixture
  re-proves it end to end).
- INV-209: `..` and absolute relpaths refuse (containment; the DoD names
  this fixture class explicitly).
- INV-215/216: `stale` needs real git — the scenario builds a repo,
  commits, changes files, asserts the intersection and path-ASC order.

## What this open did not do

No code was written. Close review: check that `stale` stays read-only
(it intersects git state but must not write tracker state), and that
`log` is a plain read verb on the events index.
