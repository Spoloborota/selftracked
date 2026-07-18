# Schema migrations for a dump-as-source-of-truth SQLite store

**Date:** 2026-07-18 · **Status:** research complete, recommendation at the end
**Question:** when a selftracked release changes the schema, how do users'
existing databases *and tracked dumps* get migrated, and how is that automated?
Owner asked specifically about goose.

**Framing that drives everything below:** in selftracked the tracked artifact
is the **dump**, not the DB (spec §3, §8; decision D4). The DB is a local,
gitignored, rebuildable derivative; `load` already rebuilds it from the dump
via a whitelist parser, and R1 requires dump ⇄ DB byte-roundtrip. So
"migration" is not the server-fleet problem migration tools solve (mutate one
long-lived precious DB in place). It is: *carry the pair (dump, DB) from
schema vK to vN, keeping the dump loadable by every newer binary forever.*
Three candidate mechanisms were analyzed: (a) migrate the live DB in place,
then re-dump; (b) hydrate the old dump under the old schema, transform, rebuild
a fresh new-schema DB, re-serialize with the new serializer; (c) transform the
dump textually. The recommendation is (b) with the live DB as an alternate
hydration source — see "Recommended v0 architecture".

---

## 1. Go migration tooling today

### 1.1 pressly/goose

- Current release **v3.27.2** (2026-06-30); v3.27.0 raised minimum Go to 1.25
  and simplified SQL migration templates.
  Evidence: https://github.com/pressly/goose/releases ·
  https://pkg.go.dev/github.com/pressly/goose/v3
- Embedded migrations: yes — `embed.FS` via `goose.SetBaseFS(...)` (apply-only;
  `create`/`fix` stay filesystem-based), and the newer
  `goose.NewProvider(dialect, db, fsys, opts...)` API takes any `fs.FS`.
  Evidence: https://github.com/pressly/goose
- Go-function migrations: `goose.AddMigrationContext(up, down)` receiving
  `*sql.Tx`, plus `NoTxContext` variants for statements that must run outside a
  transaction.
- SQLite: `DialectSQLite3` is supported and the README lists both the CGO
  `sqlite3` and the pure-Go `modernc` driver as supported — no CGO obstacle.
  Evidence: https://github.com/pressly/goose ·
  https://pkg.go.dev/github.com/pressly/goose/v3
- State tracking: an extra table (default `goose_db_version`). v3's Provider
  can replace it with a custom `database.Store` (`goose.WithStore(...)` +
  `DialectCustom`) — so a `user_version`-backed store is *possible* but is
  swimming against the tool.
  Evidence: https://pkg.go.dev/github.com/pressly/goose/v3
- No automatic run-at-open: the embedding application must call
  `goose.Up(...)` / `provider.Up(ctx)` itself, so "auto-migrate on open" is
  application code either way — goose only contributes file discovery,
  ordering, and the bookkeeping table.
- SQLite-specific friction: goose wraps each migration in a transaction unless
  `-- +goose NO TRANSACTION`; `PRAGMA foreign_keys` is a no-op inside a
  transaction, so any table-rebuild migration that needs `foreign_keys=OFF`
  must opt out of goose's transaction management — precisely the case where a
  migration tool is supposed to help.
  Evidence: https://www.sqlite.org/pragma.html#pragma_foreign_keys
  (foreign_keys is a no-op inside a transaction) ·
  https://github.com/pressly/goose/issues/728 (NO TRANSACTION failures do not
  even mark the DB dirty)

**Fit verdict:** poor, for structural reasons, not quality reasons. (i) The
`goose_db_version` table lands in `sqlite_schema`, and §8.5 requires the
dump's DDL block to byte-equal the compiled-in canonical DDL — so the tracking
table must either enter the canonical schema of every version (noise in the
security-critical surface) or be special-cased out of the serializer *and*
the whitelist (a second source of truth about what the schema is). (ii) Goose
migrates a *database*; it has no concept of "hydrate an old dump with the old
parser and re-serialize" — the dump side, which is the actual tracked
artifact, remains 100 % hand-written. (iii) The features goose adds (down
migrations, out-of-order `WithAllowMissing`, multiple environments, CLI) are
server-fleet features; a single-writer local CLI with a linear version chain
uses none of them. What remains of goose after subtracting the misfits is a
sorted slice of migration functions — ~30 lines of hand-rolled code keyed on
`PRAGMA user_version`.

