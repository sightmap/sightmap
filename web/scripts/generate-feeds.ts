// Emits the site's machine-readable index files — dist/rss.xml,
// dist/sitemap.xml and dist/llms.txt — from the published posts and the
// vendored atlas.
//
// Neither feed existed on the Subtext site this pipeline came from. RSS
// matters here because it is the only way to follow an open-source project's
// writing without an account, and the docs site already publishes a changelog
// feed. llms.txt is the same idea aimed at agents, which is most of this
// project's audience: one plain-text table of contents for the whole site,
// with a line per atlas entry (P4.3).
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { loadPosts } from './lib/posts'
import { loadAtlas } from './lib/atlas'
import {
  SITE_URL,
  SITE_NAME,
  SITE_DESCRIPTION,
  BLOG_DESCRIPTION,
  ATLAS_DESCRIPTION,
  escXml,
} from './lib/site'

const DIST = path.resolve('dist')
const CONTENT_DIR = path.resolve('content/blog')
const ATLAS_DIR = path.resolve('src/data/atlas')

export interface FeedPost {
  slug: string
  title: string
  excerpt: string
  date: string
  author: string
}

/** The subset of an atlas entry the sitemap and llms.txt need. */
export interface FeedAtlasEntry {
  slug: string
  name: string
  description: string
  updated: string
}

// Feed readers expect RFC 822. Posts carry a date but no time, so they are
// pinned to midnight UTC rather than the build machine's timezone.
const rfc822 = (date: string): string => new Date(`${date}T00:00:00Z`).toUTCString()

export function buildRss(posts: FeedPost[], now: Date): string {
  const items = posts
    .map((p) => {
      const url = `${SITE_URL}/blog/${p.slug}`
      return `    <item>
      <title>${escXml(p.title)}</title>
      <link>${url}</link>
      <guid isPermaLink="true">${url}</guid>
      <description>${escXml(p.excerpt)}</description>
      <dc:creator>${escXml(p.author)}</dc:creator>
      <pubDate>${rfc822(p.date)}</pubDate>
    </item>`
    })
    .join('\n')

  return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>${escXml(SITE_NAME)}</title>
    <link>${SITE_URL}/blog</link>
    <description>${escXml(BLOG_DESCRIPTION)}</description>
    <language>en-us</language>
    <lastBuildDate>${now.toUTCString()}</lastBuildDate>
    <atom:link href="${SITE_URL}/rss.xml" rel="self" type="application/rss+xml"/>
${items}
  </channel>
</rss>
`
}

export function buildSitemap(posts: FeedPost[], atlas: FeedAtlasEntry[], now: Date): string {
  const today = now.toISOString().slice(0, 10)
  const urls = [
    { loc: `${SITE_URL}/`, lastmod: today, priority: '1.0' },
    { loc: `${SITE_URL}/blog`, lastmod: posts[0]?.date ?? today, priority: '0.8' },
    ...posts.map((p) => ({
      loc: `${SITE_URL}/blog/${p.slug}`,
      lastmod: p.date,
      priority: '0.7',
    })),
    // The atlas index moves whenever any entry does, so its lastmod is the
    // most recent entry's — loadAtlas() sorts newest-first, so that is [0].
    { loc: `${SITE_URL}/atlas`, lastmod: atlas[0]?.updated ?? today, priority: '0.8' },
    ...atlas.map((e) => ({
      loc: `${SITE_URL}/atlas/${e.slug}`,
      lastmod: e.updated,
      priority: '0.7',
    })),
  ]

  const body = urls
    .map(
      (u) => `  <url>
    <loc>${u.loc}</loc>
    <lastmod>${u.lastmod}</lastmod>
    <priority>${u.priority}</priority>
  </url>`
    )
    .join('\n')

  return `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${body}
</urlset>
`
}

// A description has to survive as one line in llms.txt — a stray newline in
// community-authored text would split one entry into two malformed ones.
const oneLine = (s: string): string => s.replace(/\s+/g, ' ').trim()

/**
 * dist/llms.txt, in the llmstxt.org shape: an H1, a blockquote summary, then
 * link sections. Handwritten for the fixed parts of the site and generated for
 * the two that grow — one line per post, one line per atlas entry.
 *
 * No file to append to: the site had no llms.txt before this change, so P4.3's
 * "append a line per entry" is satisfied by creating it with the atlas section
 * already in place rather than by editing something that did not exist.
 */
export function buildLlmsTxt(posts: FeedPost[], atlas: FeedAtlasEntry[]): string {
  const lines = [
    `# ${SITE_NAME}`,
    '',
    `> ${oneLine(SITE_DESCRIPTION)}`,
    '',
    '## Docs',
    '',
    `- [Documentation](https://docs.sightmap.org): Guides, CLI reference, and the schema reference.`,
    `- [Specification](https://github.com/sightmap/sightmap/tree/main/spec): The normative spec, JSON Schema, and conformance fixtures.`,
    '',
    '## Atlas',
    '',
    `> ${oneLine(ATLAS_DESCRIPTION)} Index: ${SITE_URL}/atlas/index.json`,
    '',
  ]

  if (atlas.length === 0) {
    lines.push('- No entries published yet.', '')
  } else {
    for (const e of atlas) {
      // Each entry links to its Markdown twin, not the HTML page: this file is
      // read by agents, and the twin is the version without the chrome.
      lines.push(`- [${oneLine(e.name)}](${SITE_URL}/atlas/${e.slug}.md): ${oneLine(e.description)}`)
    }
    lines.push('')
  }

  lines.push('## Blog', '', `> ${oneLine(BLOG_DESCRIPTION)}`, '')
  if (posts.length === 0) {
    lines.push('- No posts published yet.', '')
  } else {
    for (const p of posts) {
      lines.push(`- [${oneLine(p.title)}](${SITE_URL}/blog/${p.slug}): ${oneLine(p.excerpt)}`)
    }
    lines.push('')
  }

  return lines.join('\n')
}

