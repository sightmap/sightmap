# @sightmap/next

The Next.js side of Sightmap and Sightkick. Three commands and one component, no runtime
dependencies beyond `next` and `react`.

```bash
npm i -D @sightmap/sightmap @sightmap/sightkick agent-browser
npm i @sightmap/next
```

## `sightmap-next seed`

One-shot scaffold of `.sightmap/` from the router. Never overwrites.

```
sightmap-next seed [app-root] [--app-dir DIR] [--sightmap-dir DIR] [--base-url URL] [--dry-run]
```

- Walks `app/**/page.*` (and `pages/**`). Route groups vanish, `[id]` → `:id`,
  `[...slug]` → `**`, `@slots`, `(.)intercepts`, and `_private` folders are skipped.
- One feature file per top-level route (`home.yaml`, `tasks.yaml`, …), per the spec's
  authoring conventions. Each view gets `route:`, `source:`, the layout chain as
  `dependencies:`, and `stability: stub`.
- Route handlers under `app/api` become `requests:` seeds, with `method:` when the file
  exports exactly one HTTP verb.

Then curate against the running app with the `sightmap-authoring` skill and drop the
`stub` markers.

## `sightmap-next build`

```
sightmap-next build [app-root] [--public-dir DIR] [--no-init-script]
```

Writes, using the `sightmap` and `sightkick` CLIs:

| File | What |
|---|---|
| `public/.well-known/sightmap.json` | the corpus, `sightmap export` |
| `public/.well-known/sightkick.json` | the compiled tool IR, `sightkick build` |
| `public/sightkick-runtime.js` | the runtime that registers the IR on `document.modelContext` |
| `webmcp.init.js` | runtime + IR in one file for `agent-browser --init-script` |

Wire it as `"prebuild": "sightmap-next build"`.

## `<SightkickTools/>`

```tsx
// app/layout.tsx
import { SightkickTools } from "@sightmap/next";
import ir from "../public/.well-known/sightkick.json";

<SightkickTools ir={ir} />          // inline the IR (registers as soon as the runtime loads)
<SightkickTools />                  // or fetch /.well-known/sightkick.json after hydration
<SightkickTools enabled={process.env.VERCEL_ENV !== "production"} />   // gate it
```

Registers the tool layer on every page. Tools are view-scoped and re-register on
client-side navigation. If an agent-browser init script already booted the runtime, the
component reuses it.

## `sightmap-next run-plan`

```
sightmap-next run-plan <plan.json ...> [--base-url URL] [--init-script FILE] [--session NAME]
                       [--stamp] [--stale-ok] [--dry-run]
```

Replays a stored plan — a Gherkin scenario resolved to tool calls and expectations —
through `agent-browser webmcp invoke`. Same plan format and expectation vocabulary as
Sightkick's `scripts/run-plan.mjs` (`ok`, `value.{equals,contains,absent}`,
`list.{length,contains,excludes}`); only the executor differs. `--stamp` records the
feature-file and compiled-IR hashes; a later run refuses to proceed when either moved.

## Tests

```bash
npm test
```
