# Sightmap

**Teach agents how to see and use your web app.**

`sitemap.xml` tells search engines how to crawl your site.
`.sightmap/` **teaches** agents how to use it.

A `.sightmap/` directory in your repo is a YAML map of your app's **views**,
**components**, and **API routes** — checked in, shared across every agent, and
learned from the running app, not the source code. Any definition can carry a
`memory` list: freeform notes about quirks, invariants, and shortcuts the source
code doesn't record.

→ **[sightmap.org](https://sightmap.org)** — overview and landing page
→ **[docs.sightmap.org](https://docs.sightmap.org)** — full documentation
→ [`spec/`](spec/) — the canonical specification
→ [`go/`](go/) — the reference implementation (Go library + `sightmap` CLI)

## This repository

This is the home of the open Sightmap project. It holds the spec, the reference
implementation, and both websites in one place:

| Path | What it is |
|---|---|
| [`spec/`](spec/) | The **normative** specification — `spec/v1/` schema + JSON Schema, the SEP process (`spec/seps/`), and language-agnostic conformance fixtures. Source of truth. |
| [`go/`](go/) | The reference **Go implementation** — the `sightmap` CLI (live browser capture, annotated snapshots, coverage) plus a `go get`-able library for the component model and selector matching. Published to npm as [`@sightmap/sightmap`](https://www.npmjs.com/package/@sightmap/sightmap). |
| [`docs/`](docs/) | The documentation site at [docs.sightmap.org](https://docs.sightmap.org) (Astro Starlight). |
| [`web/`](web/) | The marketing landing page at [sightmap.org](https://sightmap.org) (React + Vite). |

Each area has its own README with build and contribution details.

## Quickstart

Drop a `.sightmap/` directory at your project root. Every `*.yaml` / `*.yml`
file under it is discovered recursively and merged.

```yaml
# .sightmap/home.yaml
version: 1

views:
  - name: FlightSearch
    route: /search
    components:
      - name: DepartureDatePicker
        selector: '[data-picker="departure"]'
        memory:
          - Accepts typed YYYY-MM-DD — skips the calendar
```

Then point your agent at the directory. The [quickstart](https://docs.sightmap.org/start/quickstart/)
walks the full loop, and the [`sightmap` CLI](go/) drives curation against a live
browser.

## Who reads it

- **Your coding agents** — Claude Code, Cursor, Codex, Windsurf, and anything else that reads repo files.
- **[Subtext](https://subtext.fullstory.com)** — runtime enrichment for live browser sessions and session replays; snapshots and network traces get semantic names, memory guides, and source paths injected automatically.

## Contributing

Sightmap is open source and stewarded by the [Subtext](https://subtext.fullstory.com)
team at Fullstory. We welcome contributions from anyone.

- Start with [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Proposing a spec change? See [`spec/seps/README.md`](spec/seps/README.md)
- Reporting security issues: [`SECURITY.md`](SECURITY.md)
- Who maintains this: [`MAINTAINERS.md`](MAINTAINERS.md)

## License

MIT — see [`LICENSE`](LICENSE).