### 1.2 golang-migrate/migrate

Current **v4.19.1** (2025-11-29), active. Its `database/sqlite` driver *is*
`modernc.org/sqlite` (pure Go, CGO-free), tracks state in a
`schema_migrations` table with a **dirty flag** that requires manual
intervention after a failed migration, wraps each migration in an implicit
transaction (`x-no-tx-wrap` to opt out — same PRAGMA problem as goose), and
supports `iofs`/embed sources.
Evidence: https://github.com/golang-migrate/migrate ·
https://github.com/golang-migrate/migrate/tree/master/database/sqlite
Same structural objections as goose (foreign table in the schema, DB-only
worldview), plus the dirty-flag model is user-hostile for an end-user tool
(a failed migration leaves a state only a developer understands).

### 1.3 Atlas (ariga)

Declarative diff-based engine, SQLite supported, but built around an external
CLI binary, HCL/registry workflows, and a freemium cloud component; migration
generation at *development* time is its strength, embedded auto-apply inside a
shipped end-user binary is not its shape. Useful at dev time to *draft* the
SQL of a rebuild, not as a runtime dependency.
Evidence: https://atlasgo.io/versioned/intro

### 1.4 Hand-rolled `PRAGMA user_version` switch

`user_version` is a 32-bit integer in the DB header that "SQLite makes no use
of" — reserved for exactly this purpose; `application_id` identifies the file
type (registered in SQLite's `magic.txt`).
Evidence: https://www.sqlite.org/pragma.html#pragma_user_version ·
https://www.sqlite.org/pragma.html#pragma_application_id
The spec already reserves both (§3.1) and mirrors the schema version in
`meta`. A linear `for v := userVersion; v < target; v++ { steps[v](...) }`
inside the already-held EXCLUSIVE write lock is the standard embedded-app
pattern and adds zero schema footprint — the dump stays exactly the entity
tables. This is what fossil, and most SQLite-as-application-file-format
programs, effectively do.

---

## 2. SQLite-specific migration constraints

- `ALTER TABLE` is deliberately limited: rename table/column, add column
  (restricted), drop column (restricted), and (3.53.0+) set/drop NOT NULL.
  No type changes, no PK/UNIQUE changes, no column reordering, **no
  STRICT ⇄ non-STRICT conversion**.
  Evidence: https://sqlite.org/lang_altertable.html
- Everything else requires the documented **12-step table-rebuild recipe**:
  `PRAGMA foreign_keys=OFF` (outside any transaction — it is a no-op inside
  one), begin transaction, save associated indexes/triggers/views from
  `sqlite_schema`, create `new_X`, `INSERT INTO new_X SELECT ... FROM X`,
  drop `X`, rename, recreate indexes/**triggers**/views, `PRAGMA
  foreign_key_check`, commit, re-enable FKs. Triggers and views touching the
  table are dropped by the rebuild and must be recreated from source — for
  selftracked that means every gate trigger of §5 is re-issued from the
  canonical DDL, never reconstructed ad hoc.
  Evidence: https://sqlite.org/lang_altertable.html
- STRICT tables: file format identical, but a DB containing STRICT tables is
  readable only by SQLite ≥ 3.37.0 (irrelevant for modernc, which tracks
  current SQLite); `integrity_check`/`quick_check` verify STRICT column types
  — Stage-0 verify after migration therefore also validates the migrated data
  against STRICT.
  Evidence: https://www.sqlite.org/stricttables.html
- Consequence drawn below: because selftracked's DB is *rebuildable*, the
  12-step in-place recipe can be sidestepped entirely by building a fresh DB
  from the new canonical DDL and inserting transformed rows in dump order —
  the same code path `load` already exercises. In-place ALTER becomes an
  optimization we do not need at v0.

---

## 3. How comparable local-first SQLite tools ship schema upgrades

- **fossil-scm — the directly relevant model.** Fossil splits its repository
  DB into a small "persistent" schema (the artifact store) and an "auxiliary"
  schema of derived tables; the auxiliary schema "changes from time to time as
  the implementation is enhanced, and the content is recomputed from the
  unchanging bag of artifacts" by `fossil rebuild`, which users run after
  upgrading the binary. Reverting to an older binary = run `rebuild` again
  (auxiliary tables are dropped and rebuilt). The one time fossil ALTERed a
  *persistent* table in place it was documented as "The Irreversible Schema
  Change" — evidence of how much they avoid in-place mutation of the durable
  layer. Some minor changes apply automatically on first write.
  Evidence: https://fossil-scm.org/home/doc/tip/www/tech_overview.wiki ·
  https://fossil-scm.org/home/help/rebuild ·
  https://fossil-scm.org/home/event/a1f9f17b6
  Mapping to selftracked: dump = fossil's durable artifact layer; DB = the
  rebuildable cache; a schema release = "rebuild the cache under the new
  schema", except selftracked must additionally re-serialize the durable
  layer itself (fossil's artifact format never changes; our dump format may).
- **fossil file-format promise** — the durable format is designed to outlive
  the storage schema: "readable … by people not yet born". That is the
  standard the dump should aim at: any dump ever committed to a repo's history
  must load in every future binary.
  Evidence: https://fossil-scm.org/home/doc/trunk/www/fileformat.wiki
- **beads (steveyegge/beads) — the cautionary tale.** Same architecture as
  selftracked (SQLite cache + tracked JSONL export as the git artifact). Its
  schema was refactored ~29 times; the 1.0 transition could not read older
  layouts at all, and the recovery path the community documented is exactly
  mechanism (b): *export old store to the interchange format → init fresh →
  import*. Non-automatic migrations produced broken installs and a cottage
  industry of migration gists.
  Evidence: https://github.com/Dicklesworthstone/beads_rust ·
  https://gist.github.com/leonletto/606e8afbb3603870d14b4123707416a2 ·
  https://github.com/steveyegge/beads/issues/534
  Lessons: (i) the interchange file, not the DB, is the migration medium that
  actually works; (ii) auto-migration must exist from v1, retrofitting it
  after divergence is what hurt them.
- **jujutsu (jj):** pre-1.0 on-disk format changes come with "transparent
  upgrades or upgrade commands"; the docs explicitly budget for "messages
  printed during automatic upgrades of the repo format" — auto-upgrade at
  open with a notice is the norm there.
  Evidence: https://github.com/jj-vcs/jj ·
  https://docs.jj-vcs.dev/latest/technical/architecture/
- **sqlite-utils / Datasette ecosystem:** sqlite-utils 4.0 (2026-07-07) built
  a migrations system into the library (formerly the `sqlite-migrate` plugin),
  tracking applied named steps in a `_sqlite_migrations` table — applied
  explicitly via `sqlite-utils migrate`, not at open. Python, table-based
  tracking; pattern-relevant only.
  Evidence: https://simonwillison.net/2026/Jul/7/sqlite-utils-4/ ·
  https://github.com/simonw/sqlite-migrate
- **litestream-adjacent tooling** is replication of DB pages, schema-agnostic
  — orthogonal to this question.

---

## 4. The dump-as-source-of-truth angle: versioned interchange formats

- **pg_dump's contract** is the canonical precedent: "the output of pg_dump
  can be expected to load into PostgreSQL server versions newer than pg_dump's
  version"; pg_dump "cannot dump from servers newer than its own major
  version; it will refuse to even try"; loading a newer dump into an older
  server is explicitly not guaranteed. I.e. dumps travel **forward only**, and
  refusal (not best-effort) guards the other direction.
  Evidence: https://www.postgresql.org/docs/current/app-pgdump.html
- **git repository-format versioning:** an implementation that does not
  understand `core.repositoryformatversion` "MUST NOT operate on that
  repository"; under v1, one that sees any unknown `extensions.*` key (such
  as `objectFormat=sha256`) "MUST NOT proceed".
  Versions are bumped rarely; individual data files carry their own versions;
  features degrade gracefully where possible.
  Evidence: https://git-scm.com/docs/repository-version
- **fossil**: durable-format stability + rebuildable cache (see §3).

**Dump contract derived from these precedents:**

1. The dump self-identifies its schema version *before* any data — but not
   via the `meta` table's position: the full DDL block precedes all data, so
   the `meta.schema_version` row sits well past the top of the file. The
   spec (§8.1) therefore makes the dump's single header comment line carry
   `schema_version` **normatively**: the loader reads it first and selects
   the version's canonical DDL (byte-equality check) and whitelist from it.
   The `meta.schema_version` row must agree with the header; a mismatch is a
   refusal, never a silent preference of one carrier. (The header comment is
   thus normative, not redundant decoration for human readers.)
2. `application_id` in the DB header identifies the file as selftracked's;
   `user_version` = schema version, mirrored in `meta.schema_version` (spec
   §3.1 already reserves both). The dump has "no PRAGMAs ever" (§8.1), so
   the in-dump carriers are the header comment and the `meta` row (which
   must agree); on load, `user_version` and `application_id` are stamped
   from `meta.schema_version`, and the events high-water mark is seeded.
3. **Forward-only promise (pg_dump rule):** every binary ≥ vN loads every dump
   with schema_version ≤ N, forever. This requires the binary to embed, per
   historical version k: the canonical DDL of k (byte-equal check of §8.5 is
   already version-parameterized: "for the dump's schema version") and the
   table/column whitelist of k (derivable from that DDL). The serializer
   *grammar* (INSERT with explicit column list, literal tokens only) is
   version-invariant by design; only the table set and column lists vary —
   so "old parser" = same parser, version-k whitelist.
4. **Refusal rule (git/pg_dump rule):** `schema_version > N` ⇒ hard exit 2,
   "this dump requires a newer selftracked" — never best-effort, never
   partial load.

---

## 5. Downgrade / forward-compat policy and testing patterns

- **Refuse-older-binary is the defensible policy.** git refuses unknown
  versions/extensions outright; pg_dump refuses to even read newer servers;
  fossil tolerates binary downgrade only because its durable format never
  changes (auxiliary tables are just rebuilt). selftracked's durable format
  *is* the versioned artifact, so the git/pg_dump stance applies: an older
  binary meeting a newer dump (typically after `git pull`) refuses with the
  upgrade instruction. Best-effort reading would break the byte-equal DDL
  gate and R1 anyway — an old binary re-dumping "what it understood" of a
  newer dump would silently destroy data on the next commit. A
  git-`extensions`-style marker for *additive, ignorable* features can be
  introduced later if ever needed; not v0.
- **Never migrate downward.** No down-migrations are shipped (goose/migrate
  down-migrations are a server-ops affordance). The user's escape hatch is
  git itself: check out an older dump, run the older binary. History already
  stores every prior state — the VCS is the undo.
- **Testing pattern: golden old-dump corpus.** Freeze one representative dump
  (plus red/tampered fixtures) per historical schema version into testdata at
  the moment that version ships. CI, per corpus entry: load with the head
  binary → migration chain runs → full `verify` (Stage 0 + R1–R15) green →
  re-serialized dump is byte-deterministic across the OS matrix (§16 already
  runs one). Plus a commutativity property test: hydrating a vK dump and
  migrating must byte-equal migrating a live vK DB and re-dumping (the two
  hydration sources of the same chain; asserted in CI at
  `events_archived_through = 0`). Plus a refusal test: a synthetic
  dump with `schema_version = N+1` must exit 2 without touching the DB. This
  mirrors pg_dump's cross-version buildfarm practice and fossil's
  "reconstructible forever" claim, executed as CI instead of promise.

---

## Recommended v0 migration architecture

**Mechanism — versioned rebuild, not in-place ALTER (the fossil model, with a
re-serialized durable layer):**

- The binary compiles in, per schema version k ∈ 1..N: canonical DDL(k),
  whitelist(k) (tables + column lists, derived from DDL(k)), and a pure
  transform `T_k: rows(k) → rows(k+1)` written in Go (rename/derive/backfill
  columns, split tables, synthesize events rows for the migration itself if
  the audit trail warrants it).
- One migration engine, two hydration sources:
  1. **Old live DB** (normal upgrade: user installs new release, runs any
     verb): read rows out of the vK DB.
  2. **Old dump** (`load` after a pull, fresh clone, or missing DB): parse
     with whitelist(k) against DDL(k)'s byte-equal check — the existing §8.5
     loader parameterized by version.
- Apply `T_k .. T_{N-1}` to the row sets, then **build a fresh DB from
  DDL(N)** in a temp file — inserting in the fixed table order, rows in PK
  order, exactly as `load` already does (INSERT paths are open by design,
  §5's gates bind UPDATE/DELETE; AUTOINCREMENT high-water marks self-set from
  explicit ids, §8.1). Set `user_version = N`, update `meta.schema_version`.
  Run Stage-0 verify + full verify. Atomically rename the temp DB into
  place, then regenerate dump + STATE.md with the vN serializer, then update
  the sidecar hash. This *extends* the §8.3 crash-safe sequence rather than
  leaving it unchanged: a temp-DB build plus atomic DB rename now precede
  the dump render. **Corrected after round-5 verification:** crash-residue
  recognition is specified spec-side via the **sidecar**, not a bare
  version comparison. Two cases: (1) crash between the migration's DB swap
  and the re-dump leaves the tracked dump still hash-matching the sidecar,
  while the DB's `user_version` is already ahead of the dump's header
  version — this is unambiguous residue and heals by re-dump. (2) if the
  tracked dump does *not* match the sidecar (an external change happened —
  e.g. a `git pull` landed a newer dump, including the crash-plus-pull
  combination), the migration completes DB-side only, does **not** overwrite
  the dump, and surfaces the divergence instead of guessing: write verbs
  refuse, and `prime` flags it. No 12-step recipe, no `foreign_keys=OFF`
  window, no trigger drop/recreate hazard: the new schema including all gate
  triggers comes verbatim from DDL(N).

**When it runs — automatic, at open, inside the lock:**

- **The version gate is two ordered comparisons, not one.** (i) tracked dump
  header `schema_version` vs the binary's N — forward-only refusal: if the
  dump's header claims a version newer than the binary knows, exit 2 before
  anything else runs. (ii) only once (i) passes: DB `user_version` vs N —
  this is the migrate-or-proceed decision. For `load` on a fresh clone
  (no DB yet), the same ordering applies with rebuild standing in for
  migrate: (i) then rebuild the DB from the dump at its declared version and
  migrate forward to N.
- Every verb, immediately after acquiring the EXCLUSIVE write lock (read
  verbs escalate to the write path for the migration only — spec §8.6
  adopted this normatively), having passed gate (i), compares `user_version`
  to N: `< N` ⇒ migrate now, print one notice line **to stderr** (`migrated
  schema v3 → v5`) — always stderr, never the stdout JSON surface; `prime`
  additionally reports `"migrated"` in its JSON — then proceed with the verb;
  `> N` ⇒ exit 2 with the upgrade message. `load` performs the same two-gate
  check on the dump's `meta.schema_version` before hydrating. No end-user
  `migrate` command exists (jj's "transparent upgrade with a printed
  notice"); the migrated dump rides into the next commit through the
  existing pre-commit dump-refresh flow — the "state trails git by ≤ 1
  commit" rule (§8.3) already covers it, and the dump diff of a migration is
  itself reviewable in git, which is the whole point of the dump.
- **Escalation race:** two same-machine read verbs can race the migration
  escalation (both see `user_version < N` and both escalate to the write
  path). The loser, on `BUSY`, re-checks `user_version` after the wait —
  by then the winner has migrated — and proceeds read-only against the
  already-migrated DB. A `BUSY` that survives `busy_timeout` (i.e. neither
  side backs off cleanly) exits 2 with an "another process may be
  migrating — retry" hint, rather than a generic busy error.

**How dumps version:** the dump's header comment line carries
`schema_version` normatively (the full DDL block precedes all data, so the
`meta` row is not near the top of the file; the loader keys DDL/whitelist
selection on the header); `meta.schema_version` must agree with it
(mismatch ⇒ refusal), and `user_version`/`application_id` are its DB header
mirrors; the forward-only + refusal contract of §4 above is documented in
the dump header comment and the migration guide.

**Required-rows check on `load`:** the grammar whitelist alone is not
sufficient — a dump missing the `meta.schema_version` or
`meta.events_archived_through` row still parses cleanly against the
whitelist (a missing row is not a grammar violation). `load` therefore
additionally **refuses** a dump whose `meta` table lacks either row, as an
explicit required-rows check layered on top of the whitelist parse, not
folded into it.

**CI:** golden dump corpus per historical version (with tampered red
fixtures) → head binary loads, migrates, full-verifies, re-dumps
byte-deterministically across the OS matrix; commutativity property
(DB-path ≡ dump-path, asserted at `events_archived_through = 0`); N+1
refusal test; the §8.5 fuzzer extended to run against every historical
whitelist.

**Cost accepted:** the binary carries all historical DDLs, whitelists, and
transforms forever. This is deliberate — it is the price of the pg_dump/fossil
promise, it is small (text + pure functions), and the golden corpus keeps it
honest.

---

## Rejected alternatives

- **goose (pressly/goose)** — rejected as the mechanism, with respect: it is
  the best of the migration tools for embedded Go use (embed.FS, Go
  migrations, Provider API, modernc-compatible). But its `goose_db_version`
  table pollutes `sqlite_schema`, which §8.5's byte-equal DDL gate and the
  serializer's fixed table set treat as canonical — every schema version
  would carry tool bookkeeping, or serializer + whitelist grow special cases
  (`WithStore`/`DialectCustom` could redirect tracking to `user_version`, but
  then goose contributes only a sorted slice of steps — the part that is
  trivial to own). Decisively: goose migrates only the DB; the dump — the
  actual tracked artifact and the security boundary — would still need the
  entire versioned-whitelist/re-serialize machinery hand-built around it. Its
  transaction management also inverts on SQLite exactly when needed most
  (`foreign_keys=OFF` requires `NO TRANSACTION`), and the fresh-rebuild
  design removes the need for in-place DDL sequencing altogether.
- **golang-migrate** — same structural objections plus the dirty-flag failure
  mode, which strands end users in a state only operators understand.
- **Atlas** — external binary + HCL/registry/cloud workflow; wrong shape for
  an embedded auto-run path. Possibly useful at development time to draft
  rebuild SQL; never a runtime dependency.
- **(a) In-place ALTER of the live DB, then re-dump — as the primary
  mechanism** — rejected: limited ALTER TABLE forces the 12-step recipe for
  most real changes; that recipe requires `foreign_keys=OFF` outside a
  transaction and drop/recreate of the gate triggers — a window where the
  schema's own enforcement is down; and it still needs the dump-side
  versioned loader anyway (fresh clones have no DB to ALTER). The fresh
  rebuild gets the same result through the already-hardened `load` path.
- **(c) Textual dump-to-dump transformation** — rejected: it re-implements
  the migration in a second medium with no SQL engine underneath, so CHECKs,
  FKs, STRICT types, and triggers validate nothing during the transform; and
  its output must byte-equal what the vN serializer would emit — every
  transform is a chance to violate R1. The dump is an interchange format, not
  an editing surface (pg_dump's "manual editing of the dump file" is its
  documented failure mode, not its design).
- **Best-effort forward compatibility (older binary reads newer dump)** —
  rejected per git's refuse-on-unknown-format rule ("MUST NOT operate" /
  "MUST NOT proceed") and pg_dump's refusal; an old
  binary's next re-dump would silently drop everything it did not understand.
- **A separate end-user `migrate` command instead of auto-run** — rejected:
  beads demonstrates where non-automatic migration of a two-representation
  store ends (broken installs, community-written recovery gists); fossil's
  manual `rebuild` works only because forgetting it fails loudly and loses
  nothing durable. A single-writer CLI holding an exclusive lock can migrate
  at open with zero coordination cost; jj sets the UX precedent (automatic,
  with a printed notice).
