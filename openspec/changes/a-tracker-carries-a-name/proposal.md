# Change: a tracker carries an opaque instance digest, and `prime` says which one it is

Target: `docs/v0-spec.md` §11.1 (the `prime` JSON contract gains one
field, and the human digest one token), §9 (the `.selftracked/` tree
gains one gitignored per-machine file), and §6.1 (the read-verb contract
gains its one stated exception: `prime` may create that file); revision
3.37 → 3.38.
`internal/verb/prime.go` — the field, its computation and the digest
line; `internal/verb/prime_test.go` — `TestPrimeTotalsAndNoProseScan`'s
classification map; `internal/scaffold/templates/gitignore` — one line,
and with it `internal/scaffold/testdata/golden/.gitignore` and this
repository's own `.gitignore:31-33` block. Fixtures and golden JSON.
**No §14 amendment** — see below. No schema change; no `meta` row; no
write verb's output changes.
Status: **accepted** · raised 2026-07-26 by task #24 under epic
`adoption-contract`, story S1 · review tier **FULL** (plan §5, D-EP7) ·
**this was a design fork the epic could not settle for itself; the branch
below is the researched recommendation** · **revised 2026-07-26 against a
critic round that refuted this proposal's first branch — the identity was
a directory basename and is now an opaque digest; the basename is
recorded as the rejected branch** · **revised again the same day: the
digest's preimage-oracle exposure was raised as a security-class
escalation under the critic protocol, upheld by the owner, and the digest
is now salted — the unsalted form is recorded as the rejected branch** ·
the security-class question therefore carries the owner's own
ratification; the remaining branch choice was ratified by the coordinating
agent 2026-07-26 under the owner's explicit 2026-07-26 grant of autonomy for this session · applied to the spec the same day

## Why

No verb says which tracker it is operating on.

The incident: two trackers on one machine — a coordinating repository and
a sandbox host — a wrong working directory, and a write that landed
silently in the wrong database. Nothing in any verb's output carried a
cue. The verb resolved `.selftracked/` against the working directory,
found one, and did exactly what it was told.

Verified 2026-07-26: `prime`'s JSON payload keys are
`epics_active, epics_paused, epics_backlog, ready, triage, in_review,
stale, sprint_goals, notices, totals, dump_divergence,
dump_requires_newer_binary` — nothing identifies the instance. The human
digest opens with `active epics: 1` and the epic's goal line. An agent
reading that at session start learns everything about the *state* and
nothing about *whose* state it is.

### Why the obvious identities do not work

**A content digest cannot answer the question.** The failure this must
catch is two trackers that are near-copies of each other — which is
exactly how they arise: a rehearsal copy, a sandbox host, a
copy-on-write clone (`docs/migration-guide.md` §8 recommends one per
attempt). A fresh `cp -a` copy is **byte-identical to its source** until
it diverges, so any digest of tracker *content* — the dump's SHA, a hash
of the schema plus the epic set, the sidecar — returns the same value on
both sides at precisely the moment the confusion is most likely. An
identity derived from content is not an identity; it is a similarity
measure, and it reports "same" for the case it exists to distinguish.

**The absolute path itself cannot be emitted.** §14 states the rule
flatly: "selftracked's own verbs never write hostnames, usernames, or
absolute paths." That is not stylistic — `prime`'s output is what a
SessionStart hook injects into an agent's context, and an agent pasting
verb output into a task note would seat a machine path in a dump that is
published and permanent.

**And the tracker's directory name — this proposal's own first branch —
does not survive its own channel.** It is recorded in full below, because
it was the researched recommendation until a critic round measured it.

## What changes

**A tracker's identity is a short hex digest of a per-machine salt and
its root's absolute path.** One field, computed at run time; nothing
about it enters the database or the dump.

**Input: `salt || path`.**

The **path** half is the absolute path of the directory containing
`.selftracked/`, **physically resolved** — symbolic links evaluated —
with no trailing separator. The physical basis is load-bearing and is
specified here rather than left open: Go's `os.Getwd` documents that when
a directory "can be reached via multiple paths (due to symbolic links),
Getwd may return any one of them", so a logical basis makes one tracker
report two different identities depending on how the session reached it —
which breaks the single question the field answers.

The **salt** half is a random **per-tracker** value in
`.selftracked/instance.salt`, **gitignored**, generated on first use when
absent **from a cryptographic random source**. It is what closes the
preimage oracle documented below, and it is
the reason this proposal touches §9 and the shipped `.gitignore` at all.

