# Market landscape: local-first / git-native task trackers for AI agents

Status: CURRENT (as of 2026-07-17; star counts are approximate
same-day snapshots from the cited pages). Method: web research over primary
sources (repos, changelogs, issue trackers, HN threads); every claim carries
its source. Figures are third-party observations, not measurements made in
this repo.

## beads (steveyegge/beads, ~25.4k★, v1.1.0 2026-07-04)

- Post-v1.0, embedded Dolt is the only first-class backend; per its own
  CHANGELOG v1.0.0 (2026-04-02): SQLite backend deprecated, daemon removed.
  `issues.jsonl` demoted to "an export only — not the source of truth, not a
  backup"; sync via `bd dolt push/pull`. Now the substrate for a multi-agent
  fleet orchestrator — no longer aimed at small-crew use.
- The multi-writer migration cost is documented in its own tracker:
  - Issue #2573: "Moving to dolt pretty much made beads unusable for me…
    My experience with beads was great with sqlite."
  - CHANGELOG v1.0.0 concedes "critical reliability issues that affected the
    v0.55–v0.63 series"; migration 0050 notes un-synced clones crossing the
    boundary "become permanently un-mergeable"; Homebrew rolled back v1.0.5.
- Worth adopting from its design: verb vocabulary (`ready/create/update/
  claim/close/show/dep/remember/prime/onboard/stale`), typed link set
  (blocks/relates-to/duplicates/supersedes/replies-to), hierarchical ids,
  AGENTS.md/CLAUDE.md generation at init, `prime` session bootstrap.

## The 2026 "beads refugee" ecosystem — the market voting for single-writer

| Project | What it kept / dropped |
|---|---|
| beads_rust "br" (Dicklesworthstone/beads_rust, ~995★, Yegge-endorsed) | Froze the pre-Dolt SQLite+JSONL architecture; no daemon; explicit git ops. Verbs worth studying: `ready/blocked/stale/defer/orphans/dep tree/cycles/epic/graph/changelog/audit/robot-docs/schema`; optional MCP feature flag. |
| ticket (wedow/ticket) | Bash + flat markdown; kept only graph dependencies. Author on HN: the daemon kept "syncing the wrong things at the wrong times." |
| Trekker (obsfx/trekker) | Minimal tracker as a Claude Code plugin. |
| beans (hmans/beans, ~874★) | Markdown files + a GraphQL query interface for agents; `prime`; SessionStart/PreCompact hooks. |

**beads_rust is the nearest direct competitor** (same storage philosophy,
same agent focus). It lacks: a path dictionary, a docs/self-documentation
layer, and evidence/doc staleness (its `stale` = inactive issues).

## Others

- **shiplog** (devallibus, ~53★): a skill/governance layer over Git+GitHub —
  evidence-linked closure (issues cannot close without a linked merged
  PR/commit/decision artifact), model-provenance stamping. GitHub-bound, no
  local DB. Steal the evidence-linked-closure semantics.
- **grite** (neul-labs, Rust, ~8★): issues as an append-only event log in git
  refs + CRDT merge — "multi-writer done right"; agent-hostile random 128-bit
  ids, no human-diffable dump. The philosophical counter-thesis to watch.
  Steal: `doctor` verb; AGENTS.md on init.
- **PlanDB** (~97★): Rust+SQLite local "Jira for Claude Code"; steal atomic
  `go` claim, `done --next`, BM25 search, pre/post conditions. No git story.
- **Backlog.md** (~5k★): markdown-file-per-task + MCP; the mindshare leader
  at the unstructured end; concede that segment.
- **saga-mcp** and a commodity tier of SQLite-MCP todo servers.
- dstask alive but quiet; git-appraise and SIT remain dead; engram (~2.4k★)
  is an agent-memory neighbor, not a tracker.

## Articles digest

- OSS Insight "The Agent Memory Race of 2026": four incompatible memory
  architectures, none subsumes the others → purpose-built stores validated.
- memweave (Towards Data Science): markdown as truth + SQLite as derived
  index — the inverse of selftracked's model; both reject vector DBs at small
  scale.
- Several 2026 pieces argue separating storage from search; one describes
  SQLite as a "thin transactional gatekeeper" in front of file writes — the
  closest published articulation of single-writer discipline found.
- "AGENTS.md is the new ADR": architecture docs as executable constraints.
- The literal framing "single-writer discipline for agent fleets" appears
  nowhere — available to own.

## Synthesis

Uncontested gaps (nothing found doing these): (1) a path dictionary mapping
artifact classes to filesystem roots; (2) staleness of documentation/evidence
against code reality; (3) the combination tracker + typed links +
docs-registry + deterministic git dump; (4) a deterministic dump as
source-of-truth mirror (only beads_rust keeps JSONL-in-git).

Positioning consequence: selftracked cannot win as "a better tracker" against
beads_rust; it wins as tracker + self-documentation registry + evidence/
staleness layer. beads' own changelog and issues are the documented case for
single-writer.

Not verified this round: Claw Task Hub's repository; henriquebastos/beans;
exact Backlog.md star count (sources conflicted); whether current beads
requires CGo (an earlier claim not confirmed by its README as of 2026-07-17).
