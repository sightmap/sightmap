<!--
Thanks for contributing! Please fill in the sections below. Delete any that don't apply.

If you're proposing a change to the spec itself (schema semantics, new fields, new concepts),
please open an SEP first. See spec/seps/README.md.
-->

## What this changes

<!-- One or two sentences. The "what" is visible in the diff; focus on scope. -->

## Why

<!-- The motivation. What problem does this solve? What's the context a reviewer needs? -->

## Area

- [ ] Spec (`spec/`)
- [ ] Go implementation / CLI (`go/`)
- [ ] Docs site (`docs/`)
- [ ] Marketing site (`web/`)
- [ ] Maintainer / infrastructure

## Type of change

- [ ] Bug fix
- [ ] Docs / wording / typo
- [ ] Feature
- [ ] Spec clarification (no semantic change)
- [ ] Spec change (requires an accepted SEP — link below)
- [ ] Example or conformance fixture added/updated

## Linked issues or SEPs

<!-- "Closes #123" or "Implements SEP-0007" -->

## Checklist

- [ ] I read [`CONTRIBUTING.md`](../CONTRIBUTING.md)
- [ ] Every commit on this branch is signed off (`git commit -s`) per [DCO](../CONTRIBUTING.md#developer-certificate-of-origin)
- [ ] I kept this PR focused on a single concern
- [ ] If this changes the spec, there is an accepted SEP linked above
- [ ] If this changes the JSON Schema, I updated the schema file, `spec/v1/schema.md`, and any affected examples
- [ ] Relevant checks pass locally (`go test ./...` in `go/`; `pnpm build` for a site)
