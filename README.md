# selftracked

Local-first work tracking for repositories driven by AI agents — a SQLite
database your repo carries, one Go binary with a closed set of verbs, and a
byte-deterministic SQL dump tracked in git as the review surface and the only
sync channel.

[![CI](https://github.com/Spoloborota/selftracked/actions/workflows/ci.yml/badge.svg)](https://github.com/Spoloborota/selftracked/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.25%2B-00ADD8.svg)](go.mod)

Tasks file themselves as agents work. Statuses stay true because an integrity
engine checks them against git itself. A session starts by reading `prime`,
not by re-reading prose.

> **Status — v0.** selftracked is self-hosted: `.selftracked/` in this
> repository is its own live state, and it has run its first external import
> pilot. Early, but real — `make gates` is green and the integrity engine
> gates every commit.

## Why

An AI-agent crew does not need another SaaS tracker behind an API, or a
Markdown board that quietly drifts from reality. It needs a **closed,
machine-checkable verb set** and a **git-native review surface**: every state
change is a verb, every claim is checked against git, and the entire state is
one deterministic file a human can read in a pull request. selftracked is
that — small, offline, and honest about what it has and has not verified.

## What it is

- **A closed verb set, not a chat protocol.** `create`, `list`, `show`,
  `set-status`, `park`, `epic`/`story`/`worklog`/`criteria`, `verify`,
  `prime`, `import` — an agent gets exactly the verbs the catalog declares,
  every one with `--json`.
- **Your repo is the database.** State lives in `.selftracked/`: a local
  SQLite file (per-machine, gitignored) and a byte-deterministic `dump.sql`
  (tracked). Git is the sync channel; divergence is handled mechanically, not
  by hope.
- **An integrity engine, not good intentions.** `verify` runs fifteen rules
  (R1–R15) — dump/DB byte-agreement, commit citations that must resolve in
  git, audit-trail completeness, schema-version gates — and the generated
  pre-commit hook makes RED loud: the only bypass is explicit, one-shot, and
  recorded in the audit trail.
- **Colocation posture.** On a repository with existing hooks, `init` detects
  them and chains rather than replaces; your gates stay authoritative.
  Adopting is a directory; abandoning is deleting it (plus dropping the one
  chained line from your hook).
- **A real migration door.** `import` backfills an existing project's history
  — terminal states, legacy commit ranges, git-first dates — behind explicit
  relaxations, never silently. See [the migration guide](docs/migration-guide.md).

## Quick start

Install (Go 1.25+):

```sh
go install github.com/Spoloborota/selftracked/cmd/selftracked@latest
```

Or build from a clone: `make binaries` → `bin/selftracked`.

```sh
cd your-repo
selftracked init                        # scaffold + hook activation advice
selftracked create --title "first task"
selftracked prime                       # what a session reads at start
selftracked verify                      # the integrity engine
```

Every verb takes `--json`; `selftracked <verb> --help` prints its signature.

## Design record

The decisions are documented, with evidence, in `docs/`:

- [`docs/v0-spec.md`](docs/v0-spec.md) — the authoritative specification
  (integrity model, verb catalog, dump contract, sync semantics, schema
  evolution).
- [`docs/research/2026-07-17-market-landscape.md`](docs/research/2026-07-17-market-landscape.md)
  — why not an existing tracker.
- [`docs/research/2026-07-18-sqlite-advanced-features.md`](docs/research/2026-07-18-sqlite-advanced-features.md)
  — driver behavior verified empirically before reliance.
- [`docs/research/2026-07-18-db-migrations.md`](docs/research/2026-07-18-db-migrations.md)
  — the versioned-rebuild model and why migration tools were rejected.
- [`docs/research/2026-07-18-go-stack.md`](docs/research/2026-07-18-go-stack.md)
  — stack and pinning decisions.
- [`docs/research/2026-07-18-spec-to-execution-planning.md`](docs/research/2026-07-18-spec-to-execution-planning.md)
  — how a 1400-line spec was executed by agents without rotting.
- [`docs/research/`](docs/research/) — the rest of the evidence base.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) — DCO sign-off required, and the
AI-contribution clause applies (AI-assisted work is the project's normal mode;
a human takes DCO responsibility, and verification is never delegated to the
author).

## License

Apache-2.0 (see [NOTICE](NOTICE) for third-party attributions).
