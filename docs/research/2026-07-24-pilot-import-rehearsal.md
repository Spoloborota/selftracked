# Pilot import rehearsal — rungs 3–4 of the D13 ladder (2026-07-24)

The first external pilot of the importer, run at stage S12 against a
private local project ("the pilot client" throughout; nothing
client-identifying appears in this repository's tracked surface — the
disposable clone, the derived corpus, the derivation script and the
per-round artifacts live under the gitignored local path). This document
is the public record of the method and the findings; it is deliberately
free of client content.

## Method

Rung 3 (disposable clone): `git clone --no-hardlinks` of the client into
the gitignored path; a derivation script turns the client's prose
backlog (a task-registry table and per-epic files with card sections)
into the importer's JSON corpus; then repeated from-scratch rounds —
delete `.selftracked/`, `init`, `import --legacy`, full `verify` — with
each round's import log, verify log and dump archived privately.
Determinism across rounds is judged by normalizing the ISO-8601
timestamps out of both dumps and byte-comparing the residuals — not by
eyeballing the diff.

Rung 4 (colocated live install), only after the corpus round-tripped
green: `init` in the live client (incumbent hooks detected → chaining
recipe, never takeover), the real import, full `verify`, the chained
lines added to the host's hooks exactly per the recipe, and the whole
footprint left uncommitted for the owner's review.

## What the rehearsal found

Importer-side (filed as tracker tasks before any fix):

- **#8 — forward `dup_of` refuses.** The importer resolves a
  `DUPLICATE`'s canonical against already-inserted tasks, so a corpus
  whose canonical appears after its duplicate refuses. Real backlogs
  renumber canonicals late, so this is a natural shape; it is now a
  documented authoring constraint (canonical-before-duplicate) pending
  an S9-surface decision on two-pass resolution.
- **#9 — an unresolvable cited commit imports silently.** A sha-shaped
  string that is not a commit of the repository is accepted at import
  (the git-first date engine falls back to synthesis) and surfaces only
  as a later `verify` R5 failure. Real prose carries sha-shaped strings
  that are not commits (content-hash identities), so corpus derivation
  must validate citations against git — the guide now says so.

Derivation-side (found by the close review's mechanics critic, fixed and
re-run before the live install was redone):

- A table cell containing a literal `|` silently corrupted the row's
  parsed status and note (the row still looked ordinary).
- A fixed-width text window for harvesting commit citations attributed
  one card's commit to its neighbours; the window is now bounded at the
  next card heading.
- Card headings with trailing text after the status word, or with the
  status marker on a continuation line, were silently dropped.
- The first derivation proved "verify green" only for the corpus it had
  actually produced — a reduced and partly corrupted one. The lesson
  generalizes: **a green import round is evidence about the corpus, not
  about the derivation**; auditing the derivation against the source is
  a separate check, and the per-round artifacts must survive for a
  reviewer to re-judge.

## Outcome

After the derivation fixes: two archived from-scratch rounds, full
`verify` 0 violations each, timestamp-normalized dumps byte-identical;
the live colocated install repeated with the corrected corpus, full
`verify` 0 violations, the host's own gate authoritative with the
chained calls in the recipe's exact form, and the client-side footprint
minimal (three modified files, the rest new) and entirely uncommitted.
The importer needed no code changes to pass its first real corpus — the
two filed tasks are behavior-sharpening, not failures — and every
defect the rehearsal itself surfaced was in the corpus derivation, which
is exactly the layer the migration guide now warns about.
