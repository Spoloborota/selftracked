# ADR 0001: filesystem containment against a hostile clone stays out of scope

- **Status:** accepted
- **Date:** 2026-07-24
- **Deciders:** repository owner

## Context

selftracked runs inside a git repository and reads state that the repository
supplies: the tracked `dump.sql`, the path dictionary it rebuilds, and the
`.selftracked/` layout itself. A clone can therefore be hostile — it can
commit a symlink where a directory is expected, or values crafted to steer a
filesystem operation.

The spec already answers this, in three places that agree:

- §1.1 — against a deliberate adversary, prevention is impossible in an
  embedded database; **detection is the contract**.
- §8.5 — the security boundary is the **dump parser**: a byte-equal DDL
  check, a token-shape whitelist, and mandated fuzzing (§16).
- §14 and the `criteria` verb's threat model — runnable criteria are shell
  commands from repo state, so running them "is the same trust decision as
  running the repo's build/tests or its tracked hooks; a hostile branch
  already owns those surfaces".

In July 2026 a pre-publication security review nonetheless produced a
four-pass campaign that added a symlink-containment layer across the write
and read paths. It was withdrawn before publication and its commits were
dropped from history.

## Decision

We will not defend the filesystem layout of a repository against the
repository itself. A verb that follows a symlinked path in a clone the
operator chose to run the tool inside is not treated as a vulnerability, for
the reason §14 already states: that adversary holds a code-execution surface
this project deliberately accepts, so a containment layer costs behaviour and
complexity without removing anything from the attacker's reach.

Two narrower classes remain in scope and are defended, because they are
reachable from verbs an operator runs *without* electing to execute repository
state:

1. Repository-supplied values reaching a `git` subprocess as an **option**
   rather than as data (argument injection).
2. Repository-supplied text entering the database, bounded by the parser's
   whitelist and the write-time text rules.

Re-opening the containment question requires a change proposal under
`openspec/changes/`, because it revises the trust boundary the spec declares.

## Consequences

Easier: benign layouts keep working — a `.selftracked/` symlinked into shared
or synced storage, a relocated database — none of which the tool refuses.
The verb surface stays free of a refusal class with no recovery path, and the
exit-code contract of §6.1 keeps one meaning per code.

Harder: a reviewer applying the standing security lens
(`.claude/rules/critic-protocol.md` rule 8) will keep rediscovering the
symlink class and rating it critical. That is expected. Before accepting such
a finding, check it against the threat model the spec declares — a finding
that assumes a stricter boundary than §1.1/§8.5/§14 is a proposal to change
the spec, and travels the amendment flow, not the fix flow.

Also harder: an operator who *wants* the stricter boundary has no switch for
it in v0.

## Evidence

The withdrawn campaign, the five-lens critic round that adjudicated it, and
the measured cost of its refusals are recorded in the private research
archive, which is deliberately untracked: it maps this project's own attack
surface.
