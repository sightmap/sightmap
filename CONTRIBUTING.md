# Contributing to Sightmap

Thanks for your interest in improving Sightmap. This project is open source and we welcome contributions from anyone — not just the Subtext team.

This document is the entry point. For deeper material see:

- [`GOVERNANCE.md`](https://github.com/sightmap/.github/blob/main/GOVERNANCE.md) — how decisions are made (org-wide)
- [`MAINTAINERS.md`](MAINTAINERS.md) — who the maintainers are
- [`CODE_OF_CONDUCT.md`](https://github.com/sightmap/.github/blob/main/CODE_OF_CONDUCT.md) — community standards (org-wide)
- [`SECURITY.md`](SECURITY.md) — how to report security issues
- [`spec/VERSIONING.md`](spec/VERSIONING.md) — how the spec is versioned
- [`spec/seps/README.md`](spec/seps/README.md) — the Sightmap Enhancement Proposal process
- [`maintainers/`](maintainers/) — maintainer playbooks

## What lives in this repo

The whole open Sightmap project, in one place:

1. **The spec** — [`spec/`](spec/) contains the canonical specification, JSON Schema, examples, the SEP process, and conformance fixtures. This is the source of truth.
2. **The reference implementation** — [`go/`](go/) is the Go library and `sightmap` CLI (published to npm as `@sightmap/sightmap`).
3. **The websites** — [`docs/`](docs/) renders [docs.sightmap.org](https://docs.sightmap.org) (Mintlify); [`web/`](web/) renders [sightmap.org](https://sightmap.org) (React + Vite).

Community SDK ports in other languages may live in separate repositories under the [`sightmap` GitHub organization](https://github.com/sightmap); they implement the spec defined here.

## Ways to contribute

### Ask a question

Open a [GitHub Discussion](https://github.com/sightmap/sightmap/discussions). Questions are not bugs. Please don't file them as issues.

### Report a bug

Open an issue using the **Bug report** template. For a website, include the URL and steps to reproduce. For the spec, include the relevant YAML snippet and what you expected. For the CLI, include the command and output.

### Propose a small change to docs, a website, or the tooling

Open a PR directly. Small = typo fixes, wording polish, a missing example, a clarifying table, a contained bug fix. Anything that doesn't change what the spec *means*.

### Propose a change to the spec itself

This is the important one. Changes to the spec go through the **SEP process** (Sightmap Enhancement Proposal). The short version:

1. Open a [Discussion](https://github.com/sightmap/sightmap/discussions) to float the idea.
2. If it gains traction, open a PR that adds a new SEP document under `spec/seps/`. Use [`spec/seps/0000-template.md`](spec/seps/0000-template.md) as a starting point.
3. Maintainers review, discuss, and accept or decline. Accepted SEPs become the basis for a schema change in a subsequent PR.

Full process: [`spec/seps/README.md`](spec/seps/README.md).

We use SEPs because the spec is consumed by multiple tools. A drive-by PR that changes a field name or adds a new top-level key can break everyone downstream. SEPs force us to think it through first.

**AI-assisted contributions** (including SEP drafts) are accepted as long as you've read the output yourself and can defend it under review. The maintainer-side guidance lives in [`maintainers/spec-evolution.md`](maintainers/spec-evolution.md#handling-ai-generated-seps).

### Build a tool, validator, or integration

Great — you don't need to own it or ask permission. If you want it listed on sightmap.org, open a PR. To stay aware of spec changes, subscribe to issues labeled `spec-change`.

### Share a real-world sightmap

Examples from real apps (with permission) are genuinely useful. Open a PR adding a file under [`spec/v1/examples/`](spec/v1/examples/) or link to one in a Discussion.

## Development setup

The repo is split by area; set up only what you're touching.

- **Spec** (`spec/`) — schema validation runs on Node. See [`spec/`](spec/) for the validator.
- **Go** (`go/`) — Go 1.25+. `cd go && go test ./...`.
- **Docs** (`docs/`) — Node 20+ and the Mintlify CLI: `npm i -g mint`, then `cd docs && mint dev`.
- **Web** (`web/`) — Node 20+ and **pnpm**. `cd web && pnpm install && pnpm dev`. Please don't commit a `package-lock.json` or `yarn.lock`.

CI runs the relevant checks per area on every PR.

### Generated files

A couple of artifacts are **generated from a canonical source and checked in**, and CI fails if they drift:

- `docs/reference/schema.md` — generated from `spec/v1/schema.md`. Regenerate with `npm run sync-docs` (or `node docs/scripts/sync-spec.mjs`).
- `go/skills/<name>/` — generated from the canonical `skills/`. Regenerate with `go generate ./skills/...` from `go/`.

If you edit `spec/v1/schema.md`, run `npm run sync-docs` and commit the regenerated page in the same PR. To have this happen automatically on commit, enable the opt-in git hooks once:

```sh
git config core.hooksPath .githooks
```

The hook regenerates and stages the schema page only when its inputs are staged; bypass any time with `git commit --no-verify`.

## Pull request expectations

- **One concern per PR.** If you fix two unrelated bugs, that's two PRs. Small PRs get reviewed fast.
- **Describe the *why*.** The PR template asks for it. "What" is visible in the diff.
- **Update docs in the same PR.** If you change a schema field, update `spec/v1/schema.md`, the JSON Schema, and any affected examples together.
- **Add a changeset** if your PR affects the published `@sightmap/sightmap` package (`go/`). Run `npm run changeset`, pick the bump, and commit the `.changeset/*.md` file. Infra/docs-only changes can skip it. See [Releasing](#releasing).
- **Don't reformat unrelated code.** Keep diffs focused.
- **Expect review.** Maintainers aim for first response within 3 business days. See [`maintainers/reviewing-prs.md`](maintainers/reviewing-prs.md) for the review bar.

We do **not** require a CLA. Instead, contributions are gated by a lightweight Developer Certificate of Origin sign-off (see below).

## Releasing

The `@sightmap/sightmap` npm package is versioned with
[changesets](https://github.com/changesets/changesets). You don't hand-edit
versions, `CHANGELOG.md`, or the plugin manifest versions — the tooling does it.

1. **In your PR**, if the change is user-facing for the package, add a changeset:

   ```sh
   npm run changeset
   ```

   Pick `patch` / `minor` / `major`, write a short summary, and commit the
   generated `.changeset/*.md`.

2. **On merge to `main`**, the `release` workflow opens a "Version Packages"
   PR that bumps `go/npm/package.json`, writes `go/npm/CHANGELOG.md`, and syncs
   the plugin manifest versions.

3. **A maintainer merges that PR.** That merge triggers the `release` workflow
   again; with no changesets left to consume, it tags the release, runs
   goreleaser, and publishes the npm packages automatically. No manual tag
   push. More detail in [`.changeset/README.md`](.changeset/README.md).

## Developer Certificate of Origin

We use the [Developer Certificate of Origin](https://developercertificate.org/) (DCO) — a one-line per-commit assertion that you wrote the contribution, or otherwise have the right to submit it under the project's license. It's the same mechanism the Linux kernel, Kubernetes, and GitLab use.

Sign off every commit with `-s`:

```bash
git commit -s -m "Your commit message"
```

That appends a `Signed-off-by: Your Name <your@email>` trailer to the commit body. The trailer must match `git config user.name` and `git config user.email`. The DCO check on the PR runs against every commit on the branch.

Forgot to sign off? Amend the last commit:

```bash
git commit --amend --signoff
```

To add sign-off to a range of commits already on your branch, rebase against the base branch — this rewrites history, so you'll need to force-push:

```bash
git rebase --signoff main
git push --force-with-lease
```

## Commit and branch conventions

- Branch names: `feat/<topic>`, `fix/<topic>`, `docs/<topic>`, `spec/<topic>`, or `sep/<number>-<slug>` for SEP drafts.
- Commit messages: short imperative subject, optional body explaining *why*. Conventional Commits are welcome but not required.
- Every commit ends with `Signed-off-by: …` (see DCO above).

## License

By contributing, you agree that your contributions will be licensed under the project's [MIT License](LICENSE).
