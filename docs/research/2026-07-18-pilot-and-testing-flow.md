# Research: development, debugging, and pilot flow for a git-coupled SQLite CLI

Date: 2026-07-18. Status: research record for the v0 implementation phase
(complements `docs/v0-spec.md` §16). Question: what is the right
development/testing/pilot flow for selftracked, given that its first real
client is a private parent project, and the choice space includes (a) running
experiments against an unversioned snapshot of that project inside a scratch
area, versus (b) installing v0 into the client live and iterating there.

Method: web research on (1) how Go CLI tools are tested against realistic
repositories, (2) how comparable git-native tracker/VCS tools dogfood and
pilot themselves, (3) init/onboarding UX variants, (4) the staged-adoption
pattern. Sources cited inline; concrete client analysis lives in a private
companion document.

---

## 1. Testing a git-coupled SQLite CLI

### 1.1 The `testscript`/`txtar` approach — best fit

The Go toolchain tests `cmd/go` itself with **script tests**: each test is a
`.txt`/`.txtar` file that begins with a shell-like command script followed by
embedded supporting files; each script runs in a fresh temporary work
directory (`$WORK`) with a controlled environment
([go.dev cmd/go script README](https://go.dev/src/cmd/go/testdata/script/README),
[script_test.go](https://go.dev/src/cmd/go/script_test.go)). The
`-testwork` flag preserves the work directory for debugging a failing case.

This machinery is extracted for general use as
[`rogpeppe/go-internal/testscript`](https://github.com/rogpeppe/go-internal)
([package docs](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)):

- `testscript.Main(m, map[string]func(){...})` registers the tool's own
  `main` as an in-process command, so scripts invoke the real binary logic
  with `go test`-native coverage — no separate build step.
- Assertions cover stdout/stderr regexps, exit success/failure (`!` prefix),
  and byte comparison against embedded golden files (`cmp`).
- `UpdateScripts: true` (or `-update`) regenerates golden sections in place —
  the standard golden-file workflow
  ([Bitfield: test scripts and txtar](https://bitfieldconsulting.com/posts/test-scripts-files),
  [rednafi: testing CLIs with testscript](https://rednafi.com/go/testscript-cli/)).
- One txtar file per scenario diffs cleanly in review — the fixture *is* the
  documentation of the behavior ([txtar tour](https://rednafi.com/go/txtar/)).

**Fit assessment for a git-coupled SQLite CLI:** near-perfect, for four
reasons specific to this design:

1. Every selftracked verb is "run a binary in a repo, assert exit code, JSON
   output, and resulting file bytes" — exactly the testscript claim shape.
2. The deterministic dump (spec §8) is a byte-exact text artifact: `cmp
   .selftracked/dump.sql expected-dump.sql` is a one-line golden assertion,
   and `UpdateScripts` maintains it when the serializer legitimately changes.
3. Git fixtures cannot be committed as `.git` directories (git refuses nested
   repos; fixture projects like
   [go-git-fixtures](https://github.com/go-git/go-git-fixtures) and
   [repo-fixture](https://github.com/camertron/repo-fixture) exist precisely
   to package repos as archives). In testscript this problem disappears:
   scripts create real repos at run time (`exec git init`, `exec git commit`)
   in the throwaway `$WORK` — which is exactly what hook, `stale` (R5:
   commits must resolve via `git cat-file`), and divergence-sidecar tests
   need: *real* git objects, freshly made, hermetic.
4. Every `verify` rule and schema gate needs a red fixture (spec §7: "a gate
   that cannot fail is decoration"). A red fixture is naturally one small
   txtar file: seed state, perform the forbidden mutation (e.g. via raw
   `sqlite3` in the script to simulate the adversary), assert `verify` exits
   red with the right rule id.

### 1.2 The resulting test pyramid

| Layer | Mechanism | What it covers |
|---|---|---|
| Unit | ordinary `go test`, in-memory DB | schema triggers/CHECKs, serializer, whitelist parser (+ fuzzing — spec §8.5/§16) |
| Integration | testscript txtar in `testdata/script/` | verbs end-to-end, hooks, dump/load, divergence, red fixtures per rule |
| Determinism CI | matrix runners byte-compare dumps | cross-OS byte-equality (spec §16) |
| Dogfood | the tool's own repo runs on it | real workload, real review surface |
| Pilot | one real client, staged (see §4) | import fidelity, gate coexistence, ergonomics |

Complementary evidence that in-process CLI testing plus docs-as-fixtures
works at scale: `sqlite-utils` drives its CLI in-process via Click's
`CliRunner` and even tests that every command is documented
([sqlite-utils tests/test_docs.py](https://github.com/simonw/sqlite-utils/blob/main/tests/test_docs.py)).

---

## 2. How comparable tools dogfood and pilot

- **Fossil** (SQLite-backed VCS+tracker): "the first project hosted by
  Fossil was Fossil itself" — a prototype capable of self-hosting was reached
  on 2007-07-16, and only **after a few months** of self-hosted development
  was it trusted with an adjacent real client, the SQLite documentation
  repository (2007-11-12), before eventually hosting SQLite proper
  ([Fossil history](https://fossil-scm.org/home/doc/tip/www/history.md),
  [self-hosting page](https://fossil-scm.org/home/doc/trunk/www/selfhost.wiki)).
  This is the cleanest documented instance of the staged pattern: synthetic →
  self-host → one adjacent real project → flagship.
- **Jujutsu (jj)**: "All core developers use Jujutsu to develop Jujutsu"
  ([jj README](https://github.com/jj-vcs/jj)). Critically, dogfooding on real
  repos is made *safe* by the colocated design: the `.git` repo remains fully
  valid and authoritative, `jj` state lives beside it, and abandoning the
  experiment is deleting a directory
  ([git-compatibility docs](https://docs.jj-vcs.dev/latest/git-compatibility/)).
  The incumbent system keeps working throughout the pilot.
- **beads** (Yegge; SQLite cache + git-tracked JSONL): dogfooded by its
  author on his own agent-colony project, with "24+ dogfooding missions"
  recorded ([steveyegge/vc](https://github.com/steveyegge/vc),
  [Introducing Beads](https://steve-yegge.medium.com/introducing-beads-a-coding-agent-memory-system-637d7d92514a)).
  Real-project pilots surfaced exactly the class of failure a synthetic suite
  misses: recurring JSONL merge conflicts needing manual/agent cleanup, and
  one documented incident where ~80 issues were lost by taking "theirs"
  during a rebase — recovered only because the git-tracked text artifact let
  agents reconstruct the database from history
  ([Beads best practices](https://steve-yegge.medium.com/beads-best-practices-2db636b9760c),
  [ianbull experience report](https://ianbull.com/posts/beads/)).
  Two lessons: (1) the git-tracked dump is the recovery channel — its
  integrity gates deserve the most testing; (2) sync/merge semantics are
  where real pilots break tools, so pilot on a repo whose primary data the
  tool cannot damage. selftracked's single-writer thesis plus refuse-don't-merge
  divergence semantics (spec §8.4) are aimed at precisely the failure beads hit.
- **git-bug**: stores bug data as git objects outside the working tree —
  "non-invasive" adoption in any existing repo; the tracker rides in refs, so
  the host's files are untouched
  ([git-bug README](https://github.com/git-bug/git-bug)). The generalizable
  point: minimize working-tree footprint during adoption.
- **Backlog.md** (markdown tracker for AI agents): `backlog init` runs in an
  existing repo; **re-running init is safe and preserves existing config**,
  pre-populating prompts; the wizard offers agent-integration choices
  (instruction file / MCP / skip) and `--no-git` for filesystem-only trials
  ([Backlog.md README](https://github.com/MrLesk/Backlog.md)).

---

## 3. Init/onboarding variants worth copying

| Variant | Seen in | Worth copying? |
|---|---|---|
| `init` in an existing repo, additive files only | Backlog.md, git-bug, jj | Yes — spec §9 already conforms; keep init strictly additive (new dirs/files, `.gitignore` appends), never rewriting host files |
| Idempotent re-init preserving config | Backlog.md | Yes — `init` on an initialized repo should be a safe no-op/refresh, not a `--force`-only error path |
| Colocation: incumbent stays authoritative during trial | jj `--colocate` | Yes, as a *pilot posture*: during adoption the host's existing tracking remains the system of record until explicit cutover; selftracked runs beside it |
| Hooks offered, never seized | jj (doesn't touch git hooks), spec §9 | Yes — `git config core.hooksPath` is a **singleton**; a host repo may already point it at its own hook directory. `init` must print the activation command (spec §9 does) and additionally *detect an existing `core.hooksPath` and print a chaining recipe instead of an overwrite* — this is the one init gap this research surfaced |
| `import` / `--legacy` batch onboarding | spec §6.2/§10; beads' JSONL import | Yes — already specified; the pilot's main workload |
| Trial mode without git side effects | Backlog.md `--no-git` | Low priority — git-coupling is the point here; the testscript sandbox covers the "try it safely" need |
| Agent-instruction file generated by init | Backlog.md instruction file, spec's `PROMPT.md`/`AGENTS.md` | Already specified |

---

## 4. The staged-adoption pattern

Across the surveyed tools the sequence is consistent — **synthetic fixtures →
self-hosting → one real client → generalization** — and the switch points are
observable, not calendar-based:

1. **Synthetic until the gates hold.** The full txtar suite green, a red
   fixture per verify rule and schema gate, dump determinism across OS.
   Fossil wrote a VCS before trusting it with its own history; the prototype
   self-hosted only when it could.
2. **Self-host at first usable increment.** Fossil's first client was
   itself; jj's core developers live on jj. Spec §16 already mandates this:
   "the moment `init` works, this repo's backlog moves into `.selftracked/`".
   Self-hosting exercises real workload shapes (session-end bookkeeping
   commits, divergence after pulls) that fixtures under-represent, while the
   only repo at risk is the tool's own.
3. **One real client, snapshot-first, then live.** Fossil took an adjacent
   low-stakes repo (SQLite *docs*, not SQLite) months before the flagship.
   Beads shows what a live pilot on real data costs when sync semantics are
   still moving — data loss recovered only via git archaeology. The safe
   intermediate is running import/integration experiments against a **local
   clone of the client** (a clone, not a file copy — commit-resolution and
   hook behavior need real git objects), kept out of the tool repo's tracked
   tree. Live installation into the client comes only when: the importer
   round-trips the snapshot with `verify` green; the tool's own repo has run
   on it through real sessions; installation is provably reversible (delete
   the tool's directory + revert one commit, host files untouched); and the
   host's incumbent gates keep running (colocation posture, hooks chained
   rather than replaced).
4. **Generalize.** Only after the first client's friction is folded back
   into fixtures (as anonymized synthetic equivalents) does the generic
   migration guide harden.

**Verdict on the (a)-vs-(b) framing:** it is not a binary. The evidenced
sequence is synthetic suite → self-host → snapshot-clone experiments →
reversible live install with the incumbent system still authoritative — each
promotion gated by an observable exit criterion, not by enthusiasm.

---

## Sources

- https://go.dev/src/cmd/go/testdata/script/README · https://go.dev/src/cmd/go/script_test.go
- https://github.com/rogpeppe/go-internal · https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript
- https://bitfieldconsulting.com/posts/test-scripts-files · https://rednafi.com/go/testscript-cli/ · https://rednafi.com/go/txtar/
- https://github.com/go-git/go-git-fixtures · https://github.com/camertron/repo-fixture
- https://fossil-scm.org/home/doc/tip/www/history.md · https://fossil-scm.org/home/doc/trunk/www/selfhost.wiki
- https://github.com/jj-vcs/jj · https://docs.jj-vcs.dev/latest/git-compatibility/
- https://steve-yegge.medium.com/introducing-beads-a-coding-agent-memory-system-637d7d92514a · https://steve-yegge.medium.com/beads-best-practices-2db636b9760c · https://ianbull.com/posts/beads/ · https://github.com/steveyegge/vc
- https://github.com/git-bug/git-bug
- https://github.com/MrLesk/Backlog.md
- https://github.com/simonw/sqlite-utils/blob/main/tests/test_docs.py
