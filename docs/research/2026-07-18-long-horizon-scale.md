# Long-horizon scale: what selftracked looks like after years of use

Date: 2026-07-18. Status: research input for v0 spec (§15 D7, §17 deferrals).
Question examined: what does the system look like at 100–1000 epic/task/workdir
scale and years of operation — DB size, dump size, verb/pre-commit latency, git
repo growth, filesystem growth — and is trimming/archiving/rotation adequately
designed?

Method: (a) first-principles growth math on the actual §5 DDL, validated by
synthetic databases built with the real schema and a faithful Python
implementation of the §8.1 serializer contract (fixed table order, full-PK
ordering, explicit column lists, quote-doubling); (b) git experiments committing
the generated dumps with realistic ~4-line per-commit diffs; (c) published
evidence from comparable tools. Every number below is marked **[measured]**
(observed in the sandbox: Apple-silicon laptop, SSD, SQLite 3.51, git 2.54,
Python 3.14) or **[estimated]** (extrapolation/model). Python serialization
times are an upper bound for the Go implementation; the `sqlite3` CLI `.dump`
time is given as the C-speed reference — Go will land between them, nearer C.

---

## 1. Growth model

### 1.1 Scenarios

| Scenario | Epics | Stories | Tasks | Worklog | Events | Artifacts+links |
|---|---|---|---|---|---|---|
| A — 1 year, solo crew | 25 | 110 | 500 | 3,000 | 20,000 | ~1,200 |
| B — 5 years | 120 | 600 | 5,000 | 30,000 | 200,000 | ~13,700 |
| C — pathological | 1,000 | 5,000 | 10,000 | 60,000 | 500,000 | ~24,000 |

Content realism: worklog notes ~180 chars (episode prose), event details
~70 chars, task titles ~45 chars. A "lean" variant of B (event details
~30 chars, worklog notes ~80 chars) brackets the low end. Spec revs 3.6-3.8
add mandatory prose to `events.detail`/`tasks.status_note` (IN-REVIEW exit
notes, `story unblock --resolution`) and define `edit` events as carrying
changed-field old→new values with BOTH values bounded to short prefixes
(spec §5.9 — the bound exists precisely because unbounded verbatim copies
would break these calibrations). Ratification prose affects only
the small fraction of events that are ratification moments and its size
class (a quoted verdict, on the order of a worklog note) sits [estimated]
inside the ~2× content-size margin the B vs B-lean bracket above already
carries; bounded edit-event details stay within the ~70-char average's
regime. The one-off per-epic import source map (spec §6.2/§10) is a
single-row outlier proportional to imported worklog rows — low-KB at
scenario B's densities, an aggregate rounding error [estimated]. The
growth model was not re-run for any of these.

### 1.2 Results [measured]

| Metric | A (1 yr) | B (5 yr) | B lean | C (pathological) |
|---|---|---|---|---|
| DB file | 3.9 MB | 38.0 MB | 23.7 MB | 91.6 MB |
| dump.sql | 5.6 MB | 55.7 MB | 42.9 MB | 133.5 MB |
| dump.sql gzip (git-blob proxy) | 0.69 MB | 7.1 MB | 4.9 MB | 17.5 MB |
| Full dump serialization, Python | 37 ms | 361 ms | 334 ms | 883 ms |
| Full dump, `sqlite3 .dump` C reference | — | 275 ms (44 MB out) | — | — |
| `PRAGMA quick_check` | 41 ms | ~30–50 ms | — | ~75 ms |
| `PRAGMA integrity_check` | — | ~100 ms | — | — |
| `git add dump.sql` (zlib hash of full file) | 55 ms | 329 ms | — | — |

The quick_check/integrity_check figures for B and C are corrected after
adversarial re-measurement on same-size synthetic data — an earlier draft
overstated them ~6–10× (305 ms / 378 ms / 1,017 ms). The A-column 41 ms is
the original measurement, not re-measured; it likely carries the same
inflation (hence A apparently exceeding B here).

Text dump is ~1.4–1.5× the DB file (not 2–4×: SQLite's own page overhead and
the WITHOUT ROWID B-trees narrow the gap).

### 1.3 Where the bytes are: events dominate everything [measured]

Per-table dump bytes at scenario B:

| Table | Rows | Dump bytes | Avg line | Share |
|---|---|---|---|---|
| **events** | 200,000 | **40.4 MB** | 201 B | **72%** |
| worklog | 30,000 | 12.0 MB | 399 B | 22% |
| tasks | 5,000 | 1.5 MB | 291 B | 2.6% |
| artifacts + link tables | ~13,700 | 1.6 MB | ~110 B | 2.9% |
| epics/stories/criteria | ~1,100 | 0.26 MB | — | 0.5% |

60 bytes of every events line is the fixed
`INSERT INTO events (seq, at, entity, event, detail) VALUES (` prefix — even
with 30-char details, 200k events cost ~31 MB of dump text. **Everything about
long-horizon behavior reduces to one fact: `events` is ~72% of the dump and is
the only table with unbounded per-action growth** (every write verb appends at
least one row; worklog grows per episode, tasks per idea — both an order of
magnitude slower).

STATE.md is O(active) by construction (fixed sections + last 10 events): a few
KB at any age **[estimated]** — correctly windowed, no issue.

---

## 2. Hot spots verified

### 2.1 Full dump re-serialization on every write verb

Every write verb pays a full O(total rows) serialize + write + fsync + rename.
**[measured]** cost of the serialize step: 37 ms at year 1 → 275–360 ms at year
5 → ~0.9 s pathological. The transaction itself is sub-millisecond; **at year-5
scale the dump tail is >99% of write-verb latency**. The >200 ms "annoying
verb" line is crossed at roughly 100–150k events (~30 MB dump), i.e. year 3–4
of a solo crew, earlier for chatty crews **[estimated]** — and agents invoke
write verbs constantly.

### 2.2 Pre-commit latency

**Corrected after round-5 verification** (the prior draft's `--fast` shape was
wrong against the spec): the §9 hook runs `verify --fast` — which per §7 is
`quick_check` + `foreign_key_check` + the DB-only Stage-1 rules (R6–R9, R12)
+ R15, with **R1 skipped entirely** — then `dump` then `state` then `git add`.
There is no shared R1/dump pass to speak of: the hook's own `dump` step is
the single O(total rows) serialization pass, full stop; R1 (dump⇄DB
byte-roundtrip) simply does not run in `--fast`. Budget at Go≈C speed
**[estimated from measured parts; quick_check corrected after adversarial
re-measurement]**:

| Stage | A (1 yr) | B (5 yr) |
|---|---|---|
| quick_check + foreign_key_check | ~10 ms | ~30 ms |
| DB-only Stage-1 rule battery (R6–R9, R12) + R15 | negligible | ~25 ms (R6 1.7 ms + R7 0.04 ms + R8 ~20 ms with covering index + R9 <1 ms, aggregate; R12 and R15 sub-ms) |
| serialize + write (the hook's `dump` step; the only full-scan pass) | ~35 ms | ~345 ms |
| `state` | ~ms | ~ms |
| `git add` | 55 ms | 329 ms |
| **Total** | **~0.1 s** | **~0.7 s** |

The added rule battery's ~25 ms at scenario B does not move the year-5 total:
it was already inside the measured/estimated envelope that produced ~0.7 s,
so **the ~0.7 s year-5 total stands**. Year 1 is imperceptible; year 5 at
~0.7 s does **not** cross the 1 s annoyance line (an earlier draft's
~1.2–1.5 s figure double-counted a supposed R1 serialization pass — which
per the corrected `--fast` shape above never runs — and used the inflated
quick_check number); pathological is ~2 s **[estimated]**, dominated by the
~0.9 s serialize.

Adjacent, now-named cost: the post-commit dump-vs-sidecar compare (spec §8.4)
hashes the dump file with SHA-256 — **[estimated]** ~24 ms at 40 MB — cheap
relative to the hook budget above, not a new hot spot.

### 2.3 Git behavior on a multi-MB dump rewritten every commit

Experiment: 200 commits of the A dump (5.6 MB) and 100 commits of the B dump
(55.7 MB), each commit a realistic status-flip diff — 4 changed lines: 1
removed + 1 added status line, plus 2 appended event lines. **[measured]**

**Calibration note, corrected after round-5 verification:** this over-appends
relative to spec — a status flip writes exactly **one** events row (spec
§8.2: state row ±pair + one events line), not two. The measured commit shape
above appended events at ~2× the spec's actual per-flip rate. Since events
are the dominant dump-growth term (§1.3), every pack-growth extrapolation
below built on this experiment is therefore **conservative — an upper
bound**; the true per-commit and pack-growth numbers run somewhat lower. The
direction of every conclusion (delta compression collapses history; full
repack matters; GitHub's per-file threshold is approached) is unchanged.

- **Before any repack, every commit stores a full zlib blob of the whole
  dump**: 1.35 MB/commit at A-size, 9.2 MB/commit at B-size. This matches the
  Git Book: deltas exist only inside packfiles
  (https://git-scm.com/book/en/v2/Git-Internals-Packfiles). `git gc --auto`
  fires at ~6,700 loose objects ≈ every ~2,200 commits at ~3 objects/commit —
  so multi-GB loose-object accumulation between packs is the *default* state
  at year-5 dump sizes **[estimated]**.
- **After a full `git repack -adf`**, history collapses: 200 commits of the
  5.6 MB dump → **6.7 MB total pack**; with `--window=250 --depth=250`
  (aggressive-gc settings) → **0.76 MB total**. 100 commits of the 55.7 MB dump
  → 34 MB pack (14 s repack). Delta compression of near-identical SQL text is
  excellent, exactly as the Git Book's 9-byte-delta example promises.
- Caveat observed: plain `git gc` (git 2.54 defaults) once left 78
  *non-deltified* ~5 MB packs totaling 268 MB where `repack -adf` produced
  6.7 MB — an environment-specific figure (an independent re-run confirmed
  the direction, ~23× worse than a full repack; the mechanism is not
  established) — default incremental packing does not reliably find deltas
  between many large similar blobs; periodic full repack is what delivers
  the collapse.
- **Extrapolation model** **[estimated]**: default delta depth is 50
  (https://git-scm.com/docs/git-pack-objects), so packed history needs roughly
  one compressed fulltext per ≤50 versions plus tiny deltas:
  `pack ≈ (commits/50) × gzip(dump) + commits × ~0.3 KB`. At 10,000 commits:
  A-size dump → ~140–200 MB; B-size → ~1.4 GB (a few hundred MB with
  aggressive windows). GitHub recommends repos stay under 1 GB and *warns on
  any file over 50 MiB* (= 52.43 MB) — the unarchived year-5 dump straddles
  that line (modeled 52.0–55.7 MB depending on note verbosity), so it
  approaches and may cross the per-file warning threshold
  (https://docs.github.com/en/repositories/working-with-files/managing-large-files/about-large-files-on-github).
- Diff review stays fine at any size: `git diff` of adjacent commits on the
  56 MB dump renders the 4-line change in 122 ms **[measured]**. Unreviewable
  dumps are not the failure mode; repo mass and hook latency are.

### 2.4 Boundedness of prime / views / STATE.md

- `STATE.md`: bounded (fixed sections, last 10 events) — O(active). Good.
- `prime`: O(open work), not O(history) — `ready[]`, `triage[]`, `in_review[]`
  scale with the open backlog, `epics_active[]` with active epics. Correct
  asymptotics, but **no cap**: a neglected backlog with hundreds of OPEN tasks
  injects all of them into every session start (483 ready rows ≈ ~50 KB JSON
  **[estimated]**). A cap (first N + total count) is one line of spec.
- `v_backlog`/`list` with no filter is O(all tasks ever) — 5,000 rows in 3.4 ms
  **[measured]**; a context-noise concern for agents, not a perf one.
- `log <ref>` is a full events scan: 9.6 ms at 200k events **[measured]** —
  fine, but see R12.

### 2.5 SQLite query performance: a non-issue, with one implementation trap

All operational queries are trivial at any plausible scale **[measured at B]**:
v_ready 0.56 ms, `list --status` 0.45 ms, last-10-events 0.01 ms. SQLite's own
guidance places the comfort zone below ~1 TB with a 281 TB hard limit and
~2×10¹³ practical rows (https://www.sqlite.org/whentouse.html,
https://www.sqlite.org/limits.html); a 92 MB DB is 4+ orders of magnitude below
"consider client/server".

**The trap: R12 as a naive correlated subquery is quadratic** — terminal
entities × a full events scan per entity: **7.23 s** at scenario B
**[measured]**. The `'#'||t.id` concat is *not* the culprit (corrected after
adversarial re-measurement: with an `events(entity)` index present, EXPLAIN
shows a covering-index search through the concat, ~1.4 ms) — the slowness is
purely the absence of the index. With an `events(entity)` index R12 is
**4.7 ms**; the same index makes `log` 0.02 ms. Verdict: `verify` must be
implemented with an events-entity index or single-pass hash maps — a v0
implementation note, or R12 alone will dominate `verify` by year 3.

`load` at year-5 scale (parse 56 MB whitelist grammar + insert ~247k rows
including artifacts/links +
Stage-0 verify): low seconds **[estimated]** — acceptable for its frequency
(pulls, fresh clones), and the temp-build can relax `synchronous` safely since
it ends in an atomic rename.

---

## 3. Archiving precedent in comparable tools

- **Fossil/SQLite self-hosting** — the existence proof that "one SQLite file
  holds decades": fossil's repo after ~19 years: 20,849 check-ins, 67,504
  artifacts, **149 MB**; SQLite's repo after 26 years: 36,521 check-ins,
  **187 MB**, 70:1 internal delta compression
  (https://www.fossil-scm.org/home/stat, https://www.sqlite.org/src/stat).
  Fossil never archives — but it stores content *delta-compressed inside* the
  DB. selftracked's equivalent compressor is git's packfile layer over the
  dump; the live DB/dump themselves stay uncompressed, which is why events
  need an archive valve where fossil needs none.
- **beads** (the closest neighbor: git-synced agent-first issue tracker)
  converged on *three* size-reduction commands: `bd admin compact` (semantic
  compression of issues closed >30 days, "70% reduction", "permanent graceful
  decay"), `bd gc` (delete closed >90 days → squash history → engine GC), and
  history squashing because "auto-commit per mutation" grows the log
  (https://github.com/gastownhall/beads/blob/main/docs/cli-reference/admin.md,
  https://github.com/gastownhall/beads/blob/main/docs/cli-reference/gc.md).
  A purpose-built agent tracker found unbounded retention untenable early —
  but note it *discards* content, which selftracked's axiom 6 forbids; the
  selftracked answer must be *move*, never delete.
- **git-bug**: no archiving mechanism documented; first scale pain was memory
  (67 MB RSS at 700 issues, https://github.com/MichaelMure/git-bug/issues/132).
- **Jira practice**: archive issues that are done and untouched 12–24 months;
  Atlassian's measured wins from archiving 43% of issues: 28% faster JQL, 42%
  smaller index, 44% shorter re-index
  (https://www.atlassian.com/blog/enterprise/issue-archiving,
  https://confluence.atlassian.com/enterprise/too-many-issues-1402420938.html).
  The recurring pattern: **age-of-inactivity cutoff, reversible, metadata
  retained**.
- **Event-sourcing canon**: three standard mechanisms — snapshot + replay tail
  (https://martinfowler.com/eaaDev/EventSourcing.html), keyed compaction
  (Kafka log compaction: retain last value per key,
  https://kafka.apache.org/documentation/#compaction), and explicit scavenge
  (EventStoreDB rewrites chunks minus dead events, manual operation,
  https://docs.kurrent.io/server/v23.10/operations/scavenge). selftracked's
  dump *is already a snapshot* of entity state — the events table is a pure
  audit trail, so the snapshot+tail pattern maps exactly: entity tables = the
  snapshot, recent events = the tail, old events = archivable cold history.

### 3.1 Recommended D7 design (`events archive`)

The append-only, tombstone-free nature makes this unusually clean:

1. **Cutoff by seq/age**: `events archive --before DATE` (or `--keep N`) moves
   rows with `at < DATE` into archive, always preserving a recent tail.
2. **Archive = separate dump files, write-once**: move archived rows to
   `.selftracked/archive/events-<year>.sql` in the same serializer grammar,
   tracked in git but **never rewritten after creation**. Because events are
   immutable and globally seq-ordered, each archive file is frozen the moment
   it is written — git stores each exactly once, forever; the D7 merge
   objection to a *live* append file does not apply to frozen segments.
3. **Boundary in `meta`**: `events_archived_through = <seq>`. The live dump
   contains exactly the events with `seq > boundary`; determinism and R1 are
   unaffected. No tombstones needed — the boundary *is* the tombstone.
4. **In-DB semantics**: either delete-below-boundary under a sanctioned verb
   (a gated exception to no-delete, mirroring `epic close`'s `active_verb`
   pattern) or move to an `events_archive` table excluded from the live dump.
   The delete variant keeps the DB small (the point); the verb writes an
   `archive` events row above the boundary so the trail records its own
   truncation.
5. **Readers**: `log --all` and R8/R12 consult archive files on demand. The
   segments are in-repo but **not** parseable by the §8.5 load parser — they
   carry no DDL block, so that loader rejects them at step 1; the segments
   need their own versioned grammar and reader, which land with D7 (the spec
   §8.2 now says exactly this). Boundary-aware R12 semantics are likewise a
   D7 design obligation, not a free rescoping: "terminal since boundary" is
   not computable from the current schema (there is no became-terminal-at
   column). In v0 the boundary is enforced `= 0` by a new R9 clause (a
   non-zero boundary is a tamper signature), which also closes the
   forged-boundary truncation hole; `verify --deep` walking archives is D7
   scope.

Worklog is the second-order case (22% of dump, 10× slower growth): the same
frozen-segment trick applies per CLOSED epic later (only rare V-rows append
post-close); not needed before events archiving is.

---

## 4. work/ filesystem growth

At 5,000 tasks with per-task dated workdirs: ~5,000 directories. POSIX
filesystems handle this trivially; human/file-picker ergonomics degrade past a
few thousand entries in one directory **[estimated]** — date-sharded roots
(`work/2026/…`) are a one-row `paths move` away by design, no schema change.
The real cost is content: agent workdirs at 1–10 MB each (an uncited working
assumption, not a measurement) → **5–50 GB over five years if nothing is ever
cleaned** **[estimated]** — the filesystem, not the DB, is the dominant disk
consumer at horizon.

The schema already contains the enabling design: `workdir` and `run` classes
are `ephemeral=1`, existence-exempt, and ignored by `stale`/R3 — **deleting
ephemeral files breaks no invariant today**. What is missing is the verb and
the policy. Defensible v0.x shape: `gc --before DATE [--class C]` — delete
files under ephemeral-class roots older than the cutoff — excluding any
nested non-ephemeral-class root (e.g. `work/reports/` under `work/`), as the
spec's §17 now mandates — and flip the linked
artifacts to `archived=1` (rows persist; append-only respected — history is
moved off disk, never erased from the record). This matches universal CI/agent
workspace practice: Jenkins' Workspace Cleanup deletes workspaces post-build
and its distributed variant exists precisely to "keep overall disk usage down
on long-lifetime nodes" (https://plugins.jenkins.io/ws-cleanup/,
https://plugins.jenkins.io/hudson-wsclean-plugin); beads ships `bd gc` for the
same reason. `report` (ephemeral=0) is the deliberate keep-forever class —
correct split.

---

## 5. Failure-mode horizon and mitigation ladder

All mitigations below preserve the axioms (dump = source of truth, append-only,
single-writer).

| Hot spot | Annoyance line | Crossed at [estimated] | Mitigation ladder |
|---|---|---|---|
| Write-verb latency (dump tail) | >200 ms | ~100–150k events (~30 MB dump), yr 3–4 | ① fast Go serializer (buys ~yr 1–2 headroom) → ② **events archive (D7)** — dump drops to the non-events residual (measured ~14.8 MB, ~98 ms serialize at scenario B; worklog archives separately later, tasks/artifacts never archive; note the residual still exceeds the 10 MB trigger, so D7's events step alone does not clear that threshold at B) → ③ per-table incremental serialization: cache each table's text+hash, re-render only tables the verb touched, concatenate in fixed order — byte-identical output, determinism survives → ④ dump sharding: `dump/` dir with one file per table, only changed files rewritten (R1 hashes the fixed-order concatenation; format change = schema-version event) |
| Pre-commit hook | >1 s | not crossed at modeled scales (yr 5 ≈ 0.7 s corrected; pathological ~2 s) | ① `--fast` already skips R1 entirely (spec §7: quick_check + foreign_key_check + DB-only Stage-1 rules R6–R9, R12 + R15), so the hook's `dump` step is the only full-scan serialization pass — already reflected in the 0.7 s figure → ② D7 → ③ same ladder as above |
| Git repo mass | loose objects GB-scale; pack >1 GB | dump >10 MB with daily commits; 10k commits at yr-5 dump size ≈ 1.4 GB pack | ① documented periodic `git gc`/`git maintenance` (defaults alone leave near-worst-case: measured 268 MB where full repack gives 6.7 MB — environment-specific; direction confirmed ~23× by an independent re-run) → ② D7 keeps the live dump small, which shrinks every future blob AND makes frozen archive files one-time costs → ③ aggressive repack windows for the history that exists |
| GitHub per-file warning | 50 MiB file | dump at ~200k events (yr 5, verbose crew) | D7 before the dump nears 50 MB — hard external trigger |
| `verify` R12/R8 | >1 s | quadratic R12: already 7.2 s at 200k events if implemented naively | **v0**: `events(entity)` index or single-pass hash-map implementation (4.7 ms measured with index) |
| `prime` payload | >100 KB into context | ~1,000 open/triage tasks | **v0**: cap each list at N + total count |
| `work/` disk | tens of GB | yr 3–5 uncleaned | v0.x `gc --before` over ephemeral classes (§4); v0: document the retention stance in PROMPT.md |
| quick_check in hook | ~1 s | not reached — corrected measurement: ~30 ms at yr 5, ~75 ms at ~500k events (an earlier draft's 1.0 s figure was inflated) | none needed |
| DB size / query perf proper | — | never at this scale (92 MB vs sqlite.org's ~TB guidance; fossil: 26 yrs in 187 MB) | none needed |

---

## 6. Verdict

**The design is long-horizon sound for the 1–2-person AI-crew scale, on one
condition: D7 (`events archive`) is real and lands before year ~3.** The
architecture concentrates 100% of its scale risk into one deliberately chosen
place — the append-only events table inside the every-verb-rewritten dump
(72% of dump bytes, the only per-action-unbounded table). Year-1 numbers are
excellent everywhere (all verbs ≪100 ms, hook ~0.1 s, dump 5.6 MB). Nothing
degrades catastrophically — everything degrades linearly and predictably, and
every comparable tool that faced this converged on the same age-cutoff archive
answer, which the append-only design makes tombstone-free and git-friendly
(frozen write-once archive segments).

**Must land in v0 (cheap insurance):**
1. `events(entity)` index — or a stated implementation guarantee that
   R8/R12/`log` are single-pass — the naive form is already 7 s at year-5 scale
   (measured), and the index also future-proofs archive-boundary queries.
2. `prime` list caps (first N + counts) — keeps the session-start contract
   O(bounded), not O(backlog discipline).
3. Archive-ready semantics on paper: reserve `meta.events_archived_through`,
   and state in §8.1 that the live dump contains events above the boundary
   (boundary = 0 in v0). Cost: two sentences; it prevents any v0 consumer from
   baking in "all events, always".
4. Pre-commit single-serialization — **corrected after round-5
   verification**: `--fast` per spec §7 is quick_check + foreign_key_check +
   DB-only Stage-1 rules (R6–R9, R12) + R15, with R1 skipped outright, so
   there is no R1/dump pass to "share" — the hook's own `dump` step is
   already the single O(total rows) pass, and that is what the corrected
   ~0.7 s year-5 hook budget reflects. No further halving is available here;
   the added rule battery (~25 ms at scenario B) does not change that.
5. §17 additions: name `gc` (ephemeral-file retention) alongside
   `events archive`; document periodic `git gc` in the generated docs.

**Can wait (v0.x), with concrete triggers:**
- **D7 `events archive`** — activate when events > 50k *or* dump.sql > 10 MB
  *or* any write verb > 200 ms; mandatory before dump.sql approaches 50 MiB
  (52.4 MB, the GitHub per-file warning) — for the modeled solo crew that is
  year 2–4.
- **`gc --before`** for ephemeral workdirs/runs — when `work/` > a few GB or
  >1–2k workdirs.
- Worklog archiving per CLOSED epic — after D7; note that at scenario B the
  post-D7 residual (~14.8 MB measured) still exceeds the 10 MB trigger, so
  this step lands sooner than "if ever". The spec now acknowledges this
  trigger-not-clearable point directly: §8.2 frames the 10 MB trigger as
  "start D7 work, worklog archiving may need to follow" rather than
  promising D7 alone clears the threshold — so this is spec-side accounted
  for, not an open gap this document is raising for the first time.
- Incremental/sharded serialization — only if worklog archiving still leaves
  the dump above ~10 MB (very verbose crews); determinism survives per-table caching by
  construction, so the option is real but likely never needed.

The 1000-epic pathological case adds no new failure mode: dump serialization
~0.9 s (measured) remains the dominant hot spot at higher magnitude
(quick_check corrected to ~75 ms after adversarial re-measurement); per-epic
structures (worklog seq lookup, WIP index, criteria) are all indexed by PK and
flat in cost.
