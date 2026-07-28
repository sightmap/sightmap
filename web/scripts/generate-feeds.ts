// Emits dist/rss.xml and dist/sitemap.xml from the published posts.
//
// Neither existed on the Subtext site this pipeline came from. RSS matters
// here because it is the only way to follow an open-source project's writing
// without an account, and the docs site already publishes a changelog feed.
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { loadPosts } from './lib/posts'
import { SITE_URL, SITE_NAME, BLOG_DESCRIPTION, escXml } from './lib/site'

const DIST = path.resolve('dist')
const CONTENT_DIR = path.resolve('content/blog')

export interface FeedPost {
  slug: string
  title: string
  excerpt: string
  date: string
  author: string
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

export function buildSitemap(posts: FeedPost[], now: Date): string {
  const today = now.toISOString().slice(0, 10)
  const urls = [
    { loc: `${SITE_URL}/`, lastmod: today, priority: '1.0' },
    { loc: `${SITE_URL}/blog`, lastmod: posts[0]?.date ?? today, priority: '0.8' },
    ...posts.map((p) => ({
      loc: `${SITE_URL}/blog/${p.slug}`,
      lastmod: p.date,
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

async function main() {
  const loaded = await loadPosts(CONTENT_DIR)
  const posts: FeedPost[] = loaded.map((p) => ({
    slug: p.frontmatter.slug,
    title: p.frontmatter.title,
    excerpt: p.frontmatter.excerpt,
    date: p.frontmatter.date,
    author: p.frontmatter.author,
  }))

  // Build time, not post time — only affects lastBuildDate and the homepage
  // lastmod, both of which are meant to move on every deploy.
  const now = new Date()

  fs.mkdirSync(DIST, { recursive: true })
  fs.writeFileSync(path.join(DIST, 'rss.xml'), buildRss(posts, now))
  fs.writeFileSync(path.join(DIST, 'sitemap.xml'), buildSitemap(posts, now))
  console.log(`  wrote dist/rss.xml and dist/sitemap.xml (${posts.length} post(s))`)
}

// Only run when invoked directly, so the test can import buildRss and
// buildSitemap. Compares resolved URLs rather than a filename suffix check
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
