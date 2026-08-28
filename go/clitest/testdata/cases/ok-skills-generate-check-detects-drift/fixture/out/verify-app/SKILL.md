---
name: verify-app
description: "Use when a request names part of the App app UI — library ui — or asks to click, fill, verify, or screenshot something in it. Maps those words onto sightmap component queries and `sightmap browser` commands."
---

# Drive App

This app has a sightmap corpus at `.sightmap/`: 1 area, 1 view, 0 components with stable names. Read the area file for whatever the request names before opening a browser — it has the routes, the component names, and the exact commands. Do not guess selectors; the corpus already has them.

## Pick a flow

Read [references/README.md](references/README.md) for the full area index. Each area's reference file names the routes, components, and vocabulary for that part of the app.

## Quick start

```bash
sightmap version                     # is the CLI installed?
sightmap browser start                # launch Chrome + the overlay server
sightmap browser status               # current URL
sightmap snapshot --coverage           # read the page as an annotated component tree
```

## Command surface

Categories, not an exhaustive list. **`--help` is canonical** — run `sightmap <command> --help` before inventing a flag.

| Category | Commands |
|---|---|
| Session | `browser start` · `stop` · `status` · `navigate '<url>'` (positional, no `--url`) · `tabs list\|new\|close\|resize` |
| Observe | `snapshot [--coverage]` · `browser bounds` · `browser eval 'js'` |
| Act | `browser click` · `fill [--clear]` · `hover` · `keypress` · `scroll` · `drag` · `dialog accept\|dismiss` |
| Synchronize | `browser wait-for --view NAME \| --component 'Query' \| --selector CSS \| --url SUBSTR \| --load` |
| Evidence | `browser screenshot --out F.png [--component NAME] [--expand-pct N]` |
| Debug | `console list\|get` · `network list\|get` |

Every verb accepts `--addr`, `--tab`, `--sightmap-dir`.

## Proof bar

A claim that something works is not evidence. Before saying a flow passes:

1. `sightmap browser wait-for --view <ViewName>` succeeded — you are demonstrably on the right route, not a redirect or an error page.
2. `sightmap snapshot --coverage` returned a non-empty tree. Zero interactive nodes renders `∅` and exits non-zero: the page is blank or still hydrating.
3. A screenshot for anything visual, clipped to the component under test with `--component`.
4. If a step was skipped, say which one and why — don't silently narrow scope.

## Driving conventions

- **Address by component query, not by probe ID.** IDs come from one snapshot and go stale the moment the page re-renders. Queries re-resolve atomically.
- **Whitespace is a descendant combinator** (`LibraryTable LibrarySearchBar`). There is no `>` child combinator.
- **Predicates**: `Name[prop=value]`, ops `=`, `^=` (prefix), `*=` (substring), trailing ` i` for case-insensitive.
- **Occurrence**: `Name#N`, 0-based, when several nodes match.
- **After any navigating action**, `wait-for --view` before the next snapshot.

## Feature map

1 areas, one reference file each, under [`references/`](references/README.md).
