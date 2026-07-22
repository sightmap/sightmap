# Triage

The goal of triage: every new issue and PR gets an acknowledgment within **3 business days** and ends up in one of a small number of known states.

We do not triage to solve things. We triage to route them.

## Triage cadence

No formal rotation yet. In practice:

- Every maintainer glances at the [triage queue](https://github.com/sightmap/sightmap/issues?q=is%3Aissue+is%3Aopen+label%3Atriage) at least once a week
- If you open Sightmap on GitHub and see something with a `triage` label older than 3 business days, you own it for the moment
- If you're going OOO for more than a week, say so in the maintainer channel

If we ever need a formal rotation, that's a signal the project has grown and we should set one up.

## New issue: what to do

1. **Read it fully.** Seriously — half the triage mistakes are from skimming.
2. **Check for duplicates.** Search open and closed issues. Link and close if duplicate.
3. **Decide the area.** Remove `triage`. Add one of: `bug`, `spec`, `cli`, `proposal`, `docs`, `website`, `sdk-feedback`.
4. **Check quality of reporting.** If the report is missing repro / version / context and the author hasn't been guided, leave a short, friendly comment asking for the missing bits. Use `needs-info` (add it if we don't have it yet).
5. **Decide scope.** Is this a paper cut a first-timer could fix? Add `good first issue`. Is it something we'd take but won't get to? Add `help wanted`. Blocked on an SEP? Add `blocked` and link the SEP.
6. **Respond.** Even one sentence — "Thanks, we'll take a look" — is better than silence. Silence communicates "we don't care."

## New PR: what to do

1. **Check CI.** If it's red, leave a one-line comment pointing at the failing job.
2. **Check scope.** Is this one concern, or did it balloon? If ballooned, ask politely to split.
3. **Check the PR description.** If the "why" is missing, ask for it before reviewing.
4. **Route by type.** See [`reviewing-prs.md`](reviewing-prs.md) for the review bar.
5. **Assign yourself or tag someone.** A PR with no assignee is a PR nobody thinks is theirs.

## What not to triage

- **Questions filed as issues.** Redirect to Discussions, close with a friendly pointer.
- **Subtext product complaints.** Redirect to subtext.fullstory.com support channel, close.
- **Angry bug reports with no repro.** Ask for a minimal repro once. If no response in 14 days, close with `needs-info` still attached. Reopen if they come back.

## Closing politely

Closing an issue is not a rejection of the person. Close with:

- A sentence explaining why
- A link to where they can continue (Discussion, docs, another repo)
- Thanks for filing, even if you disagreed

Never close with a thumbs-down reaction as the only signal.

## Edge cases

### "I'll write an SDK for language X"

Response: "Great — you don't need permission. See [`SUPPORT.md`](../SUPPORT.md). Open a PR to list it on sightmap.org when it's ready, and subscribe to issues labeled `spec-change` if you want early warning on breaking changes." Close.

### "Can Subtext maintain an SDK for me?"

Response: "We don't have the bandwidth to take on additional maintenance today, but we'd happily steward community-owned SDKs — happy to cross-link, review PRs, and coordinate on spec changes." Close or move to Discussion.

### Someone opens an issue clearly aimed at a different project

Response: explain the confusion, link to the right place, close.

### Issue that's really a security report

Close immediately with a short comment pointing at [`SECURITY.md`](../SECURITY.md). Delete the issue only if it contains genuinely sensitive detail; otherwise closed-and-locked is fine.
