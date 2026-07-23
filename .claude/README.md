# `.claude/` — agent configuration

Two layers, split deliberately along what belongs to the project and what
belongs to a machine.

## Tracked (this directory, committed)

- `settings.json` — a `SessionStart` hook that emits the tracker's session
  context: `selftracked prime --json`, falling back to `load` + retry on a
  fresh clone and to a static error JSON when the binary is absent (spec
  §11.1 — the scaffold's chain; the bootstrap-era ledger print retired with
  the ledger at S10). Its job is unchanged: make "read the state first" a
  property of the environment rather than a rule an agent has to remember.
  To silence it locally, override the hook in `settings.local.json`.
- `CLAUDE.md` — the project's working rules, read automatically at session
  start. It lives here rather than at the repository root: both locations
  are first-class project-memory paths, and keeping it beside the settings
  and this README puts every agent-facing file in one place.

Both are tracked on purpose. This project ships the same pattern to its
adopters: `init` generates `PROMPT.md`, `AGENTS.md` and `.claude/` files as
**tracked** artifacts (spec §9), so that a fresh clone carries its own
instructions. A repository that hid its own would be recommending one thing
and doing another.

## Not tracked (gitignored)

- `settings.local.json` — per-machine overrides: local paths, personal
  permissions, anything specific to one workstation.
- `work/local/` — operational notes that are nobody else's business:
  which models run which roles, orchestration details, private analysis,
  translations kept for the owner's convenience.

The rule of thumb: **what a contributor must obey is tracked; what only this
machine needs to know is not.** The project's methodology is not a secret —
it is published in `docs/`, and it is most of what this project is.
