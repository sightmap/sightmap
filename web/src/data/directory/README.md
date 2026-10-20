# Atlas directory: WebMCP listings

This directory holds the **WebMCP listings** the Atlas publishes alongside the
vendored community sightmaps in `../atlas/`. A listing is a site that exposes
callable WebMCP tools, plus the scan report that inspected them. It is the
lightweight sibling of an atlas entry: no corpus, no screenshots, no PR against
`sightmap/atlas` — one YAML file a maintainer can read in a minute.

Nothing here is fetched at build or run time. Listings arrive by pull request
(usually opened by the review agent, see `../../../ATLAS_PIPELINE.md`), are
reviewed by a human, and ship with the next deploy. Takedown is deletion plus
rebuild. Validation is per listing and non-fatal (`scripts/lib/directory.ts`):
a broken file is dropped from the gallery with a build-log warning, and the
rest of the site ships.

## Layout

```
src/data/directory/
  README.md                        this file
  <slug>.yaml                      one listing (file name == slug)
  scans/<slug>/<date>.json         scan reports, one per scan, kept forever
```

## Listing (`<slug>.yaml`)

```yaml
slug: example                      # lowercase kebab-case; == file name; unique across atlas + directory
name: Example
url: https://example.com/
host: example.com
description: Example does something useful.     # one sentence, written on review
category: devtools                 # lowercase id; free vocabulary, keep it small
type: live                         # live | demo
built_with_sightkick: false
submitted_by: owner                # owner | nominator | maintainer  (never an email)
labels: []                         # editorial: promising | featured
collections: []                    # editorial: e.g. competition-demos, built-with-sightkick, new-this-week
added: 2026-09-09
updated: 2026-09-09
scan: scans/example/2026-09-09.json   # the report this listing was reviewed against

tools:                             # the reviewed summary; full schemas live in the scan
  - name: search
    kind: read                     # read | action | sensitive — Atlas's classification, corrected on review
    description: Search public records.
    page: /

journey:                           # optional: one supervised, read-only run a maintainer performed
  intent: Find the pricing page
  outcome: passed                  # passed | failed
  ran_at: 2026-09-09
  tools_called: [search, get_page]
  duration_ms: 4200
  notes: Ran headless with no account; two tool calls.

suggested_journeys:                # from the review; shown as "what an agent could try"
  - intent: Find the pricing page
    tools: [search, get_page]
strengths: []                      # from the review; short factual lines
improvements: []
```

Three labels, never a grade: `live` (a public product surface), `demo`
(hackathon, competition, example, or experimental), and the existing atlas
entries, which the site presents as **community maps** — useful automation
data, not a claim that the owner ships WebMCP.

## Scan report (`scans/<slug>/<date>.json`)

Written by `pnpm atlas:scan <url>` (`scripts/lib/scan.ts`), which drives a
`sightmap browser` session with a registration recorder installed on every
document. The scan **enumerates tools and never executes one**. Shape (see
`src/types/directory.ts` for the full type):

```jsonc
{
  "version": 1,
  "url": "https://example.com/", "finalUrl": "https://example.com/", "host": "example.com",
  "scannedAt": "2026-09-09T20:00:00.000Z",
  "scanner": { "name": "sightmap-atlas-scan", "version": "1.0.0", "browser": "sightmap 0.31.2" },
  "surface": "native",           // native | polyfilled | declarative | absent
  "status": "tools-found",       // tools-found | api-empty | api-absent | blocked | load-error | needs-review
  "intent": "Find pricing",      // what the submitter said; stored, not acted on
  "pages": [{ "url": "…", "path": "/", "title": "…", "status": 200, "surface": "native", "tools": ["search"] }],
  "tools": [{
    "name": "search", "description": "…", "inputSchema": { "type": "object", "properties": {} },
    "page": "/", "pages": ["/"], "impl": "imperative", "api": "navigator",
    "risk": "read", "riskReason": "name starts with \"search\"", "warnings": []
  }],
  "checks": [{ "id": "input-schema", "label": "Every tool has an input schema", "ok": true, "detail": "9/9" }],
  "counts": { "tools": 9, "pages": 3, "read": 5, "action": 3, "sensitive": 1, "described": 9, "withInputSchema": 8, "declarative": 0 },
  "hints": { "forms": [], "links": [], "title": "…", "description": "…" },
  "notes": ["$ sightmap browser start …"]   // includes the CLI transcript
}
```

Statuses are observable outcomes only. There is deliberately no `safe`,
`trusted`, or `certified`.

## What the build generates

`scripts/build-atlas.ts` reads this directory (after the community atlas) and
emits:

| Output | Purpose |
|---|---|
| `src/generated/atlas-manifest.ts` | adds `directoryListings: DirectoryListingView[]` and `directoryCategories: string[]` next to the existing `atlasEntries` / `atlasCategories` |
| `public/atlas/directory.json` | `{ schema_version: 1, generated_at, listings: [...] }` — the agent-facing index (no full schemas) |
| `public/atlas/stats.json` | counts: listings by type, tools by kind, surfaces, community maps |
| `public/atlas/sites/<slug>.json` | one listing with its counts, checks, journey, drift |
| `public/atlas/sites/<slug>/tools.json` | that listing's tools with full input schemas |
| `public/atlas/scans/<slug>.json` | the latest scan report, verbatim |
| `public/atlas/scans/<slug>/<date>.json` | every scan on file |
| `public/atlas/<slug>.md` | markdown twin of the listing page |
| `public/atlas/<slug>/badge.svg` | "Sightmap · N tools detected · <date>" badge |
| `public/atlas/hosts/<host>.json` | host lookup: `{ slug, url, tool_count, last_scanned }`; served at `/api/atlas/lookup/<host>` by a netlify.toml rewrite |

Pages: `/atlas` lists community maps and directory listings together with a
type filter; `/atlas/<slug>` renders a listing (or a community entry — slugs
are unique across both).

## Scripts

| Command | Does |
|---|---|
| `pnpm atlas:scan <url> [--out f.json] [--md f.md] [--max-pages 3] [--intent …] [--path /p] [--allow-local]` | scan one site; needs the `sightmap` CLI (`$SIGHTMAP_BIN`) and a Chrome build (`sightmap browser install` or `$ATLAS_CHROME_PATH`) |

Every step is idempotent: rerunning a scan adds a new dated report and the
listing's `drift` block shows what changed since the one before.