*Correction 2026-07-26 (security lens, applied at rev 3.42): the first
draft said "per-machine" and named no random source. Both were wrong in
the same direction — they described the salt as stronger and more shared
than the design makes it. The file lives inside the tracker, so two
trackers on one machine hold two independent salts; and the entire
argument below is that the salt is not guessable, which a specification
that never says where the randomness comes from leaves to the
implementer, whose cheapest correct-looking choice is a non-cryptographic
generator. Neither correction changes the design; both close a gap
between what the design is and what the text promised.*

It costs nothing in capability: the digest was already per-machine,
because the absolute path differs on every machine, so cross-machine
comparison was never on offer to lose. Copy-distinction survives intact —
a `cp -a` copy carries the salt with it and still sits at a different
path, so its digest still differs from its source's, which is the case
that forced a path-derived identity in the first place.

**The salt does NOT live in `meta`.** `meta` rows are serialized into the
tracked dump (verified 2026-07-26: `INSERT INTO meta (key, value)` at
`.selftracked/dump.sql:324-328`), so a salt stored there would be
published beside the digest it salts — which is not a salt. Compounding
it, §6.2's `config` row states new `meta` keys "arrive only with schema
versions", so the wrong home also costs a schema version. The gitignored
file is not a workaround for that; it is the correct home, and §9 already
establishes both the place and the pattern —
`internal/scaffold/templates/gitignore:1-6` gitignores
`/.selftracked/db.sqlite*`, `/.selftracked/dump.hash` and
`/.selftracked/skip-pending`, root-anchored, under the heading
"local, per-machine files (the tracked surface is dump.sql)". The salt is
a fourth entry in an existing category, not a new kind of thing.

**Two failure modes, both resolving to an omitted field.**

- The path cannot be resolved — the permission-error class the sibling
  proposal `resolution-names-the-root-it-found` specifies.
- The salt cannot be read or written — a read-only checkout, a
  permission error, a full filesystem.

In both cases **the field is omitted**. It is specifically **not**
computed from an unresolved path, and specifically **not** fallen back to
an unsalted digest. A silent fallback to the weaker form is how the
oracle comes back: it would appear exactly where the environment is
unusual, produce a value indistinguishable from the salted one, and
publish it. An absent field is honest; a quietly-downgraded one is the
defect this whole revision exists to remove.

**Length: 12 hex characters**, the leading 48 bits of the SHA-256 of that
path. Twelve because the project already has exactly one truncation
convention — §9's `as of dump <sha12>` anchor — and a second length would
be a second convention with no reason behind it. The threat model here is
confusion between a handful of trackers on one machine, not adversarial
collision, so 48 bits is far past sufficient.

**Position: the first field of `primeOutput`**, immediately before
`epics_active`; in the human digest, the first token of the first line,
before `active epics:`. §11.1 makes field order part of the contract
while admitting its own prose enumeration "has never been in payload
order" — so this placement is specified against the **struct**
(`internal/verb/prime.go:31-50`), which is what that clause directs a
reader to. The subject of a payload belongs before its predicates, and an
agent that reads the state before learning whose state it is has already
formed the model this field exists to correct.

**Json tag: `instance`** — the project's own word for a tracker
(`internal/verb/pipeline.go:37`, "a missing local instance"; §11.1's
"instance-scoped" events). The name is the cheapest thing here to change
and the owner should overrule it freely.

**Spec §9** gains the salt file to the `.selftracked/` tree it
enumerates, in the form the three existing gitignored entries already
use — `instance.salt   # gitignored — per-machine identity salt
(§11.1)`. That tree is the spec's authoritative list of what lives in
`.selftracked/`, so a file present on disk and absent from it is exactly
the documented-versus-actual drift `gates-catch-installed-copy-drift`
exists to catch, one layer up. The shipped `.gitignore`
(`internal/scaffold/templates/gitignore`) gains the matching
root-anchored line, and with it the golden fixture
`internal/scaffold/testdata/golden/.gitignore` and this repository's own
`.gitignore:31-33` block — three places, named because the third is
appended rather than copied and so is *not* covered by that drift guard.

**It satisfies the reflective guard by classification, not by
exemption.** `TestPrimeTotalsAndNoProseScan`
(`internal/verb/prime_test.go:187-242`) walks the whole `primeOutput`
type graph via `stringJSONFields` and asserts the set of string-bearing
json tags is **exactly** a known map — INV-469's mechanism for keeping
"the only prose is `goal` and `title`" a checked property rather than an
observation. Any new string field trips it. This change adds `instance`
to that map with the same one-line justification `code` already carries
there: **twelve characters from the fixed alphabet `[0-9a-f]`, emitted by
the code that computes it, never written by a user** — an identifier by
construction, so §11.1's two-prose-fields rule holds unchanged. The
walker is not edited, the field is not exempted, and the guard is not
widened. That is the test this change must pass, named here so a
reviewer checks it rather than discovering it.