async function main() {
  const loaded = await loadPosts(CONTENT_DIR)
  const posts: FeedPost[] = loaded.map((p) => ({
    slug: p.frontmatter.slug,
    title: p.frontmatter.title,
    excerpt: p.frontmatter.excerpt,
    date: p.frontmatter.date,
    author: p.frontmatter.author,
  }))

  const atlas: FeedAtlasEntry[] = (await loadAtlas(ATLAS_DIR)).entries.map((e) => ({
    slug: e.slug,
    name: e.name,
    description: e.description,
    updated: e.updated,
  }))

  // Build time, not post time — only affects lastBuildDate and the homepage
  // lastmod, both of which are meant to move on every deploy.
  const now = new Date()

  fs.mkdirSync(DIST, { recursive: true })
  fs.writeFileSync(path.join(DIST, 'rss.xml'), buildRss(posts, now))
  fs.writeFileSync(path.join(DIST, 'sitemap.xml'), buildSitemap(posts, atlas, now))
  fs.writeFileSync(path.join(DIST, 'llms.txt'), buildLlmsTxt(posts, atlas))
  console.log(
    `  wrote dist/rss.xml, dist/sitemap.xml and dist/llms.txt (${posts.length} post(s), ${atlas.length} atlas entry(s))`
  )
}

// Only run when invoked directly, so the test can import buildRss,
// buildSitemap and buildLlmsTxt. Compares resolved URLs rather than a filename suffix check
// (`.endsWith('generate-feeds.ts')`) so a different runner — a symlink, a
// wrapper script, being required under another name — can't silently skip
// feed generation with no error. This is the last step of `pnpm build`, so a
// skipped run would ship stale feeds with exit 0 and no output at all.
const entry = process.argv[1]
if (entry && import.meta.url === pathToFileURL(entry).href) {
  main().catch((err) => {
    console.error('Feed generation failed:\n', err instanceof Error ? err.message : err)
    process.exit(1)
  })
}
