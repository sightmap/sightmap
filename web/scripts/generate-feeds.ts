// Emits the site's machine-readable index files — dist/rss.xml,
// dist/sitemap.xml and dist/llms.txt — from the published posts and the
// vendored atlas.
//
// Neither feed existed on the Subtext site this pipeline came from. RSS
// matters here because it is the only way to follow an open-source project's
// writing without an account, and the docs site already publishes a changelog
// feed. llms.txt is the same idea aimed at agents, which is most of this
// project's audience: one plain-text table of contents for the whole site,
// with a line per atlas entry and a line per WebMCP directory listing.
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { loadPosts } from './lib/posts'
import { loadAtlas } from './lib/atlas'
import { loadDirectory } from './lib/directory'
import { primaryDomain } from '../src/lib/atlas'
import type { AtlasStats } from '../src/types/atlas'
import {
  SITE_URL,
  SITE_NAME,
  SITE_DESCRIPTION,
  BLOG_DESCRIPTION,
  ATLAS_DESCRIPTION,
  escXml,
  BUILDING_DESCRIPTION,
  SIGHTKICK_DESCRIPTION,
} from './lib/site'

const DIST = path.resolve('dist')
const CONTENT_DIR = path.resolve('content/blog')
const ATLAS_DIR = path.resolve('src/data/atlas')
const DIRECTORY_DIR = path.resolve('src/data/directory')

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
  /**
   * Every hostname the entry covers. An agent arrives at llms.txt holding a
   * hostname and nothing else, so this is the field it matches on and the one
   * field an entry line cannot go without.
   */
  domains: string[]
  last_verified: string
  /** Enough of `stats` to say how much of the site the entry covers. */
  stats: Pick<AtlasStats, 'views' | 'components' | 'requests'>
}

/**
 * The subset of a WebMCP directory listing the sitemap and llms.txt need.
 *
 * `host` is the field an agent matches on, the same role `domains[]` plays for
 * an entry — a listing covers exactly one host, so it is a string rather than
 * a list. `tool_count` is here because it is the one number that decides
 * whether a listing is worth a second fetch at all.
 */
