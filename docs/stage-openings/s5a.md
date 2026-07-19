# S5a opening record

Stage: S5a — task-lifecycle verbs (plan §4). Opened 2026-07-19, per
D-EP13. Spec revision at open: 3.13. Plan revision at open: 12.
Rows owned at open: 58. After the open: 40 at S5a — eighteen moved.

This was the largest placement correction of the build so far: 58 rows
arrived here, of which eighteen belong to other stages. The pattern is
consistent — a task-lifecycle stage collected every row that *mentions* a
task verb, including rows whose obligation is an epic verb, a rel verb,
`verify`, the generated docs, or `load` (already built at S4). S5a's scope
is exactly nine verbs — create, show, list, ready, set-status, reopen,
park, unpark, edit — plus the events pipeline and the write-verb pipeline
with the **§8.4 core** (match / crash-residue heal / external-change
refuse / missing sidecar). Its DoD: a testscript per verb (happy path +
every documented refusal) and the §14 no-identity-leak fixture (verbs run
under sentinel HOME/USER, outputs clean).

**Resolved verification command.** Verb behaviour runs as testscript
scenarios (`go test ./internal/cli -run TestScripts/<scenario>`); pipeline
and events unit checks run in the package that owns them
(`go test ./internal/verb -run '<Test>'`).

## Rows moved (placement defects)

| Rows | To | Why |
|---|---|---|
| INV-218, INV-224, INV-225, INV-536, INV-540 | S6 | Epic verbs (create/activate/pause/dissolve/close/list). S5a builds no epic verb. |
| INV-235 | S6 | `story start` — a story verb. |
| INV-118 | S6 | The worklog correction row's close-rule-2 exclusion needs `worklog add --corrects`, the S6 verb. |
| INV-202, INV-203 | S5b | `rel` catalog facts (no duplicates type; cycle refusal) — `rel` is an S5b verb. The set-status side of the duplicates rule stays here (INV-095). |
| INV-350 | S7 | "`verify` reports a dirty dump" — the verify verb. |
| INV-412 | S8a | The `as of dump <sha12>` durable-doc anchor rule ships in the generated PROMPT.md (S8a). |
| INV-484, INV-485 | S8a | The privacy warning's content, and "init and PROMPT.md carry it" — init and the generated docs are S8a. The verbs' own no-identity-leak duty stays here (INV-487). |
| INV-277 | S8b | The gate-skip marker's conversion pairs with the `gate` verb that writes the marker (S8b). |
| INV-349 | S8c | "The skill ends every session with a bookkeeping commit" — the Claude Code skill (S8c). |
| INV-346, INV-347, INV-489 | S4 | `load --force` discards + summary; `load` temp-build + rename; `load` treats content untrusted. All three are load behaviours S4 already built and tested — handed back and flipped `verified-by-command` against S4's close, not left on a task-verb stage. |

## Per-row verdicts (40 kept)

Content `ok` = renders the current spec faithfully. Placement `ok` =
executable by S5a's verbs/pipeline.

