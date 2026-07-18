# Repo scaffold & entity taxonomy: is the default right?

Status: research complete; verdict at the end (KEEP the seed, one addition to
init's generated docs, a documented opt-in list — no schema change). Method:
web research over primary sources (docs sites, repos, vendor docs, practitioner
write-ups); URLs inline. The current scaffold under review is the §9 seed: `docs/decisions/`,
`docs/research/`, `work/`, `work/runs/`, `work/reports/`, backed by
path_dictionary classes adr / research / workdir / run / report (§5.2).

## 1. Documentation taxonomies — what a small agent-driven repo needs

**Diátaxis** (https://diataxis.fr/, https://diataxis.fr/start-here/) splits
docs by *reader need*: tutorials (learning), how-to (goal), reference (facts),
explanation (why). The split is need-driven, so it collapses when the audience
changes: selftracked's primary reader is an agent that does not "learn" —
tutorials have no audience. What remains maps cleanly onto the existing seed:

- reference → generated (`STATE.md`, `PROMPT.md`, class-contract READMEs);
- how-to (for the agent) → `PROMPT.md`/`AGENTS.md`, not a directory;
- explanation → exactly `docs/research/` (why-exploration) + `docs/decisions/`
  (why-this-choice). Diátaxis itself says it "doesn't impose implementation
  constraints" — it validates the *categories*, not a 4-directory layout.

**arc42** (https://arc42.org/overview, https://github.com/arc42/arc42-template)
is a 12-section architecture template where "everything is optional"; its own
guidance says small projects skip most sections and that §9 (Architecture
Decisions) is best done as ADRs. For a 1–2-person crew arc42 is a *writing
outline* for one eventual architecture doc (which would live as a research or
ADR artifact), not a directory taxonomy. No scaffold implication.

**ADR ecosystem**: MADR 4.0.0 (https://adr.github.io/madr/,
https://github.com/adr/madr) ships bare/minimal templates; adr.github.io
catalogs templates (https://adr.github.io/adr-templates/). log4brains init
creates exactly one directory (`docs/adr` by default) with `template.md`,
`index.md` and a first sample ADR, and deliberately uses date-based filenames —
"no required file numbering schema … avoids git merge issues"
(https://github.com/thomvaill/log4brains). Its monorepo mode (global ADRs +
per-package `docs/adr`) is the same shape as selftracked's (class, scope)
pairs. The seed's `docs/decisions/` + `_template.md` is squarely inside this
mainstream; the date-based naming precedent also matches the research-class
convention already in use.

**RFC/design-doc cultures**: Rust RFCs are one repo directory + one template
(`text/0000-*.md`, https://github.com/rust-lang/rfcs,
https://rust-lang.github.io/rfcs/0002-rfc-process.html); Oxide RFDs are one
AsciiDoc collection covering technical *and* process/culture decisions, >500
docs in ~5 years (https://rfd.shared.oxide.computer/rfd/0001,
https://oxide.computer/blog/rfd-1-requests-for-discussion). These cultures
exist to coordinate *many* stakeholders pre-decision. At 1–2 people the
pre-decision discussion is a research doc and the outcome is an ADR — a third
"rfc/design" class is real but redundant at this crew size; worth documenting
as an opt-in class name, not seeding.

**Runbooks/postmortems** (SRE practice,
https://sre.google/sre-book/postmortem-culture/,
https://incident.io/blog/sre-incident-postmortem-best-practices): runbooks pay
off only where there is an operational surface (services, on-call); mandated
without buy-in they become bureaucracy. A local-first CLI crew usually has no
pager. Postmortem-the-artifact is already covered structurally: `epic close`'s
atomic retro (`close_sweep`, §6.4) is the lightweight postmortem, and a longer
write-up is just a report artifact. Verdict: `runbook` = opt-in class for
adopters who *do* operate something; no postmortem class needed.

## 2. Entity models of tracker tools

**Jira** — exactly three hierarchy levels: epic → (story | task | bug, peers)
→ subtask; components/versions are grouping fields, not hierarchy; initiatives
are a paid add-on level
(https://support.atlassian.com/jira-cloud-administration/docs/what-are-issue-types/,
https://products.seibert.group/blog/jira-story-vs-task-vs-epic). Notably: bug
is a *peer of* story/task with the same lifecycle — the type is a label, not a
different state machine.

**Linear** — issues (the only work unit) + teams, projects, cycles
(auto-repeating cadence), milestones (stages inside a project), initiatives
(above projects), and a built-in Triage inbox state
(https://linear.app/docs/conceptual-model, https://linear.app/docs/projects).
Linear's triage-as-first-class validates selftracked's `NEEDS-TRIAGE` status.
Cycles/initiatives exist to coordinate multiple teams' cadence — no function
at N=1–2.

**GitHub Projects/Issues** — issues + milestones (date-bound grouping) +
custom fields (iteration, points, priority); "epics" are simulated with
umbrella issues (https://github.com/features/issues,
https://github.com/orgs/community/discussions/7267).

**Shortcut** — stories → epics → milestones; iterations for cadence; the
roadmap view is derived from epic/milestone dates
(https://www.shortcut.com/blog/how-we-use-milestones-epics-product-management-clubhouse/).

**beads** (agent-first, closest peer) — issues with a *type field* (task, bug,
chore, epic, message), priorities P0…, statuses open/in_progress/closed,
dependency types blocks / related / parent-child / duplicates / supersedes /
replies-to, hierarchical ids (`bd-a3f8.1`), and the ready-work frontier (open
+ no blocking deps) (https://github.com/steveyegge/beads,
https://betterstack.com/community/guides/ai/beads-issue-tracker-ai-agents/,
https://steve-yegge.medium.com/introducing-beads-a-coding-agent-memory-system-637d7d92514a).
No milestones, no sprints, no doc directories — `bd init` creates only
`.beads/` + updates `AGENTS.md`. selftracked's task_links vocabulary
(depends/relates/supersedes/duplicates) is beads' minus replies-to (no
agent-mail in scope); "discovered-from" provenance is representable as
`relates` + note, or a later link type — additive if pilot data demands it.

**Backlog.md** (agent-first, markdown-native) — entity set: tasks, drafts,
docs, decisions (+ archive/completed), with statuses, dependencies,
acceptance criteria, a project-wide Definition of Done, and milestone
commands; init generates the `backlog/` tree and agent instruction files
(https://github.com/MrLesk/Backlog.md). Independent convergence on exactly
selftracked's doc classes: *decisions* and *docs/research* next to tasks.

**git-bug** — entities are bugs + identities as append-only operation DAGs
(CRDT-style) to support true multi-writer merges
(https://github.com/git-bug/git-bug). Its whole identity/CRDT apparatus is the
cost of abandoning the single-writer axiom — the machinery selftracked
deliberately avoids (§8.4 refuses instead of merging).

**Fossil** — tickets, wiki, technotes, forum bundled in the repo DB
(https://fossil-scm.org/home/doc/tip/www/bugtheory.wiki,
https://fossil-scm.org/home/doc/trunk/www/whyallinone.md) — the 20-year-old
proof that tracker+knowledge-base co-located with code works; its wiki ≈
selftracked's artifact classes over plain files.

### Comparison table

| System | Hierarchy | Bug a distinct type? | Milestones/releases | Cycles/sprints | Knowledge entities | Init/scaffold |
|---|---|---|---|---|---|---|
| Jira | epic → story/task/bug → subtask | Yes (peer type, same lifecycle) | versions (field) | boards/sprints | Confluence (separate product) | n/a |
| Linear | initiative → project → milestone → issue | No (label) | milestones in projects | cycles (auto) | project docs | n/a |
| GitHub Projects | umbrella issue → issue | label | milestones | iteration field | wiki/markdown | n/a |
| Shortcut | milestone → epic → story | story type field | milestones | iterations | docs product | n/a |
| beads | epic → issue (nested ids) | type field | none | none | none (`bd remember` memory) | `.beads/` + AGENTS.md only |
| Backlog.md | milestone → task (+deps) | no | milestone verb | none | drafts, docs, decisions | `backlog/{tasks,drafts,docs,decisions,archive}` + agent files |
| git-bug | flat bugs | n/a | none | none | none | none (in-git objects) |
| Fossil | flat tickets | ticket type field | none | none | wiki, technotes | repo DB |
| **selftracked v0** | epic → story; tasks flat w/ links | **no (deliberate)** | **no (epics+criteria play the role)** | **no** | research, adr, report classes + workdir/run | §9 seed + PROMPT/STATE/AGENTS/hooks |

Recurring entities the model lacks: bug-as-type, milestones/releases,
cycles, comments/threads, identities. Assessment of each in §5.

## 3. Scaffolding precedent — what `init` generates elsewhere

- beads: dot-directory + AGENTS.md, zero doc tree (see above).
- Backlog.md: full entity tree + config.yml + agent instruction files
  (https://github.com/MrLesk/Backlog.md).
- log4brains: one ADR dir + template + first sample doc, location asked once
  at init; monorepo packages declared in `.log4brains.yml`
  (https://github.com/thomvaill/log4brains).
- Language-core tooling ("kit, not framework"): `cargo new` / `go mod init` /
  `gonew` generate a minimal skeleton and leave structure to the adopter;
  cookiecutter exists precisely because cores stay minimal
  (https://fnjoin.com/post/2026-05-15-code-scaffolding-by-stack/,
  https://cookiecutter.readthedocs.io/en/1.6.0/readme.html). "The convention
  is what the scaffolder generates" — convention-over-configuration argues for
  a *small fixed* seed, not an interactive menu.

On per-class opt-in at init time: log4brains asks one question (mono-repo?),
cookiecutter asks many (and is a template engine, not a tool). For selftracked
the deciding constraint is its own invariant — *fresh `init` ⇒ `verify` green*
(§5.2/§9) — which favors a deterministic non-interactive seed. The (class,
scope) dictionary already provides the opt-in mechanism post-init
(`paths set`); an interactive init would duplicate it and break determinism of
the generated tree. Precedent supports: fixed minimal seed + documented named
conventions for common additions.

## 4. The work/ area pattern in agent-driven development

- Practitioner scratchpad convention: one top-level scratch dir with
  purpose-subdirs (scripts/dumps/drafts/traces/fixtures/screenshots) and —
  for concurrent work — **task-specific folders** (`temp/tasks/<slug>/`) so
  parallel agents' evidence doesn't blend; promote useful files out
  intentionally; keep the scratch area out of shared git
  (https://dev.to/jackm-singularity/ai-agent-scratchpad-keep-coding-agents-fast-without-polluting-git-329c).
- Context-engineering write-ups converge on persistent markdown as external
  agent memory across sessions (structured note-taking, plan files)
  (https://www.langchain.com/blog/context-engineering-for-agents,
  https://sourcegraph.com/blog/context-engineering,
  https://nikiforovall.blog/ai/2026/06/08/scratch.html).

Reading onto the seed: `work/` (workdir class, ephemeral) *is* the
task-scoped scratch area — the spec already dates workdirs and links them to
tasks/epics via role='workdir', which is the per-task-folder pattern with a
DB-backed index on top (stronger than the article's README-only convention).
`work/runs/` (ephemeral) = traces/dumps; `work/reports/` (non-ephemeral) =
the "promote intentionally" rule made a first-class class — the durable
distillate. The three-way split ephemeral-workdir / ephemeral-run /
durable-report is *ahead* of the published conventions, not behind them. One
divergence to note: the practitioner convention keeps scratch fully untracked;
the seed tracks `work/` in git with `ephemeral=1` as metadata. That is a
defensible difference (a 1–2-person crew wants scratch synced across
machines and reviewable), but the class-contract README for `work/` should
state the cleanup expectation explicitly, since "ephemeral" is the only thing
standing between `work/runs/` and unbounded growth.

## 5. Verdict

**KEEP the default scaffold as-is.** The two agent-first peers independently
converge on the same doc classes (Backlog.md ships decisions+docs; beads ships
none and its users bolt markdown on); the ADR ecosystem validates
`docs/decisions/` + template; the scratchpad literature validates the
work/-with-per-task-dirs shape and the seed's report class exceeds it. Nothing
in the survey argues for a different *base*.

**Default (unchanged):** `docs/decisions/` (adr), `docs/research/` (research),
`work/` (workdir, ephemeral), `work/runs/` (run, ephemeral),
`work/reports/` (report).

**One addition inside init's generated docs (no new class, no new dir):**
PROMPT.md / the class-contract READMEs should name the *recommended opt-in
classes* with their conventional roots, so adopters extend by copying a named
convention instead of inventing one:

| Opt-in class | Conventional root | Add when | Evidence |
|---|---|---|---|
| `src` | code root(s) | enabling `stale` over sources | already in spec §5.2 |
| `runbook` | `docs/runbooks/` | the project operates anything with failure modes | SRE practice (sre.google) |
| `guide` | `docs/guides/` | human-facing how-tos appear (Diátaxis how-to) | diataxis.fr |
| `rfc` | `docs/rfcs/` | >2 stakeholders pre-decision (Rust/Oxide shape) | rust-lang/rfcs, Oxide RFD 1 |
| `external` | remote alias | out-of-repo references | forks doc D1 |

Plus one file convention (not a class): `CHANGELOG.md` at repo root per Keep a
Changelog (https://keepachangelog.com/en/1.0.0/) when the project starts
cutting releases — a file, so it needs no path_dictionary row.

**Init stays non-interactive.** Per-class opt-in at init would trade the
fresh-init-verifies-green determinism for a menu duplicating `paths set`;
kit-not-framework precedent (cargo/go) and log4brains' one-question init both
support seeding small and extending post-init.

**Entities deliberately omitted, with reasons:**

- **Bug as a distinct type** — in Jira/beads the bug type shares the task
  lifecycle entirely; it is a triage label, not a state machine. For 1–2
  people the title says "fix …"; consistent with the no-kinds verdict
  (forks doc D8). Revisit only if pilot data shows filtering need — then it
  is an additive text column, not a redesign.
- **Milestones/releases** — Shortcut/GitHub use them as date-bound groupings;
  epics + runnable epic_criteria already provide goal-bound grouping with
  *stronger* (executable) completion semantics; releases = git tags +
  opt-in CHANGELOG.md.
- **Cycles/sprints/iterations** — cadence coordination for teams
  (linear.app/docs); an AI crew works continuously; both agent-first peers
  omit them.
- **Initiatives/roadmap layer** — an above-epic level exists for portfolios
  (Jira Premium, Linear); at this scale the epic list in STATE.md is the
  roadmap.
- **Components/versions** — grouping fields for large backlogs; (class, scope)
  already covers the monorepo case that components mostly serve.
- **Comments/threads & identities** — git-bug pays a CRDT/identity tax to
  support multi-writer threads; the single-writer axiom (§8.4) makes both
  unnecessary — notes, status_note and events carry the narrative.
- **Wiki** — Fossil proves in-repo knowledge bases work, but its wiki is
  freeform; selftracked's classed artifacts over plain files are the
  deliberate, indexable replacement.
