# Working in this repository

selftracked is a local-first self-tracking system for small AI-agent crews:
a SQLite database plus a Go CLI with a fixed verb set, a deterministic SQL
dump tracked in git, and a path dictionary. Since S10 the project is
**self-hosted**: `.selftracked/` tracks it, and `selftracked prime` (or
`STATE.md`) says how far it has got.

Revision numbers are deliberately absent from this file: each document states
its own, and a number copied here is a number that goes stale unnoticed.

## Read these before doing anything

| Artifact | What it is |
|---|---|
| `docs/v0-spec.md` | The specification: the single authoritative description of v0. |
| `docs/v0-execution-plan.md` | How v0 got built and how the spec is governed; freezes at S10 close as the bootstrap's historical record — its §2 lifecycle contract stays live. |
| `selftracked prime` / `STATE.md` + `PROMPT.md` | The living project state and the working contract — read them first in every session, update the tracker last (the bootstrap ledger and inventory retired at S10; git history keeps both). |
| `docs/research/` | The evidence base. Every design decision in the spec cites a document here. |

## Non-negotiables

1. **The spec is authoritative.** Any implementation decision that deviates
   from `docs/v0-spec.md` is written as a change proposal under
   `openspec/changes/<name>/` and reviewed **before** the deviating code
   merges — never as a code comment, a task note, or a silent choice. An
   approval given in conversation is not an amendment: the proposal exists
   first, the code lands after.
2. **Every obligation is accounted for.** The bootstrap inventory retired at
   S10 (git history keeps its final state; its obligations live on as the
   tracker's stories and criteria). A change that adds scope needs a tracker
   record — a story, task, or criteria row — and, if the spec did not ask
   for it, an amendment; the amendment's applier re-walks the affected
   obligations as part of the same story's DoD (plan §9).
3. **Done means a command exited 0.** Work closes when its verification
   commands pass, with the evidence recorded in the tracker (worklog
   `gate`/`review` fields, criteria `evidence`), and a fresh-context
   reviewer has re-run them. An agent's assertion that something works is
   not evidence.
4. **The implementing agent never edits its own stage's scope or definition
   of done.** Those changes travel the amendment flow like any other.
5. **Reviews report, they do not fix.** Review passes run read-only, enumerate
   defects with evidence, and propose nothing; the fixes are decided
   afterwards, and anything touching privacy, security, or a deviation from
   the spec is ratified by the owner. The full protocol —
   `.claude/rules/critic-protocol.md`; the plan §5 is its authority.
6. **Commit freely, never push.** Local commits are made as often as work
   reaches a coherent state — they cost nothing and are the only restore
   point untracked files do not have. **Publishing is a separate act and is
   the owner's alone**: never run `git push`, and never suggest a workflow
   whose next step is a push. A commit is invisible until pushed, which is
   why the prohibition sits on the push and not on the commit.

## Assertion provenance — read this before asserting anything

A general "verify your facts" rule is not enough: this project has already
published an unverifiable count into a document's revision history, asserted
that a set of obligations was correctly placed when three of them were not,
and twice pasted non-English text into a public file while confident the rule
was being followed. Each time the claim came from a **secondary source** —
earlier prose in the same session, a memory of a rule, an assumption about
what a script had done — while the primary source was one command away.

So this is a **closed list**, not an exhortation. These five claim types
require a fresh command **in the same reply**:

1. **Any number** about the repository, the inventory, or the spec — counts,
   sizes, row totals, percentages, "N findings".
2. **Any `file:line` citation** — open the file, at that line, now.
3. **Any claim about what is on disk** — a file, a path, an installed tool.
   Use `ls` / `find` / `command -v`. A comment or a document *about* a file
   is not evidence that the file exists.
4. **Any claim about what code does** — from reading the code now, not from
   its comment, not from a document describing it, not from a test's name.
5. **Any event date** — from `date`, a file's mtime, or `git log`; never from
   the session's own narrative.

**Mark every factual claim about the repository with its provenance:**

- `✓` — a command was run, or a file opened, **in this same reply**, and its
  output is in the thread.
- `?` — from memory, a document, or earlier context. **Not re-verified.**

A `?` claim may not ground a decision and may not enter a document.

The advisory hook `.claude/hooks/assertion_check.py` flags the mechanical
half of this and cannot block. It catches an unbacked claim and a
provably-impossible date; it cannot catch a claim verified with the wrong
query — only a review does that. **A quiet hook is not evidence that the
numbers are right.**

## Scope discipline

The stage or sub-batch in progress is the session's goal. When a request —
the owner's, or your own discovery — falls outside it:

1. **Name the drift out loud.** "This is outside stage S<n>."
2. **Route by size.** Under ~15 minutes *and* it unblocks the current work →
   do it now. Otherwise → a task in the tracker (`selftracked create
   --title … --note …`; `park` it if deferred). Worth an epic → propose one.
3. **The decision is the owner's.** An explicit choice to switch is
   legitimate and is not argued with. On silence, park it and return.
4. **Park by writing the line immediately** — not "at the end of the
   session", which is where parked items go to be forgotten.

## Conventions

- Documentation, code, comments and commit messages are written in English —
  including quoted decisions. An owner's ruling given in another language is
  recorded in English by meaning, never pasted verbatim in the original: a
  repository with one language is one a contributor can read end to end, and
  a decision's force is in what it decided, not in its phrasing.
- Durable documents never duplicate state that a verb or `STATE.md` can
  print; numbers quoted from the database carry an `as of dump <sha12>`
  anchor; dates come from the clock or git, never from a session's narrative.
  The full rules ship in the generated `PROMPT.md` and are specified in
  `docs/v0-spec.md` §9.
- Research, review and analysis results are saved as dated documents under
  `docs/research/`, not left in conversation.
