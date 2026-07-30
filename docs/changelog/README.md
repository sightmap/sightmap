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

## How entries get written

For now, **manually** — a person drops an entry file per release.

Changesets already produces essay-quality prose in `go/npm/CHANGELOG.md`. Copy the released version's block into an entry file, dropping the `### Patch Changes` heading and the `- <hash>:` prefixes. No rewriting needed.

Releases that ship no user-visible change (a release-automation smoke test, for instance) still get an entry — the version appears on npm either way, and a gap in the list reads as an omission.

## RSS

The page sets `rss: true`, so Mintlify publishes a feed at `/changelog/rss.xml` with an RSS button on the page. Public sites only.

## Keeping the page in sync (optional CI)

`node scripts/build-changelog.mjs --check` exits non-zero when `changelog.mdx` is stale relative to the entry files. Wire it into CI to prevent a forgotten rebuild from shipping.
