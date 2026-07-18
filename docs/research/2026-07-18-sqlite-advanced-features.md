# SQLite advanced features × selftracked — adoption research

Status: EXECUTED → v0-spec.md §3.1/§5 (adopted into the spec). Method:
sqlite.org primary documentation per feature (URLs inline), empirical checks
against a local sqlite3 3.51.0 CLI, and driver-support verification against
modernc.org/sqlite v1.54.0 source (bundles SQLite 3.53.3). Two driver facts
frame everything: DSN `_pragma=` parameters apply to **every pooled
connection** (closing the classic per-connection PRAGMA pitfall), and the
driver exposes **no raw sqlite3 handle** (anything not reachable via
SQL/PRAGMA/DSN or an explicitly wrapped API is unreachable).

## GOLD — adopted

1. **Triggers + RAISE(ABORT) as in-schema gates** (lang_createtrigger.html).
   Verified: append-only tables (BEFORE UPDATE/DELETE → RAISE), status
   transition matrices in a WHEN clause (since 3.47 the RAISE message can be
   an expression naming the transition), seq-contiguity checks, and a
   meta-flag pattern gating a column to one verb (main-table flag; main
   triggers cannot reference temp objects — verified error). Found hole:
   `INSERT OR REPLACE` bypasses DELETE triggers unless
   `recursive_triggers=ON` — that pragma is mandatory. Triggers ride in dump
   DDL, binding any process that opens the DB.
2. **WIP limit as a partial UNIQUE index** (partialindex.html):
   `CREATE UNIQUE INDEX … ON stories(epic) WHERE status='IN-PROGRESS'` —
   verified to refuse a second in-progress row per epic at the engine level.
3. **Cross-column CHECKs** for status⇔field invariants (lang_createtable.html):
   `(status='DUPLICATE') = (dup_of IS NOT NULL)` etc. Subqueries are
   prohibited in CHECK (verified) — cross-row rules stay in triggers/verify.
4. **STRICT tables** (stricttables.html, 3.37+): type gate for a DB rebuilt
   from untrusted dumps; `integrity_check` then also validates types. Nuance:
   STRICT still performs lossless coercion — the canonical serializer owns
   literal forms.
5. **Serialize/Deserialize wrapped by the driver** (c3ref/serialize.html;
   verified in driver source): the dump↔DB roundtrip gate can run entirely
   in `:memory:` — no temp files, no locks.
6. **`PRAGMA locking_mode=EXCLUSIVE`** (pragma.html#pragma_locking_mode): the
   OS-level exclusive lock IS the single-writer mechanism — holds against any
   process including a stray sqlite3 shell; replaces a hand-rolled lock file.
   Driver sets no busy_timeout by default (verified 0) — set it in the DSN.
7. **AUTOINCREMENT is mandatory for never-reused ids** (autoinc.html):
   verified that plain INTEGER PRIMARY KEY reuses a deleted max id; with
   AUTOINCREMENT ids are guaranteed never reused. Refined after adversarial
   testing: `sqlite_sequence` must NOT be serialized as INSERT rows — the
   table has no PK on `name`, and loading explicit-id data auto-creates its
   rows, so dumped rows duplicate (verified; breaks the roundtrip gate).
   Since explicit-id inserts set the high-water marks automatically, the dump
   carries no sequence data at all; a live-DB sanity rule checks
   `sqlite_sequence ≥ MAX(id)` instead.
8. **Composite FK `(epic,story) → stories(epic,id)`** works (foreignkeys.html);
   FK enforcement is per-connection and off by default — solved by the DSN
   pragma. `integrity_check` does NOT catch FK violations (verified) —
   `verify` must run `PRAGMA foreign_key_check` as well.
9. **Own dump serializer is mandatory; `.dump` is disqualified** (cli.html):
   `.dump` emits no column lists, includes sqlite_sequence rows, dumps FTS5
   shadow tables as opaque version-dependent blobs, and 3.50.0 changed its
   output format (`unistr()`) between adjacent releases; no stability
   contract exists. The serializer must own table order, explicit ORDER BY
   full PK, explicit column lists, one escaping rule, shadow-table exclusion.
10. **RETURNING / UPSERT** (lang_returning.html, lang_upsert.html): fits
    `create → #NN`; driver floor v1.48.2 (earlier versions lost RETURNING
    rows through Exec — upstream fix 2026-04); use Query/QueryRow only.
    Upserts do not bypass CHECK/trigger gates (documented) — good.
11. **Untrusted-dump defense posture** (security.html): reachable subset —
    `trusted_schema=OFF`, `cell_size_check=ON`, `query_only` on read verbs,
    `mmap_size=0`, capped limits via the driver's `Limit()`. NOT reachable:
    `SQLITE_DBCONFIG_DEFENSIVE` (no raw handle) — the primary defense is a
    whitelist loader that executes only serializer-shaped statements.
12. **Housekeeping**: `PRAGMA user_version`/`application_id` for schema
    versioning; `PRAGMA optimize` at write-verb teardown (official guidance
    for short-lived connections); temp-file + atomic-rename rebuild flow
    (VACUUM INTO's own docs warn an interrupted run leaves a corrupt target);
    extended result codes map cleanly to the CLI exit-code taxonomy
    (constraint family → "refusal", busy/corrupt → "error") (rescode.html).
13. **Views shipped in the schema** (lang_createview.html): the ready
    frontier and backlog as CREATE VIEW — every consumer sees one read model.

## Rejected (with reasons)

- **Session extension/changesets**: compiled into the driver's C core but no
  Go API is reachable from database/sql; changesets are binary (not a
  reviewable surface). The dump diff already is our changeset.
- **WAL mode**: its benefits require concurrency excluded by axiom; costs are
  real (persistent mode flag, `-wal`/`-shm` litter, no network-FS support).
  Rollback journal + `synchronous=FULL` fits a short-lived single-writer CLI.
- **JSONB storage**: officially "intended for internal use by SQLite only",
  no cross-version format guarantee, and BLOB hex would kill dump diffs.
  TEXT JSON + `json_valid()` CHECK where structure is needed (json1.html).
- **sqldiff as a runtime dependency**: C binary (breaks pure-Go shipping);
  weaker than the byte-compare pair gate.
- **STORED generated columns** (would enter the dump; VIRTUAL is free).
- **FTS5 in v0** (feasible pure-Go — defer the `search` verb; the serializer
  excludes shadow tables from day one so adding it later cannot break dumps).
- sqlar, `.recover`, dbstat/dbpage: no problem of ours they solve.

## Open questions for the implementation phase (must be tested in Go)

Serialize/Deserialize roundtrip with the full schema (triggers + STRICT +
WITHOUT ROWID + partial indexes); VACUUM INTO under the driver; whether the
driver's error type returns extended or primary result codes; the
`recursive_triggers` + `INSERT OR REPLACE` regression under the driver;
RETURNING via Query on ≥ v1.48.2; cross-OS byte-determinism of the
serializer output (the pair gate's foundation — proven, not assumed).
Driver operational note: pin `modernc.org/libc` to the driver's go.mod
version.