**§14 is not amended.** The first branch needed an exception carved into
it; this one does not. A one-way digest emits no path, no user name and
no host name, so §14's sentence stays literally true and untouched. That
is the single largest reason this branch was preferred, and it is worth
saying plainly: an amendment that has to weaken a privacy rule to fit is
an amendment whose shape is wrong.

## Why the basename branch was withdrawn

The first version of this proposal made the identity **the basename of
the tracker root's directory**. A critic round refuted it on four
independent grounds, all reproduced:

1. **It fits neither bucket of the reflective guard.**
   `TestPrimeTotalsAndNoProseScan` classifies every string field as prose
   (exactly two: `goal`, `title`) or as an identifier "chosen from a
   closed set by the code that emits it, never written by a user"
   (`prime_test.go:231-235`). A directory basename is **user-chosen free
   text** — `mkdir` accepts nearly anything — so it is neither. Adding it
   would either break INV-469 or force the deliberately narrow invariant
   wider, which is the guard failing at the exact job it was written for.
2. **It bypasses `validateText`.** Every other user-supplied string that
   reaches a dump row passes `internal/verb/pipeline.go:339-352`, which
   refuses any rune `< 0x20` or `0x7f` because "dump rows are one line
   each". A basename passes through nothing. A directory whose name
   contains a raw newline is a legal filesystem object, and `prime`'s
   human digest prints with a plain `fmt.Fprintf`
   (`internal/verb/prime.go:554`) and escapes nothing — so such a name
   injects an attacker-chosen extra line into the **first line** of
   session-start output, on the channel a SessionStart hook feeds
   straight into an agent's context.
3. **It carries no shape check and no bound**, on the exact channel task
   #73 measures a 195 KB flood on. A path component can be hundreds of
   bytes; nothing would have bounded it.
4. **It still fails the motivating case.** Two identically-named copies —
   `../copy/<same-name>` beside its source — report the same word. That
   was already stated as the basename's honest limit, and once the other
   three grounds landed there was nothing left to trade for it.

The digest answers all four: fixed alphabet and fixed length (1, 2, 3),
and different paths give different digests even when the directory names
match (4).

**A stored `meta` id.** A random token written once at `init` and carried
by `prime`. Rejected, and the reason is sharper than it first looks: `cp
-a` copies the database, so the copy carries the **source's** token and
reports itself as the source — strictly worse than either branch above.
Compounding it, `meta` rows are serialized into the tracked dump
(verified 2026-07-26: `INSERT INTO meta (key, value)` rows at
`.selftracked/dump.sql:324-328`), so the token would be published
anyway. Recorded because it is the branch a reader reaches for when the
opacity below bites.

**This is not in tension with the salt, and the difference is worth one
sentence** — a reader meeting both will otherwise see "a copied value is
fatal" beside "a copied value is fine". A stored token would BE the
identity, so `cp -a` carrying it makes the copy report itself as the
source: fatal. The salt is only *half* the input, so `cp -a` carrying it
changes nothing — the path half still differs, and the copy's digest
still differs from its source's. Copying the salt is harmless precisely
because the salt is not the identity.

## The limits, stated plainly

**It is opaque, it is retrospective, and it does not discharge #24 on its
own.** This field is not self-sufficient and must not be read as such.

It answers "is this the same tracker I saw before?" and not "which
project is this?". #24's incident needed the second question answered.
Sharper: **`prime` has no memory.** A lone digest at session start has
nothing to be compared against, because nothing persists the previous
one — so at the moment of reading, the field carries no information at
all. Its value is entirely *retrospective*: two sessions' logs, or two
task notes written from two repositories, become distinguishable **after
the fact**, once a reader has two digests in hand.

What discharges #24 is the **pair**. Orientation — the human-readable
"which repository am I in" — is
`resolution-names-the-root-it-found`'s job, where a
working-directory-relative path to the root names the place; this field
supplies the identity that path refers to. Either alone is half an
answer, and that is why the two are ratified together rather than
sequenced.

**It does not catch a silent mid-session write to the wrong tracker.**
Write verbs are unchanged. What this catches is the condition where an
agent forms its model of which repository it is in, which is session
start — the condition the incident's chain actually ran through.

