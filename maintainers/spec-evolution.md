# Shepherding SEPs

This is the internal-facing companion to [`spec/seps/README.md`](../spec/seps/README.md). If `spec/seps/README.md` is the process, this is the playbook for maintainers who are helping move a proposal through it.

## Why maintainers need a playbook here

SEPs are the single highest-leverage, highest-risk artifact in this project. A rushed SEP produces a spec field we'll regret for years. A stalled SEP burns out a contributor who put real thought into it. Shepherding well means:

- The proposal gets honest feedback fast
- The author isn't guessing what "good enough" looks like
- The decision, when made, is clearly documented and defensible a year later

## When an SEP PR lands

Within **3 business days**:

1. Acknowledge the SEP on the PR. One sentence is fine: "Thanks — reading through now."
2. Assign yourself as the **shepherd** (leave a comment saying so). One maintainer per SEP, not all of them. The shepherd's job is to drive it to a decision, not to pre-approve it.
3. Skim for SEP hygiene: template followed, status set to Draft or Review, numbered, named sensibly. If hygiene is off, comment with what to fix — don't fix it yourself.
4. Skim for "is this an SEP?" If it's really a small wording fix that got dressed up as an SEP, say so kindly and redirect to a plain PR.
5. Add the `sep` label.

## As shepherd

Your jobs:

- **Be the single point of contact for the author.** They should know who is driving this, and it should be one person.
- **Pull other maintainers in deliberately.** Don't assume people read every SEP thread. Tag specific maintainers when their expertise is needed, not "all hands" every time.
- **Keep the conversation moving.** If the thread has gone quiet for a week, say something. Either push the author ("what do you need from us?") or push maintainers ("I think we're ready for a decision").
- **Write the decision memo.** When maintainers converge, write a short comment summarizing the decision and the reasoning, even if the reasoning is just "we agree with the SEP as written." This is what people will read a year from now.

## Decision mechanics

Per [`GOVERNANCE.md`](https://github.com/sightmap/.github/blob/main/GOVERNANCE.md):

- At least **two maintainer approvals** before acceptance
- No unresolved blocking objections
- For breaking changes or version bumps, **steward sign-off** in addition

An objection is "blocking" if a maintainer says so explicitly. Disagreement without a formal block is still a signal but doesn't stop acceptance. If two maintainers approve and one is still not happy but didn't block, acknowledge it in the decision memo so the dissent is recorded.

## How to give SEP feedback well

- **Lead with the interface, not the impl.** "What happens when a user does X?" beats "We'd have to change the parser."
- **Ask for examples.** If the SEP is abstract, request a concrete YAML that demonstrates the shape. You'll catch design bugs faster.
- **Play it against existing sightmaps.** Pull a couple of real examples and mentally apply the proposal. Does it break anything? Does it get awkward?
- **Be explicit about what would convince you.** "I'd accept this if…" is more useful to the author than "I'm not sure."
- **Don't design by comment thread.** If you're asking for structural changes, suggest those as a separate Discussion or a round 2. A 40-comment thread of piecemeal redesign is a worse SEP than a clean revision.

## When to say no

Rejection is a real outcome. It's not failure — a clean "no, because X" is better than a deferred-forever yes.

Good reasons to reject:

- The problem isn't big enough to justify the spec change
- The proposal solves the problem but introduces more complexity than it removes
- The proposal conflicts with the direction of another accepted SEP
- The design space isn't ready — we need more real-world data first (that's Deferred, not Rejected)

Bad reasons to reject:

- The author isn't a maintainer
- "We were about to do this ourselves" (if you were, you should have and it's not their fault you didn't)
- You'd personally do it differently but can't articulate why their way is wrong

When rejecting, write the rejection memo in the PR. Close the PR but leave the file merged if there's meaningful design record in it — a merged Rejected SEP is useful history.

## After acceptance

1. Update the SEP status to `Accepted`.
2. Create one or more implementation issues, linked to the SEP.
3. Cross-link: SEP PR → implementation issues, implementation issues → SEP.
4. When implementation PRs land, update status to `Final`.
5. Announce in Discussions if the change is user-visible.

## After rejection

1. Update status to `Rejected`.
2. Leave the decision memo as the final comment.
3. Thank the author for their work. Specifically, not generically.
4. Lock the PR conversation if rehashing is likely to generate heat.

## Handling contentious SEPs

Some proposals will attract strong opinions from the community. When that happens:

- Keep the thread on the PR itself, not spread across three Discussions
- Summarize the arguments periodically so new readers can catch up
- Don't let a loud minority drive the decision — but don't dismiss them either
- If maintainers themselves are split, take it to a maintainer-sync Discussion thread before deciding publicly
- Final call rests with the steward per [`GOVERNANCE.md`](https://github.com/sightmap/.github/blob/main/GOVERNANCE.md). Use it sparingly.

## Handling AI-generated SEPs

It is fine for an SEP to have been drafted with AI assistance. It is **not** fine for an SEP author to not understand what they submitted. If a shepherd can't get coherent answers to questions about the proposal, that's a signal to defer.

Review the content, not the provenance.
