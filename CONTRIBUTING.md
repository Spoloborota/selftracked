# Contributing to selftracked

## The one rule above the others

`docs/v0-spec.md` is authoritative. Any change that deviates from it
travels as a change proposal under `openspec/changes/<name>/` and is
reviewed **before** the deviating code merges — never as a code comment
or a silent choice. The repository's own working contract is generated
into `PROMPT.md`; read it first.

## Developer Certificate of Origin

Contributions are accepted under the
[Developer Certificate of Origin 1.1](https://developercertificate.org/).
Sign off every commit (`git commit -s`); the sign-off certifies you have
the right to submit the work under this repository's license
(Apache-2.0). Unsigned commits are not merged.

## AI-contribution clause

AI-assisted and AI-authored contributions are welcome and are, in fact,
this project's normal mode. The conditions:

- **A human (or the submitting agent's operator) takes DCO
  responsibility.** The sign-off means someone with the right to submit
  the work stands behind it; "a model wrote it" transfers nothing.
- **Provenance is disclosed** in the commit trailer (this repository
  uses `Co-Authored-By:` for the model) — honest history beats tidy
  history.
- **Verification is not delegated to the author.** "Done means a
  command exited 0": `make gates` green locally is the floor, and an
  agent's assertion that something works is not evidence — the
  reviewer re-runs the commands.
- Do not submit content whose license or origin you cannot certify —
  that is a DCO violation regardless of who or what produced it.

## The mechanics

- `make gates` runs everything a change needs to pass: build, vet,
  tests (race detector on), lint, modern-idiom check, vulnerability
  scan, pin checks, the installed-copy drift check (this repository's
  generated documents against the templates it ships), and the working
  binaries.
- Tests live next to what they test; e2e scenarios are testscript
  `.txtar` files under `internal/cli/testdata/`.
- Documentation, code, comments and commit messages are English.
- Commit locally and freely; publishing is a deliberate, separate act.