**It is not stable under a move or a rename** of the repository
directory, and the instability is *invisible* — a changed digest looks
like a different tracker with no clue that the directory merely moved.
Inherent to a path-derived identity, and the price of distinguishing
copies.

**It is not stable across a fresh clone of the same repository on the
same machine**, because a new clone has no salt file (it is gitignored)
and generates a new one. Two working copies of one project therefore
report two different identities. This is stated rather than left to be
discovered, and it is **correct for the question the field answers**: the
question is "is this the same *working tracker* I primed on", and two
clones are two working trackers, with two databases that can and do
diverge. A reader looking for "same upstream project" is looking for
something this field deliberately does not provide — that fact is the
repository's remote, which git already carries.

## Why the digest is salted — the preimage oracle

This is the reasoning behind the salt, recorded because the unsalted form
was this proposal's second branch and was ratified out on 2026-07-26.

A hex digest of an absolute path emits no path, but it is **not zero
information**: it is a stable fingerprint whose preimage has low entropy.
A published `prime` output — and one will be published, the moment an
agent pastes it into a task note, which is precisely the behaviour §14's
rule exists to anticipate — lets anyone test a guess. An unsalted digest
therefore functions as an **oracle for whatever part of the path is not
already public**: enumerate candidates, hash each one, compare.

**The general case is what condemns it.** For this repository the middle
path segments are not obvious, so the search space is not trivial. But
selftracked installs into strangers' repositories, and the modal adopter
path is `/Users/<name>/<something-obvious>/<repo-name>` where the
repository name is *already public on their remote*. The unknown that
remains is typically the user's own name, and the digest confirms a guess
at it in one hash. A tool is judged on what it does in the general case,
and in the general case an unsalted path digest published beside a public
repository name is a user-name oracle. That is a **weaker** privacy
position than the withdrawn basename, which leaked the repository's own
directory name — already public — and nothing about the user.

The salt closes it completely, because the preimage now contains a value
no outsider can enumerate. What it does not do is weaken anything else:
the properties that made the path-derived digest win over the basename
and over content-derived identity — fixed alphabet, fixed length, no
control characters, no flood, copies distinguishable — are all properties
of the *output* and of the *path* half, and the salt changes neither.

Recorded rather than assumed because the salt looks like ceremony from
inside a repository whose own path is not guessable, and because the
unsalted branch is what a later reader will propose as a simplification.

## Consequences accepted with this change

`prime`'s stable JSON contract gains a field in first position. Every
consumer that compares payloads byte-for-byte changes with it, and
`TestPrimeTotalsAndNoProseScan` fails until the field is classified —
by design, and the fixtures and golden JSON move with it.

The digest is computed on every `prime`, including the read-only pass
that reports `dump_divergence`. It is one path resolution, one small file
read and one hash; no database is opened for it, and nothing about it
enters the database or the dump.

**`prime` acquires a write.** The salt is generated on first use, so the
first `prime` in a fresh working copy creates
`.selftracked/instance.salt`. That is a filesystem write from a read
verb, and §6.1's read-verb contract says a read verb's body runs
`query_only` with "no side effects on tracker state" — which this
respects in substance (no database, no dump, no `STATE.md`, nothing
tracked, nothing an adopter's `git status` will show) and stretches in
letter. Named rather than glossed, because "the read verb writes a file"
is precisely the kind of quiet exception a later reader is entitled to
find already argued. The alternative — generating the salt at `init` —
was not taken because it would leave every tracker created before this
change permanently without one, and the field would then be absent on
exactly the installed base that most needs it.

So **§6.1 gains the exception in its own text**, rather than leaving it
inferable from §11.1: a read verb writes nothing to tracker state, with
one named exception — `prime` creates `.selftracked/instance.salt` when
it is absent. An exception a reader must reconstruct from another section
is one the next author removes as a bug, and the coordinating agent added
this target after the proposal was otherwise finished, for that reason.

An omitted field means consumers must treat `instance` as optional, and
there are now two reasons it can be omitted: an unresolvable path (the
`chmod 000` ancestor the sibling proposal reproduces) and an unwritable
salt (a read-only checkout). Both are deliberate, and neither degrades to
an unsalted value.

## Relationship to the sibling amendment

`resolution-names-the-root-it-found` prints this identity in the
instance-resolution refusals alongside a working-directory-relative path.
The two are complementary by construction: this proposal supplies the
*which one*, that one supplies the *where*. Neither restates the other,
and the pairing is what makes the digest's opacity tolerable — which is
why they are ratified together, and why that proposal states what its
message reads if only one of the two is accepted.
