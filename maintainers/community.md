# Community

This doc covers the soft stuff — how we interact with contributors in Discussions, how we keep the vibe right, and what to do when the vibe goes sideways.

## Principles

- **Default warm.** Assume good intent. Most of the time you'll be right; in the cases you aren't, you'll have set a better tone for the onlookers.
- **Default public.** If a conversation started in public and nothing sensitive came up, keep it there. Private DMs should be the exception.
- **Default concise.** A two-line answer that solves the question is better than a five-paragraph essay that shows off.

## Discussions

We use [GitHub Discussions](https://github.com/sightmap/sightmap/discussions) for:

- **Q&A** — how-to questions
- **Ideas** — pre-SEP brainstorming
- **Show and tell** — sightmaps people built, tools people wrote
- **Announcements** — maintainer-posted only; spec changes, releases, events

### Cadence expectations

Not a strict SLA. What we aim for:

- Top-level Q&A questions get an initial response within a few days
- Ideas get at least one maintainer reaction (🚀 / 🤔 / 👎) within a week so the author knows it's been seen
- Announcements are proactively posted by maintainers when something noteworthy ships

If a Discussion is genuinely useful but hasn't caught traction, a maintainer can pin it or cross-link from the issue tracker.

### Marking answers

For Q&A, mark a reply as the answer when the question is resolved. This does two things:

- Helps future askers find the answer via search
- Gives a clean visual signal that the thread is closed

If the answer is "we don't know, we're thinking about it," say so and label the Discussion with `open-question` (or the nearest existing thing).

## Moderation

Moderation tools are for unambiguous Code of Conduct violations or clear spam. Everything else — terse tone, unproductive argument, off-topic drift — is handled with words first, tools second.

Escalation ladder:

1. **Gentle redirect.** "Let's keep this focused on X" or "This is drifting — can we move it to a new thread?"
2. **Direct ask.** "Please rephrase — the tone isn't constructive."
3. **Lock the thread.** If the conversation is generating more heat than light and the useful points have been made, lock with a comment explaining why.
4. **Moderator action.** Hide, delete, or (rarely) ban per the [Code of Conduct](https://github.com/sightmap/.github/blob/main/CODE_OF_CONDUCT.md).

Always prefer lock over delete. Deleting makes it look like we're hiding something; locking tells the story.

When you take a moderation action, note it briefly in a maintainer-sync thread so other maintainers aren't surprised.

## First-time contributors

We want more of them. Small things that help:

- Respond to first-PR authors within 3 business days even if the review will take longer. "Thanks — will review this week" buys you the week.
- When pointing out a fix, pick one change at a time. A 12-comment review on a 30-line PR is a way to scare someone off.
- If their first PR is merged, say thanks in the merge comment. Named thanks in the release notes costs nothing and matters to people.

## Recognition

We don't have a formal recognition program. What we do:

- Mention substantive contributors by name in GitHub Release notes
- Feature community-built SDKs and tools on sightmap.org once they're ready
- Invite sustained, high-quality contributors to become maintainers (see [`GOVERNANCE.md`](https://github.com/sightmap/.github/blob/main/GOVERNANCE.md#maintainer))

If a formal program starts making sense (swag, yearly thank-you, whatever), we'll add it.

## Office hours / community calls

None yet. If the Discussions volume grows to a point where async isn't keeping up, we'll start a monthly community call. Not before — an empty call is worse than no call.

## Social presence

The project doesn't have its own social accounts. Subtext may post about Sightmap from its own channels, but Sightmap-as-project speaks through:

- This repo (commits, PRs, issues, Discussions, Releases)
- sightmap.org
- Official SDK repos when they exist

If that changes, it'll be proposed in an SEP-like process so it's not ad-hoc.

## Bad days

If you're having a rough day and a contributor is being frustrating, close the tab. Come back later. We have no SLA that requires you to reply angry. Hand the thread off to another maintainer if you need to.

The inverse is also true: if another maintainer seems to be having a day, it's okay to quietly pick up the thread they're stuck on.
