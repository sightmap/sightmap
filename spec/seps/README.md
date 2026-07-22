# Sightmap Enhancement Proposals (SEPs)

> Inspired by Python's PEPs and the MCP SEP process.

A **Sightmap Enhancement Proposal** is a design document that proposes a change to the Sightmap spec, the SEP process itself, or the surrounding ecosystem in a way that needs maintainer-level alignment before implementation.

This directory holds those proposals.

## Why we need this

The Sightmap spec is consumed by multiple SDKs, validators, and runtime tools. A change to a field name or matching rule can ripple across every implementation. SEPs exist to:

- Force the design to be written down before code is shipped
- Give the community a single thing to react to instead of scattered comments
- Create a durable record of *why* the spec looks the way it does

If the change you want to make is small, contained, and doesn't change spec semantics — just open a PR. SEPs are for the cases where someone reading the diff six months later would ask "wait, why did we do this?"

## When you need an SEP

You **need** an SEP for:

- Adding, removing, or renaming any field at any level of the schema
- Changing the type or semantics of an existing field
- Changing route-matching rules (`*`, `**`, `:param`)
- Changing file discovery or merge rules under `.sightmap/`
- Introducing a new top-level concept (peers to `views`, `components`, `requests`, `memory`)
- Bumping the spec to a new major version
- Changes to this SEP process or to [`GOVERNANCE.md`](https://github.com/sightmap/.github/blob/main/GOVERNANCE.md)

You **don't** need an SEP for:

- Wording fixes, typo fixes, clarification of existing behavior
- New examples
- Website changes
- Tightening the JSON Schema to reject input the prose already declared invalid
- Adding a new optional field that an SDK can ignore safely (gray area — when in doubt, ask in a Discussion)

## Lifecycle

Each SEP moves through these states:

```
Draft  →  Review  →  Accepted   →  Final
                  ↘  Rejected
                  ↘  Withdrawn
                  ↘  Deferred
```

- **Draft** — PR is open, author is iterating, not yet ready for broad feedback.
- **Review** — author has marked it ready. Maintainers and community comment. Minimum review window: 7 days for minor changes, 14 days for anything affecting the JSON Schema or existing sightmaps.
- **Accepted** — maintainers have reached consensus to adopt. Implementation PRs follow.
- **Final** — implementation has shipped in a released version of the spec.
- **Rejected** — maintainers have decided not to adopt. The SEP stays in the directory as a record of the decision and the reasons.
- **Withdrawn** — the author has chosen to stop pursuing it.
- **Deferred** — the idea is good but not now. Often promoted back to Review later.

A merged SEP file does not mean implementation is done. The SEP records *the decision*; implementation is tracked by linked issues and PRs.

## Process

The filename pattern is normative — see [`CONVENTIONS.md`](../CONVENTIONS.md) for the slug regex and validation.

1. **Float the idea first.** Open a [Discussion](https://github.com/sightmap/sightmap/discussions) or a [spec proposal issue](https://github.com/sightmap/sightmap/issues/new?template=spec_proposal.yml). Get rough feedback before investing in writing.
2. **Claim a number.** Open a draft PR adding `seps/NNNN-short-slug.md`, where `NNNN` is the next unused 4-digit integer. *"Unused"* means not on `main` **and** not claimed by an open PR — run `gh pr list --repo sightmap/sightmap --state open --search "in:title SEP-"` (or skim `gh pr list --state open`) before picking. If your SEP introduces conformance fixtures, the same reservation rule applies to fixture numbers (see [`conformance/README.md`](../conformance/README.md)) — open SEP PRs reserve their associated `NNN` fixture range too. If two PRs collide on a number, the second one renames.
3. **Write the SEP** using [`0000-template.md`](0000-template.md) as a starting point.
4. **Mark it Review** when you're ready for substantive feedback. Update the `status` in the front-matter and request review from maintainers.
5. **Iterate** based on feedback. Most SEPs go through several rounds.
6. **Decision.** Maintainers decide per [`GOVERNANCE.md`](https://github.com/sightmap/.github/blob/main/GOVERNANCE.md):
   - At least two explicit maintainer approvals
   - No unresolved blocking objections
   - Steward sign-off for breaking changes
7. **Merge** with the agreed status (`Accepted`, `Rejected`, `Deferred`, etc.). The PR description should summarize the discussion and the rationale.
8. **Implement.** Follow-up PRs reference the SEP number in their description and update the SEP's status to `Final` when implementation lands in a released spec version.

## Authoring tips

- **Lead with the problem.** Spend the first paragraph on *who hits this and what they can't do today*.
- **Show, don't tell.** A concrete YAML example of the proposed shape is worth more than a paragraph of prose.
- **Name the alternatives.** What did you consider and rule out, and why? "I picked the obvious thing" is rarely true and never satisfying to read.
- **Be honest about cost.** Migration burden, SDK churn, ambiguity introduced — call it out. The SEP that pretends to be free is the one that gets pushed back hardest.
- **Stay small.** One SEP, one decision. If your proposal contains "and also…", split it.

## Acceptance criteria

A maintainer evaluating an SEP for acceptance should be able to answer yes to all of these:

- [ ] The problem is clear and worth solving
- [ ] The proposed shape is unambiguous; an SDK author could implement from this document
- [ ] If the SEP touches `spec/v1/sightmap.schema.json`, the Shape section includes prose enumerating the `$defs` changes (not just YAML examples)
- [ ] If the SEP resolves a bullet in `spec/v1/schema.md`'s "Open questions" section, the bullet is narrowed or removed in the same PR
- [ ] Alternatives were considered and the trade-offs are clear
- [ ] Migration impact on existing sightmaps is described
- [ ] Conformance impact on existing SDKs is described
- [ ] If breaking, the SEP justifies the break and proposes a deprecation path

## Example numbering

```
seps/
├── README.md
├── 0000-template.md       ← never assigned to a real proposal
├── 0001-…                 ← first real proposal
├── 0002-…
└── …
```

## Inspirations

- [PEP 1 — PEP Purpose and Guidelines](https://peps.python.org/pep-0001/)
- [MCP SEPs](https://github.com/modelcontextprotocol/specification)
- [TC39 process](https://tc39.es/process-document/)

We've intentionally kept this lighter than any of those. The process can grow if the project does.
