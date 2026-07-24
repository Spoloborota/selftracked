# Critic protocol — in-repo canon

This file is the protocol a fresh agent reads. Rules 1, 3, 4 and 6 are also
stated in the execution plan §5, which is their authority — on any divergence
there, the plan wins. Rules 2, 5, 7 and 8 exist only here: they are working
conventions, not plan clauses, and nothing overrides them. That asymmetry is
named because "the plan is the authority" would otherwise imply a fallback
that does not exist for four of these eight rules.

Skills and agent definitions reference this file and must not restate it.

1. **Strictly read-only.** A critic never edits or writes a file and never
   runs a state-changing git command (`checkout`, `add`, `stash`, `restore`,
   `reset`, `commit`, `mv`, `rm`). It observes and reports.

2. **Sandbox any execution.** A critic that must *run* something to verify a
   claim isolates it — a scratch directory outside the repository, a
   redirected `HOME`. Do not use git-worktree isolation when the critic must
   see uncommitted changes: those are usually the thing under review.

3. **Maximum flaws, zero solutions.** Enumerate defects with concrete
   evidence — `file:line`, an official document, an empirical run. Do not
   propose fixes, alternatives, or options. A critic that suggests a remedy
   has started defending it.

4. **Fresh context per critic.** A reviewer that shares the author's context
   inherits the author's blind spots. Each critic starts clean and is given
   its lens, its scope, and a timebox.

5. **Distinct, non-overlapping lenses, run in parallel.** Then collect →
   adjudicate → fix → optionally repeat until findings converge.

6. **The coordinating agent adjudicates by default — never the critics.**
   Findings are judged refute-by-default. Two classes always escalate to the
   owner: an accepted deviation from the spec or the plan, and any privacy-
   or security-class finding. Three rounds without convergence escalates.

7. **A clean report is not proof of absence.** State what was checked and
   what was not. A reviewer that returns nothing has either found nothing or
   looked at the wrong thing, and the report must let the reader tell which.

8. **A standing security-and-leak lens rides every round.** Beyond the
   round's task-specific lenses, the security-and-publication lens is applied
   every time — by a dedicated critic when the round fans out, by the
   coordinating agent on a small change — proportional to the change and
   mandatory whenever the work touches code that runs a subprocess, parses or
   loads untrusted input, reads or writes the filesystem, handles paths or
   symlinks, or alters the published surface. Two facets, both in scope:
   (a) code vulnerabilities — injection (SQL, argument, command), path
   traversal and symlink escape (CWE-59), unsafe writes, unbounded reads,
   missing input validation; (b) publication leaks — secrets and credentials,
   personal or machine-identifying data, author de-anonymization signals, the
   pilot client's identity, and non-English text in a tracked file. It rides
   both plan-critique and implementation-critique. Why standing: this
   repository is public and largely agent-authored, and these are the classes
   a first-pass author most reliably misses — the project shipped several
   before a review caught them.

Why this is written down rather than assumed: a review that proposes its own
fixes stops being adversarial, and an adjudicating critic marks its own
homework. Both failures are invisible in the output — the report still looks
like a report.
