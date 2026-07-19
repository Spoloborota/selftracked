# Change: §5.7's worklog.story comment names the wrong guard rule

Target: `docs/v0-spec.md` §5.7 (one comment line)
Status: **accepted** · raised 2026-07-19 at the S1b open · ratified and applied same day

## Why

The `worklog.story` column comment in §5.7 reads: "FK-free by design
(composite target + V-rows); guarded by R5." The rule that actually guards
story membership is **R4**: "`worklog.story` ∈ stories(epic) or `V-[0-9]+`
(V only on CLOSED epics)…" (§7, Stage 1 table). R5 is a different rule —
"Non-`legacy:` commits resolve via `git cat-file`" — which never looks at
`worklog.story`.

The pointer is internally stale, not ambiguous: §7's own `--fast` partition
lists R4 among the pure-SQL rules and R5 among the git-bound ones, and the
membership check is pure SQL. A reader following the comment to understand
what compensates for the missing FK lands on a rule about commit hashes.

Found by the S1b stage-open re-read (D-EP13), which checked each row's
statement against the spec text it anchors; inventory row INV-109 inherits
the wrong pointer verbatim from the spec.

## What changes

**Spec §5.7**, the `worklog.story` column comment: "guarded by R5" →
"guarded by R4". Nothing else — the design (FK-free column, verify-rule
compensation) is untouched; only the rule id is corrected.

**Follow-through once ratified:** inventory row INV-109's statement is
corrected the same way (a row edit at its stage, not an amendment — the
row tracks the spec). The S1b opening record already states its INV-109
verdict against the R4 reading.

## Re-walk consequence (plan §3 rule 3)

The amendment touches one spec comment line whose obligation is already
carried by INV-109 (the stated limitation) and the R4 conjunct rows
(INV-296, INV-297). No row gains or loses scope; no `verified` status is
disturbed — the correction changes which rule id the text names, not what
any stage must build or prove.

## Ratification

Owner, 2026-07-19, on the proposal as filed: **"ratify"** — accepted as
proposed, no scope changes requested. (Recorded in English by meaning; the
direction was given in conversation.)
