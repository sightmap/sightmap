# Sightmap Authoring Reference

Authoritative reference for the sightmap authoring system. For step-by-step
workflow instructions, see the sightmap skill doc. For the event/SEP spec, see
the sightmap spec project. For the new-user quickstart, see the
[README](../README.md).

The reference is split into focused sections under [`reference/`](reference/):

| Section | Covers |
|---------|--------|
| [Coverage model](reference/coverage-model.md) | T1/T2/T3 tiers, the done signal, T2 quality, triage categories |
| [Component model](reference/component-model.md) | Component types, matching rules, properties & transforms, selector quality, naming, the `stability:` field |
| [The outer loop](reference/outer-loop.md) | Session startup → per-page iteration → quality review → cross-page promotion → health check |
| [Tool reference](reference/tools.md) | Every `sightmap` subcommand: `browser`, `snapshot`, `capture`, `coverage`, `report`, `multi-coverage`, `validate`, `lint`, and more |
| [Lint rules](reference/lint-rules.md) | `broad-tag-selector`, `deep-nesting`, `id-hash-selector`, `multi-instance-no-property` |
| [Quality checklist](reference/quality-checklist.md) | The post-T3=0 review pass (auto + manual checks) |

## Where to start

- **New to sightmap?** Read the [README quickstart](../README.md), then the
  [Coverage model](reference/coverage-model.md) and [Component model](reference/component-model.md).
- **Authoring a corpus?** Follow [The outer loop](reference/outer-loop.md) and keep
  the [Tool reference](reference/tools.md) and [Quality checklist](reference/quality-checklist.md) handy.