| Row | Content | Placement | Verification (resolved) |
|---|---|---|---|
| INV-006 | ok — the S5a-relevant half is that the verbs never delete (schema-enforced) and archive semantics ride `archive`/`unarchive` (S5b); here it is the no-delete review | ok | review at close: no task verb issues a DELETE |
| INV-024 | ok | ok | testscript: set-status to IN-REVIEW represents a PO question; it is a state, not a kind |
| INV-054 | ok | ok | events-date-iso-utc-go-generated: an events row's `at` is ISO-8601 UTC the binary wrote |
| INV-055 | ok | ok | no-verb-accepts-a-date-flag: create/set-status/edit reject a `--date`/`--at` flag |
| INV-087 | ok | ok | set-status-dup-of-refuses-a-duplicate-target |
| INV-088 | ok | ok | status-transition-clears-parked-logged |
| INV-089 | ok | ok | label-hidden-from-list-and-ready |
| INV-095 | ok — the writer/remover half (set-status/reopen); "rel never offers it" re-proven at S5b (INV-202) | ok | duplicates-link-written-by-set-status-removed-by-reopen |
| INV-138 | ok | ok | events-detail-carries-note-verbatim |
| INV-139 | ok | ok | edit-events-record-field-old-new-bounded-prefix |
| INV-141 | ok | ok | edit-old-value-only-in-prior-dump: two commits, the old value absent from the events row, present in the prior dump |
| INV-142 | ok | ok | double-edit-one-session-only-prefix-survives |
| INV-143 | ok | ok | in-review-cycle-keeps-superseded-verdict-in-trail |
| INV-180 | ok — the pipeline ORDER; the version gate is the S2 stub, §8.4 core lands here, migration escalation is S11 | ok | write-verb-pipeline-order: gate → divergence → EXCLUSIVE → tx(mutate+events) → commit → regen dump+STATE → sidecar → optimize |
| INV-181 | ok — read verbs (show/list/ready) run the gate then query_only | ok | read-verb-runs-gate-then-query_only-no-side-effects |
| INV-189 | ok | ok | create-signature-and-prints-ref |
| INV-190 | ok | ok | create-cannot-produce-a-terminal-status |
| INV-191 | ok | ok | show-accepts-any-ref-artifact-reverse-lookup |
| INV-192 | ok | ok | list-filters-by-status-epic-parked-labels |
| INV-193 | ok | ok | ready-returns-v_ready-open-unparked-deps-terminal-id-order |
| INV-194 | ok | ok | set-status-matrix-checked-dup-of-writes-link |
| INV-195 | ok | ok | set-status-refuses-terminal-to-open |
| INV-196 | ok | ok | in-review-exit-requires-note |
| INV-197 | ok | ok | reopen-is-the-terminal-to-open-path-clears-dup |
| INV-198 | ok | ok | park-unpark-defers-keeps-status-auto-unpark |
| INV-199 | ok | ok | edit-signature-fields-old-new-logged |
| INV-200 | ok | ok | edit-never-changes-status |
| INV-327 | ok | ok | write-verb-refuses-control-characters: a title with a control byte is refused before the DB |
| INV-330 | ok | ok | status-flip-diffs-as-two-lines (dump diff after set-status) |
| INV-331 | ok | ok | create-bumps-seq-header-plus-entity-line |
| INV-344 | ok | ok | write-verb-commit-then-render-temp-rename-then-sidecar |
| INV-345 | ok | ok | crash-between-steps-leaves-stale-derived-next-write-heals |
| INV-348 | ok | ok — the state-trails-git-by-one-commit property is observable at a write | ok | dump-refreshed-by-last-write-rides-next-commit |
| INV-352 | ok | ok | sidecar-mismatch-regenerates-in-memory-first |
| INV-353 | ok | ok | dump-matches-sidecar-write-proceeds |
| INV-355 | ok | ok | tracked-dump-changed-under-us-refuses-naming-load |
| INV-356 | ok | ok | missing-sidecar-treated-as-divergent |
| INV-359 | ok — the write-verb tail writes the sidecar; init's write re-proves at S8a | ok | write-verb-tail-writes-sidecar |
| INV-487 | ok — this IS the DoD's §14 no-identity-leak fixture | ok | verbs-run-under-sentinel-home-user-outputs-clean: no hostname/username/absolute path in any verb's output |
| INV-546 | ok | ok | write-verb-ends-with-pragma-optimize |

## Architectural note for the close review

S5a builds the first write pipeline and the first read verbs, so it
creates the shared write-verb harness (gate stub → divergence core →
EXCLUSIVE tx → render → sidecar → optimize) that every later write verb
(S5b, S6) reuses. Two seams to check: the §8.4 **core** here must be
exactly match/heal/refuse/missing-sidecar — NOT the full matrix
(migration-crash residue, the `user_version`-ahead branch, prime's
read-only report), which is S8b's; and the version gate stays the S2 stub
(no migration escalation — that is S11), so a read verb's "runs the gate
first" is the stub call, observably present, not yet doing work.
