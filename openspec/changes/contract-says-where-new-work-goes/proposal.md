# Change: the generated contract says where new work goes

Target: `docs/v0-spec.md` §11.3 (the skill's working loop); revision 3.32
→ 3.33. `internal/scaffold/templates/PROMPT.md` — a new section;
`internal/scaffold/templates/claude_skill.md` — one loop branch, and the
existing drift rule reconciled with it; this repository's own `PROMPT.md`
and `.claude/skills/selftracked/SKILL.md`, which are byte-identical
copies of those templates. Content tests. No schema change, no verb
change.
Status: **accepted** · raised 2026-07-25 by task #55 under
epic `tracking-integrity`, story S1 · review tier **FULL** (plan §5,
D-EP7) · ratified by the owner 2026-07-25 · applied to the spec the same
day

## Why

The generated contract documents both surfaces — the task verbs, the
story verbs, the one-IN-PROGRESS-story-per-epic constraint — and never
the choice between them. An agent holding work that is not in the backlog
has no written rule for whether it belongs as a standalone task or as a
story under the active epic. Observed as an explicit guess during a
fresh-install exercise (#55).

The shipped skill has the same hole from the other side: its loop is
`prime` → refine → **pick from `ready[]`** → execute
(`internal/scaffold/templates/claude_skill.md:18-22`). There is no branch
for work that did not come from `ready[]`, which is how most work
arrives — an owner's request, a defect found mid-task, a follow-up. The
loop's silence is what sends that work off-book.

#61 names this as one of its two prerequisites: without a written rule
for task-versus-story, every option an agent could offer the owner is
underspecified.

## What changes

**`PROMPT.md` gains a section, "Where new work goes"**, stating the rule
in three lines:

- Work that advances an **ACTIVE epic's goal** is a **story** under that
  epic. Opening one is a **scope change**: the owner authorizes it, the
  implementing agent never authorizes its own.
- Work that does not advance an active epic's goal — a defect, a
  question, a follow-up, anything standalone — is a **task**
  (`create --title …`), homed to an epic with `--epic` only when it
  genuinely belongs to one.
- Work matching **no existing task, story or epic**: say so before the
  first write and put the choice to the owner rather than working
  off-book. `prime` names the epic-scoped case as a
  `no-workable-story` notice (§11.1); the rest is the agent's own
  judgement in v0.

**The skill's loop gains one branch** at step 3: work that did not come
from `ready[]` is classified by that rule before any write, and the story
branch stops for the owner.

**The skill's existing drift rule is reconciled with it.** The skill
already ships an unconditional rule three lines below the loop — "A new
idea while working is `create` + park, one command — capture it, do not
pivot to it" (`claude_skill.md:31-34`) — with no story branch at all.
Left as they are, the two rules answer the same trigger ("new work
appears mid-session") differently. They are made to answer different
questions explicitly: the drift rule governs work discovered **while a
story is in progress** (capture it, do not pivot, whatever its size); the
new branch governs work being **taken up now**, when no story holds it.
Each names the other.

**This repository's own copies are updated in the same change.** Its
`PROMPT.md`, `.claude/skills/selftracked/SKILL.md` and
`.claude/rules/selftracked.md` are currently byte-identical to their
templates (verified by `diff`), and `init` never overwrites an existing
generated document (`internal/scaffold/scaffold.go:270-272`), so nothing
propagates a template edit to them automatically. #55 was filed because
*this repository's* agent had no written rule and guessed; a change that
edits only the templates would close the task without fixing the
repository that raised it.

**Spec**: §11.3's loop description gains the branch, the pointer to the
classification rule, and the drift rule's scoping; the rule text itself
lives in the generated contract, which §11.3 already governs.

## What this deliberately does NOT do

It does not implement #61's interactive protocol — no verb refuses,
prompts, or blocks on the classification, and nothing checks that the
agent obeyed. This is written guidance in a generated file, of the same
class as the durable-doc authoring rules `PROMPT.md` already carries and
labels as "authoring guidance in this generated file, not gated
conventions". The owner's #61 decision put the enforced protocol in v0.1;
this change is the prerequisite it names, and the honest description of
its own force is part of the text.

## Relationship to the sibling amendment

`worklog-refusals-name-the-routes` applies this same rule at the refusal
surface. The rule is stated **here**, once; that amendment's refusals
name the routes without restating the classification, so the two cannot
drift into two different rules.

## Consequences accepted with this change

Every adopter's next `init` installs the section; existing installs do
not update themselves. A repository adopted before this change keeps the
old contract until its owner copies the new one. That gap is inherent to
the scaffold model and is not introduced here; it is named because this
is the first amendment whose whole payload is generated text — and it is
why this repository's own copies are inside the change's scope rather
than assumed to follow.

The three-line rule will not decide every case — "advances the epic's
goal" is a judgement, and a wrong call is cheap to correct with
`edit --epic`/`--detach`. The rule exists to stop the *silent* guess,
not to remove judgement.
