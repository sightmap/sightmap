# Changelog

The published changelog at [`/changelog`](https://docs.sightmap.org/changelog) is **generated**. Do not edit `changelog.mdx` by hand — edit the entry files here and rebuild.

## Add an entry

1. Create a file in `changelog/entries/` named `YYYY-MM-DD-slug.mdx`:

   ```mdx
   ---
   label: "sightmap 0.17.0"
   date: "2026-08-01"
   tags: ["cli"]
   ---
   What changed, in prose. Markdown works — bullets, `code`, [links](https://example.com).
   ```

2. Regenerate the page:

   ```bash
   node scripts/build-changelog.mjs
   ```

3. Commit both the entry file and the regenerated `changelog.mdx`.

That's the whole workflow. No build step, no dependencies — Mintlify serves the committed `changelog.mdx` directly.

## Entry format

| Field | Required | Notes |
|-------|----------|-------|
| `label` | yes | Shown as the update's title, e.g. `"sightmap 0.17.0"`. |
| `date` | yes | `YYYY-MM-DD`. Sorts entries (newest first) and renders as "Released Month D, YYYY". |
| `tags` | no | JSON array, e.g. `["cli"]` or `["go"]`. Rendered as filters in the changelog's right panel. |
| `description` | no | Appended after the date in the update's subtitle. |

The body below the frontmatter is the prose shown inside the entry. Keep **Markdown headings (`##`) out of the body** — Mintlify splits the RSS feed per heading, so a heading turns one release into several feed entries.

Keep the body **MDX-safe**: it renders as MDX, so a bare `<view>` (or `{x}`) placeholder in prose is parsed as a JSX element/expression and fails `mint validate` with `Expected a closing tag for <view>`. Wrap placeholders in backticks — `` `<view>` `` — as every entry already does. `build-changelog.mjs` enforces this for `<…>` tags and fails the build, naming the offending entry. This matters most because entry prose is copied **verbatim** from a changeset, so an unsafe placeholder in a changeset rides straight through to the page.

## How entries get written

**Automatically, as part of the release.** `npm run version-packages` — the command `changesets/action` runs to open the "Version Packages" PR — chains `scripts/changelog-entry.mjs` after `changeset version`. So the moment changesets writes a new block to `go/npm/CHANGELOG.md`, the matching entry file and the regenerated `changelog.mdx` are written too, and all of it lands in that same PR alongside the version bump.

That means the entry is reviewable before the release merges, and the docs site can't fall behind npm. Two things to check while reviewing the release PR:

- **`tags`** is a guess (`["go"]` if the prose reads like library surface, else `["cli"]`). It's a judgment call about audience that the prose doesn't state — fix it in the PR if the guess is wrong.
- **`date`** is the day the Version Packages PR was generated. If that PR sits unmerged across a date boundary, adjust it to the release date.

The prose is copied from the changesets block verbatim — changesets output is already the published wording, so there is nothing to rewrite. Only the scaffolding is stripped: the `### Patch Changes` heading and the `- <hash>:` prefixes.

To write or regenerate one by hand:

```bash
node scripts/changelog-entry.mjs --version 0.17.1   # or --date YYYY-MM-DD
node docs/scripts/build-changelog.mjs
```

It never overwrites an entry that already exists, so a hand-edited entry survives a re-run.

Releases that ship no user-visible change (a release-automation smoke test, for instance) still get an entry — the version appears on npm either way, and a gap in the list reads as an omission.

## RSS

The page sets `rss: true`, so Mintlify publishes a feed at `/changelog/rss.xml` with an RSS button on the page. Public sites only.

## Keeping the page in sync (CI)

The `docs` workflow enforces both halves, and they catch different failures:

| Check | Catches |
|-------|---------|
| `node docs/scripts/build-changelog.mjs --check` | `changelog.mdx` is stale relative to the entry files — someone edited an entry and forgot to rebuild. |
| `node docs/scripts/build-changelog.mjs` (build or `--check`) | An entry body has a raw `<tag>` placeholder outside backticks — MDX would fail `mint validate` downstream. |
| `node scripts/changelog-entry.mjs --check` | A released version in `go/npm/CHANGELOG.md` has no entry at all — the docs site is a version behind npm. |

The first passes happily when a release has no entry (there's nothing to be stale against), which is how 0.17.0 shipped without one. The second is why `go/npm/CHANGELOG.md` is in the workflow's path filter: the "Version Packages" PR touches no `docs/` files, so without it the job wouldn't run on the one PR that matters.
