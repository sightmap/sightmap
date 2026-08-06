import { describe, it, expect } from 'vitest'
import {
  buildLlmsTxt,
  buildRss,
  buildSitemap,
  type FeedAtlasEntry,
  type FeedPost,
} from './generate-feeds'

const POSTS: FeedPost[] = [
  {
    slug: 'sightmaps',
    title: 'Sightmaps: the runtime map of your app',
    excerpt: "It's a map, not a movie.",
    date: '2026-07-28',
    author: 'Clint Ayres',
  },
  {
    slug: 'older',
    title: 'Sightmaps & agents',
    excerpt: 'Second.',
    date: '2026-01-01',
    author: 'Chip Lay',
  },
]

const ATLAS: FeedAtlasEntry[] = [
  {
    slug: 'sightmap-org',
    name: 'Sightmap',
    description: 'Marketing site and blog for the Sightmap spec.',
    updated: '2026-08-06',
  },
  {
    slug: 'example-shop',
    name: 'Example & Co',
    description: 'A storefront.',
    updated: '2026-03-02',
  },
]

const NOW = new Date('2026-07-28T12:00:00Z')

describe('buildRss', () => {
  it('emits a channel with the site metadata', () => {
    const xml = buildRss(POSTS, NOW)
    expect(xml).toContain('<?xml version="1.0" encoding="UTF-8"?>')
    expect(xml).toContain('<title>Sightmap</title>')
    expect(xml).toContain('<link>https://sightmap.org/blog</link>')
    expect(xml).toContain('<atom:link href="https://sightmap.org/rss.xml"')
  })

  it('emits one item per post with an absolute guid', () => {
    const xml = buildRss(POSTS, NOW)
    expect(xml.match(/<item>/g)).toHaveLength(2)
    expect(xml).toContain('<guid isPermaLink="true">https://sightmap.org/blog/sightmaps</guid>')
  })

  it('escapes apostrophes and ampersands in titles and descriptions', () => {
    const xml = buildRss(POSTS, NOW)
    expect(xml).toContain('It&apos;s a map, not a movie.')
    expect(xml).toContain('<title>Sightmaps &amp; agents</title>')
    // The raw ampersand must not survive anywhere — an unescaped `&` is the
    // single most common way to hand a feed reader invalid XML.
    expect(xml).not.toMatch(/&(?!amp;|apos;|lt;|gt;|quot;)/)
  })

  it('formats pubDate as RFC 822', () => {
    const xml = buildRss(POSTS, NOW)
    expect(xml).toContain('<pubDate>Tue, 28 Jul 2026 00:00:00 GMT</pubDate>')
  })

  it('emits a valid channel with zero items when there are no published posts', () => {
    // The actual state of the repo right now: the only post in content/blog
    // is draft: true, so loadPosts() returns []. This must still produce a
    // well-formed channel — no placeholder item, no undefined leaking in.
    const xml = buildRss([], NOW)
    expect(xml).toContain('<?xml version="1.0" encoding="UTF-8"?>')
    expect(xml).toContain('<title>Sightmap</title>')
    expect(xml).toContain('<link>https://sightmap.org/blog</link>')
    expect(xml).toContain('<atom:link href="https://sightmap.org/rss.xml"')
    expect(xml).toContain('</channel>')
    expect(xml).not.toContain('undefined')
    expect(xml.match(/<item>/g)).toBeNull()
  })
})

describe('buildSitemap', () => {
  it('lists the homepage, both indexes, every post, and every atlas entry', () => {
    const xml = buildSitemap(POSTS, ATLAS, NOW)
    expect(xml).toContain('<loc>https://sightmap.org/</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/blog</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/blog/sightmaps</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/atlas</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/atlas/sightmap-org</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/atlas/example-shop</loc>')
    expect(xml.match(/<url>/g)).toHaveLength(7)
  })

  it('uses the post date as lastmod for post URLs', () => {
    expect(buildSitemap(POSTS, ATLAS, NOW)).toContain('<lastmod>2026-07-28</lastmod>')
  })

  it('uses the newest entry as the atlas index lastmod', () => {
    // loadAtlas() sorts newest-first, so [0] is the most recently updated
    // entry — the index page changes whenever that one does.
    expect(buildSitemap(POSTS, ATLAS, NOW)).toContain('<lastmod>2026-08-06</lastmod>')
  })

  it('still lists the homepage and both indexes when nothing is published', () => {
    // Same zero-content edge case as above: /blog's and /atlas's lastmod both
    // fall back to `today` and must not be "undefined".
    const xml = buildSitemap([], [], NOW)
    expect(xml).toContain('<loc>https://sightmap.org/</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/blog</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/atlas</loc>')
    expect(xml.match(/<url>/g)).toHaveLength(3)
    expect(xml).not.toContain('undefined')
    expect(xml).toContain('<lastmod>2026-07-28</lastmod>')
  })
})

describe('buildLlmsTxt', () => {
  it('opens with the site name and a one-line summary', () => {
    const txt = buildLlmsTxt(POSTS, ATLAS)
    expect(txt.startsWith('# Sightmap\n')).toBe(true)
    expect(txt).toContain('> An open YAML spec and CLI')
  })

  it('links each atlas entry to its markdown twin, not its HTML page', () => {
    const txt = buildLlmsTxt(POSTS, ATLAS)
    expect(txt).toContain(
      '- [Sightmap](https://sightmap.org/atlas/sightmap-org.md): Marketing site and blog for the Sightmap spec.'
    )
    expect(txt).toContain('- [Example & Co](https://sightmap.org/atlas/example-shop.md): A storefront.')
    // The HTML page is linked from the Atlas heading's index pointer only.
    expect(txt).not.toContain('](https://sightmap.org/atlas/sightmap-org)')
  })

  it('points at the machine index so an agent never has to scrape the gallery', () => {
    expect(buildLlmsTxt(POSTS, ATLAS)).toContain('https://sightmap.org/atlas/index.json')
  })

  it('emits one line per post', () => {
    const txt = buildLlmsTxt(POSTS, ATLAS)
    expect(txt).toContain(
      "- [Sightmaps: the runtime map of your app](https://sightmap.org/blog/sightmaps): It's a map, not a movie."
    )
  })

  it('keeps every list item on one line', () => {
    // A newline inside a community-authored description would split one entry
    // into two malformed ones, so oneLine() collapses whitespace first.
    const txt = buildLlmsTxt(POSTS, [
      { slug: 'multi', name: 'Multi\nLine', description: 'First line.\n\nSecond line.', updated: '2026-01-01' },
    ])
    expect(txt).toContain('- [Multi Line](https://sightmap.org/atlas/multi.md): First line. Second line.')
  })

  it('says so rather than emitting an empty section when nothing is published', () => {
    const txt = buildLlmsTxt([], [])
    expect(txt).toContain('## Atlas')
    expect(txt).toContain('## Blog')
    expect(txt).toContain('- No entries published yet.')
    expect(txt).toContain('- No posts published yet.')
    expect(txt).not.toContain('undefined')
  })
})
