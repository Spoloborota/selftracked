# selftracked

Local-first work tracking for repositories driven by AI agents: a SQLite
database your repo carries, a single Go binary with a closed set of
verbs, and a deterministic SQL dump tracked in git as the review surface
and the only sync channel. Tasks file themselves as agents work; statuses
stay true because an integrity engine checks them against git itself;
a session starts by reading `prime` instead of re-reading prose.

**Status: v0, pre-release.** The tracker tracks its own development
(`.selftracked/` in this repository is live state), and the first
external pilot has run its import ladder. Nothing is published yet.

## What it is

- **A closed verb set, not a chat protocol.** `create`, `list`, `show`,
  `set-status`, `park`, `epic`/`story`/`worklog`/`criteria`, `verify`,
  `prime`, `import` — an agent gets exactly the verbs the catalog
  declares, every one with `--json`.
- **Your repo is the database.** State lives in `.selftracked/`: a
  local SQLite file (per-machine, gitignored) and a byte-deterministic
  `dump.sql` (tracked). Git is the sync channel; divergence is handled
  mechanically, not by hope.
- **An integrity engine, not good intentions.** `verify` runs fifteen
  rules — dump/DB byte-agreement, commit citations that must resolve in
  git, audit-trail completeness, schema-version gates — and the
  generated pre-commit hook makes RED unignorable.
- **Colocation posture.** On a repository with existing hooks, `init`
  detects them and chains rather than replaces; your gates stay
  authoritative. Adopting is a directory; abandoning is deleting it.
- **A real migration door.** `import` backfills an existing project's
  history — terminal states, legacy commit ranges, git-first dates —
  behind explicit relaxations, never silently.
  See [the migration guide](docs/migration-guide.md).

## Quick start

```sh
go install ./cmd/selftracked   # or: make binaries → bin/selftracked
cd your-repo
selftracked init               # scaffold + hook activation advice
selftracked create --title "first task"
selftracked prime              # what a session reads at start
selftracked verify             # the integrity engine
```

## Design record

The decisions are documented, with evidence, in `docs/`:

- [`docs/v0-spec.md`](docs/v0-spec.md) — the authoritative
  specification (integrity model, verb catalog, dump contract, sync
  semantics, schema evolution).
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
AI-contribution clause applies (AI-assisted work is the project's normal
mode; a human takes DCO responsibility and verification is never
delegated to the author).

## License

Apache-2.0 (see [NOTICE](NOTICE) for third-party attributions).
