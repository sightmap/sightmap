# Feature map — App

## Baseline preconditions

- `sightmap version` succeeds (else `npm install -g @sightmap/sightmap`).
- `sightmap browser start`, then `sightmap browser status` shows a real URL.
- Prefer component queries over raw CSS selectors; see the area file for this page.

## Proof and skip reporting

Report per area: PASS with the snapshot/screenshot that proves it, FAIL with the command and its output, or SKIP with the precondition that could not be met.

## Full sweep

Walk the areas below top to bottom for a broad regression. Read the area file before touching the browser — it has the routes, components, and commands.

## Areas

- [library-ui](areas/library-ui.md): Object library listing all saved objects

## Entry contract

Every area file above uses exactly these four H2s, in this order:

1. `## Sub-features` — every named view and component in the area, with the natural-language phrases that map a prompt onto it.
2. `## How to get to it (user POV)` — routes and the words a user would use.
3. `## Driving it with sightmap browser` — a runnable `sightmap browser` block, then the query forms for this area.
4. `## Gotchas` — quirks not recoverable from the DOM.

An area file with fewer than four H2s is stale; run `sightmap skills generate`.
