# Maintainer playbooks

These are the operational playbooks for maintaining the Sightmap project. They are written for the current maintainer team but are public so that contributors can see how the sausage gets made and so that new maintainers can onboard quickly.

Nothing in here overrides [`GOVERNANCE.md`](https://github.com/sightmap/.github/blob/main/GOVERNANCE.md). When this directory and GOVERNANCE conflict, GOVERNANCE wins.

## Index

- [`triage.md`](triage.md) — what to do with a new issue or PR
- [`reviewing-prs.md`](reviewing-prs.md) — the review bar for different PR types
- [`releasing.md`](releasing.md) — how to cut spec, CLI, and website releases
- [`spec-evolution.md`](spec-evolution.md) — shepherding an SEP from Draft to Final
- [`community.md`](community.md) — Discussions moderation, cadence, tone

Planning notes that are not yet a Discussion, SEP, or prototype live under
[`research/`](../research/). They are not normative.

## Principles

Four rules to return to when the playbooks don't cover something:

1. **Respond, even if the answer is "not now."** A contributor waiting two weeks for any signal is our worst outcome.
2. **Write decisions down.** If you decided something in Slack or DM, it's not decided. Bring it to a PR, Discussion, or SEP.
3. **Optimize for the second-time contributor.** The first-time PR is a win; the fifth-time contributor is the signal we built a healthy project.
4. **The spec is a shared artifact, not ours.** The moment we start treating it as internal, we've failed at open source.

## Tools we use

- **GitHub Issues** — bugs, SDK feedback, spec proposals (pre-SEP)
- **GitHub Discussions** — questions, ideas, announcements, show-and-tell
- **GitHub PRs** — everything that changes a file
- **Labels** — see [Labels](#labels) below for the canonical list

## Labels

We keep the label set small on purpose. Every label should answer a question a maintainer would actually ask when filtering.

| Label | Meaning |
|---|---|
| `bug` | Something behaves incorrectly per the spec or the code |
| `spec` | Touches the formal spec (schema, semantics, matching) |
| `cli` | Touches the Go implementation / CLI |
| `proposal` | Informal pre-SEP idea |
| `sep` | Linked to an accepted or in-review SEP |
| `docs` | Documentation site, README, examples |
| `website` | Marketing-site-only changes |
| `sdk-feedback` | From a tool or SDK author — carries cross-impl weight |
| `good first issue` | Self-contained, small, well-scoped |
| `help wanted` | We'd take a PR; we're not going to get to it soon |
| `triage` | Auto-applied on new issues; remove once triaged |
| `stale` | Managed by the stale workflow |
| `blocked` | Waiting on an upstream decision (usually an SEP) |
| `wontfix` | Closing with explanation; keep for historical record |
| `security` | Do not use on public issues for unreported vulns; this is for post-disclosure tracking only |
| `needs-info` | Reporter or PR author owes us a clarification; auto-stale-exempted while it sits |
| `spec-change` | Issue or PR will affect spec semantics; subscribe if you maintain a downstream tool |
| `pinned` | Stale workflow exempts pinned items so we can keep important threads visible |
| `work-in-progress` | PR is intentionally not ready for review; stale workflow exempts it |

If you feel the urge to add a new label, propose it in a maintainer-sync Discussion first. Labels proliferate and then stop being useful.

## Escalation

If any maintainer is unsure about a decision:

- **Technical**: bring it to a maintainer Discussion thread, tag the other maintainers.
- **Conduct**: see [`CODE_OF_CONDUCT.md`](https://github.com/sightmap/.github/blob/main/CODE_OF_CONDUCT.md) — report to subtext@fullstory.com.
- **Security**: see [`SECURITY.md`](../SECURITY.md).
- **Legal / trademark / licensing**: do not answer ad-hoc. Escalate to the steward (Subtext leadership).
