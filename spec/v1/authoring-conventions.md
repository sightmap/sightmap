---
title: "Authoring conventions (v1, informative)"
description: "How the curated .sightmap/ corpus is authored: model, file layout, agent routing."
---

> **Status:** v1 — **informative, non-normative**. Guidance, not a contract. The normative byte-level format and diagnostic contract is [`canonical-format.md`](./canonical-format.md); the data schema is [`schema.md`](./schema.md).

## Curated authority

`.sightmap/` is a **curated authority**: it is authored and maintained by coding agents and humans. There is no build-time generator — nothing regenerates `.sightmap/` from source, and no tool writes it as a side effect of a build.

Discovery is driven by **observing the running app**, not by parsing source. A curating agent drives a live browser session — the `sightmap` CLI, typically via the bundled `sightmap-authoring` skill — reads the annotated component tree, and **proposes** `.sightmap/` entries for human review. The agent writes the YAML, formatted per [`canonical-format.md`](./canonical-format.md#canonical-formatting-rules).

Any tool that reads the corpus at runtime (browser-driving agents, session-replay enrichers) is a **consumer**: it introspects live DOM/runtime state, never source files, and never writes `.sightmap/`.

## Recommended file layout

Non-normative; the layout agent skills converge on. Projects are free to deviate.

```
.sightmap/
  login.yaml      # feature-scoped: one file per top-level route
  dashboard.yaml
  settings.yaml
  shared.yaml     # content reachable from more than one route
  extras.yaml     # content not tied to a single feature route
  config.yaml     # optional; pins spec version
```

- **One feature file per top-level route**, named after the first non-parameter URL segment. `/login` → `login.yaml`, `/users/:id` → `users.yaml`, `/` → `home.yaml`.
- **`shared.yaml`** holds content reachable from more than one route (cross-route components, global requests).
- **`extras.yaml`** holds content not tied to a single feature route (modals not bound to a route, dynamic requests, third-party widgets with stable selectors).
- **Per-feature variants** (`<feature>.agent.yaml`) are permitted when a feature accumulates substantial content; prefer a single `extras.yaml` until usage justifies splitting.
- All `.sightmap/` files are curated and should be committed to git — none are regenerable from source.

## What goes where

When an agent observes something worth recording, default routing:

| Observation | Goes to |
|---|---|
| Component with a stable selector (third-party widget, modal portal, anything route-specific) | `<feature>.yaml` if route-scoped, else `extras.yaml` |
| Memory about an existing view/component | that entry's `memory:` field, in its `<feature>.yaml` |
| Memory about a route with no feature file yet | `extras.yaml` |
| Cross-route memory (e.g. "all forms validate on blur") | file-level `memory:` in `shared.yaml` or `extras.yaml` |
