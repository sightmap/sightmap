# Reviewing PRs

Not every PR needs the same review. This doc sets expectations for what each kind of PR needs.

## The three bars

### Bar 1 — trivial

Typos, wording polish, dependency bumps that only touch a lockfile, a new example that doesn't introduce new spec usage, CSS nudges.

- **Approvals required**: 1
- **CI required**: the relevant checks pass
- **Response time**: same day if possible, within 3 business days always
- **Who merges**: the approver

### Bar 2 — standard

Website features, new sections, new examples that exercise spec features, CLI features and bug fixes, refactors, docs that change meaning (not just wording), CI changes, playbook edits.

- **Approvals required**: 1 (2 recommended for anything touching the spec prose or schema file)
- **CI required**: all jobs pass
- **Response time**: first review within 3 business days
- **Who merges**: the approver; the author may merge after approval

### Bar 3 — spec change

Anything that changes the semantics of the spec. Must reference an accepted SEP. Includes changes to `sightmap.schema.json`, `spec/v1/schema.md` (non-clarification edits), route-matching behavior, file-merge behavior, new top-level fields.

- **Approvals required**: 2 maintainers, plus steward sign-off if the SEP is flagged as breaking
- **CI required**: all jobs pass; all examples and conformance fixtures updated; schema file and prose in sync
- **Response time**: first response within 3 business days; full review may take longer
- **Who merges**: second approver, and only after verifying the SEP is `Accepted`

If a PR claims to implement an SEP but the linked SEP is still `Draft` or `Review`, push back — don't merge implementation ahead of acceptance.

## What to look for (general)

Order of operations in a review, roughly:

1. **Does this match the described intent?** Read the PR description first. If the description and the diff disagree, the description is the problem — ask for clarification.
2. **Is it the right scope?** One concern per PR. If there's creep, say so kindly and ask for a split.
3. **Is the interface right?** For spec changes, this is the whole game. For code, check that public-facing names read well.
4. **Is the implementation right?** Correctness, edge cases, tests, typecheck.
5. **Are docs updated in the same PR?** Especially for spec/schema changes.
6. **Is it nice to read?** Not nitpicky about style — but code and docs read dozens of times more than they're written.

## What to look for (spec changes specifically)

- Does the JSON Schema match the prose in `spec/v1/schema.md`?
- Do all examples still validate, and do the conformance fixtures still pass?
- Does the SEP cover this change? If the PR drifts from the SEP, that's a problem with the PR, not the SEP.
- Any cross-implementation impact called out? If yes, is there an announcement plan?

## Review comment style

- **Be specific.** "This could be clearer" is not useful. "What does `source` mean when two components share a selector?" is.
- **Distinguish must-fix from nice-to-have.** Use prefixes: `blocking:` for things that stop the merge, `nit:` for taste, `q:` for genuine questions. No prefix = suggestion the author can take or leave.
- **Don't review the person.** Review the change.
- **Suggest, don't demand.** "What about…" works better than "Do this."
- **Approve when it's good enough, not when it's perfect.** Perfect in review is a way to never ship anything.

## When you're blocked on an author

If a PR has open comments and the author has gone quiet:

1. After 2 weeks, ping politely once in the thread.
2. After 4 weeks of no response, add the `stale` label manually (the workflow will eventually do this too).
3. After 6 weeks total, close the PR with a comment explaining they can reopen when ready.

This is not personal. People have lives. The goal is to keep the queue honest.

## When you're the one merging

- Use **Squash and merge** by default. The commit history on main stays clean; the PR still has the full review trail.
- Use **Rebase and merge** only when the author set up multiple commits deliberately and it's useful to preserve them (rare).
- Never use the merge commit style. It clutters the graph.
- Check the squash commit message before hitting the button. GitHub's default is often bad.

## Authoring your own PR as a maintainer

Same bars. Same expectations. Get review from another maintainer; don't self-approve. The only exception is absolutely trivial doc fixes (Bar 1, typo-only) which a maintainer may self-merge — and even then, leave a short rationale in the PR description.