export interface FeedDirectoryListing {
  slug: string
  name: string
  host: string
  description: string
  type: 'live' | 'demo'
  tool_count: number
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

export function buildSitemap(
  posts: FeedPost[],
  atlas: FeedAtlasEntry[],
  now: Date,
  listings: FeedDirectoryListing[] = []
): string {
  const today = now.toISOString().slice(0, 10)
  // /atlas lists entries and listings together, so it changes whenever either
  // side's newest item does. Both arrays arrive sorted newest-first.
  const atlasIndexLastmod = [atlas[0]?.updated, listings[0]?.updated]
    .filter((d): d is string => Boolean(d))
    .sort()
    .pop()
  const urls = [
    { loc: `${SITE_URL}/`, lastmod: today, priority: '1.0' },
    { loc: `${SITE_URL}/developers`, lastmod: today, priority: '0.8' },
    { loc: `${SITE_URL}/building`, lastmod: today, priority: '0.8' },
    { loc: `${SITE_URL}/sightkick`, lastmod: today, priority: '0.8' },
    { loc: `${SITE_URL}/blog`, lastmod: posts[0]?.date ?? today, priority: '0.8' },
    ...posts.map((p) => ({
      loc: `${SITE_URL}/blog/${p.slug}`,
      lastmod: p.date,
      priority: '0.7',
    })),
    { loc: `${SITE_URL}/atlas`, lastmod: atlasIndexLastmod ?? today, priority: '0.8' },
    ...atlas.map((e) => ({
      loc: `${SITE_URL}/atlas/${e.slug}`,
      lastmod: e.updated,
      priority: '0.7',
    })),
    // Listings render at /atlas/<slug> too — slugs are unique across both —
    // so they are the same kind of URL, listed the same way.
    ...listings.map((l) => ({
      loc: `${SITE_URL}/atlas/${l.slug}`,
      lastmod: l.updated,
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

// Community-authored prose does not reliably end in a full stop, and an entry
// line runs several fragments together.
const sentence = (s: string): string => {
  const t = oneLine(s)
  return t === '' || /[.!?]$/.test(t) ? t : `${t}.`
}

const count = (n: number, noun: string): string => `${n} ${noun}${n === 1 ? '' : 's'}`

// Quotes one line of the llmstxt.org summary block; a blank line inside a
// blockquote still needs its marker or the quote ends there.
const quote = (line: string): string => (line === '' ? '>' : `> ${line}`)

/**
 * The Atlas section's summary. It teaches a lookup instead of describing a
 * gallery: this file is where an agent that has never heard of the project
 * finds out the atlas exists, and what it holds at that moment is a hostname,
 * not a slug and not a product name. The two commands and the index.json
 * fallback are the whole path from `squareup.com` to a corpus on disk.
 */
const atlasSummary = (): string[] => [
  `${oneLine(ATLAS_DESCRIPTION)} Check here before exploring a site by hand.`,
  '',
  '    sightmap atlas find <domain>    find a map by hostname',
  '    sightmap atlas add <slug>       install it into .sightmap/',
  '',
  `Without the CLI, fetch ${SITE_URL}/atlas/index.json and match your hostname against each entry's \`domains[]\`, then read that entry's .md twin below.`,
]

/**
 * One entry line, leading with the hostnames it covers so an agent can resolve
 * its own hostname from this file and never fetch index.json at all. The link
 * is to the Markdown twin, not the HTML page: this file is read by agents, and
 * the twin is the version without the chrome.
 */
function atlasLine(entry: FeedAtlasEntry): string {
  const domains = entry.domains.map(oneLine).filter(Boolean).join(', ')
  const parts = [
    sentence(domains),
    sentence(entry.description),
    `${count(entry.stats.views, 'view')}, ${count(entry.stats.components, 'component')}, ${count(entry.stats.requests, 'request')}.`,
    entry.last_verified ? `Verified ${oneLine(entry.last_verified)}.` : '',
    `\`sightmap atlas add ${entry.slug}\``,
  ].filter(Boolean)
  return `- [${oneLine(entry.name)}](${SITE_URL}/atlas/${entry.slug}.md): ${parts.join(' ')}`
}

/**
 * The WebMCP directory section's summary. Same job as atlasSummary(), aimed at
 * the other question an agent arrives with: not "has anyone mapped this site"
 * but "can I call anything on it". Both machine indexes are named here because
 * this section is where an agent learns they exist — directory.json to resolve
 * a host to a listing, stats.json to see the shape of the whole set without
 * fetching it.
 */
const directorySummary = (): string[] => [
  'Sites that expose callable WebMCP tools, enumerated by a scan and reviewed by a maintainer.',
  'A listing records what was found on a date. Nothing here was executed, and no listing is a safety certification.',
  '',
  `Index:  ${SITE_URL}/atlas/directory.json    every listing, with tool counts and machine URLs`,
  `Totals: ${SITE_URL}/atlas/stats.json        listings, tools by kind, surfaces, categories`,
  `Lookup: ${SITE_URL}/api/atlas/lookup/<host> one host, from the stored index (never fetched live)`,
]

/**
 * One listing line: the host first, for the same reason an entry line leads
 * with its domains, then the tool count — the number that decides whether the
 * site is worth another fetch — and the JSON document to fetch next.
 */
function directoryLine(listing: FeedDirectoryListing): string {
  const parts = [
    sentence(listing.host),
    sentence(listing.description),
    `${count(listing.tool_count, 'WebMCP tool')}, ${oneLine(listing.type)}.`,
    `JSON: ${SITE_URL}/atlas/sites/${listing.slug}.json`,
  ].filter(Boolean)
  return `- [${oneLine(listing.name)}](${SITE_URL}/atlas/${listing.slug}.md): ${parts.join(' ')}`
}

/**
 * dist/llms.txt, in the llmstxt.org shape: an H1, a blockquote summary, then
 * link sections. Handwritten for the fixed parts of the site and generated for
 * the two that grow — one line per post, one line per atlas entry.
 */
export function buildLlmsTxt(
  posts: FeedPost[],
  atlas: FeedAtlasEntry[],
  listings: FeedDirectoryListing[] = []
): string {
  const lines = [
    `# ${SITE_NAME}`,
    '',
    `> ${oneLine(SITE_DESCRIPTION)}`,
    '',
    '## Docs',
    '',
    `- [Documentation](https://docs.sightmap.org): Guides, CLI reference, and the schema reference.`,
    `- [Specification](https://github.com/sightmap/sightmap/tree/main/spec): The normative spec, JSON Schema, and conformance fixtures.`,
    `- [The Building](${SITE_URL}/building): ${oneLine(BUILDING_DESCRIPTION)}`,
    `- [Sightkick](${SITE_URL}/sightkick): ${oneLine(SIGHTKICK_DESCRIPTION)}`,
    '',
    '## Sightmap developer resources',
    '',
    `- [Sightmap developer resources](${SITE_URL}/developers): OpenAPI, Atlas HTTP API, docs, CLI, and agent skills.`,
    `- [OpenAPI specification](${SITE_URL}/openapi.json): Machine-readable description of the public Sightmap HTTP API.`,
    `- [Atlas HTTP API](${SITE_URL}/api/atlas): JSON catalog of published sightmaps. No authentication.`,
    `- [CLI on npm](https://www.npmjs.com/package/@sightmap/sightmap): \`npm install -g @sightmap/sightmap\`.`,
    '',
    '## Atlas',
    '',
    ...atlasSummary().map(quote),
    '',
  ]

  if (atlas.length === 0) {
    lines.push('- No entries published yet.', '')
  } else {
    for (const e of atlas) lines.push(atlasLine(e))
    lines.push('')
  }

  lines.push('## WebMCP directory', '', ...directorySummary().map(quote), '')
  if (listings.length === 0) {
    lines.push('- No listings published yet.', '')
  } else {
    for (const l of listings) lines.push(directoryLine(l))
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
    // The schema lets an entry list no domains, and llms.txt is the one place
    // the hostname has to be there anyway. primaryDomain() falls back to the
    // site_url host, which is the same identity the gallery card shows.
    domains: e.domains.length > 0 ? e.domains : [primaryDomain(e)],
    last_verified: e.last_verified,
    stats: e.stats,
  }))

  // Both halves of /atlas render at /atlas/<slug>, so both belong in the
  // sitemap. Loaded with the entry slugs reserved, the same way the build and
  // the prerender load it.
  const listings: FeedDirectoryListing[] = loadDirectory(
    DIRECTORY_DIR,
    atlas.map((e) => e.slug)
  ).listings.map((l) => ({
    slug: l.slug,
    name: l.name,
    host: l.host,
    description: l.description,
    type: l.type,
    tool_count: l.counts.tools,
    updated: l.updated,
  }))

  // Build time, not post time — only affects lastBuildDate and the homepage
  // lastmod, both of which are meant to move on every deploy.
  const now = new Date()

  fs.mkdirSync(DIST, { recursive: true })
  fs.writeFileSync(path.join(DIST, 'rss.xml'), buildRss(posts, now))
  fs.writeFileSync(path.join(DIST, 'sitemap.xml'), buildSitemap(posts, atlas, now, listings))
  fs.writeFileSync(path.join(DIST, 'llms.txt'), buildLlmsTxt(posts, atlas, listings))
  console.log(
    `  wrote dist/rss.xml, dist/sitemap.xml and dist/llms.txt (${posts.length} post(s), ` +
      `${atlas.length} atlas entry(s), ${listings.length} listing(s))`
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
