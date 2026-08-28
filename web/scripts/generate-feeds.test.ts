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
    domains: ['sightmap.org'],
    last_verified: '2026-08-06',
    stats: { views: 4, components: 28, requests: 2 },
  },
  {
    // No full stop on the description and two domains, both of which an entry
    // line has to cope with.
    slug: 'example-shop',
    name: 'Example & Co',
    description: 'A storefront',
    updated: '2026-03-02',
    domains: ['example.test', 'shop.example.test'],
    last_verified: '2026-03-01',
    stats: { views: 1, components: 1, requests: 1 },
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
    expect(xml).toContain('<loc>https://sightmap.org/developers</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/webmcp</loc>')
    expect(xml.match(/<url>/g)).toHaveLength(9)
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
    expect(xml).toContain('<loc>https://sightmap.org/developers</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/webmcp</loc>')
    expect(xml.match(/<url>/g)).toHaveLength(5)
    expect(xml).not.toContain('undefined')
    expect(xml).toContain('<lastmod>2026-07-28</lastmod>')
  })
})

describe('buildLlmsTxt', () => {
  it('opens with the site name and a one-line summary', () => {
    const txt = buildLlmsTxt(POSTS, ATLAS)
    expect(txt.startsWith('# Sightmap\n')).toBe(true)
    expect(txt).toContain('> An open YAML spec and CLI')
    expect(txt).toContain('## Sightmap developer resources')
    expect(txt).toContain('https://sightmap.org/openapi.json')
    expect(txt).toContain('https://sightmap.org/developers')
    expect(txt).toContain('https://sightmap.org/api/atlas')
  })

  it('teaches the lookup an agent holding a hostname actually needs', () => {
    // What an agent has when it lands here is a hostname. The section has to
    // get it from there to a corpus on disk, so both verbs are spelled out.
    const txt = buildLlmsTxt(POSTS, ATLAS)
    expect(txt).toContain('sightmap atlas find <domain>')
    expect(txt).toContain('sightmap atlas add <slug>')
    expect(txt).toContain('Check here before exploring a site by hand.')
    // The retired un-namespaced verb must not survive anywhere in the file.
    expect(txt).not.toMatch(/`?sightmap add /)
  })

  it('leads each entry line with the hostnames it covers', () => {
    // The whole point of carrying the domains here: an agent can match its own
    // hostname in this file and never fetch index.json at all.
    const txt = buildLlmsTxt(POSTS, ATLAS)
    expect(txt).toContain(
      '- [Sightmap](https://sightmap.org/atlas/sightmap-org.md): sightmap.org. Marketing site and blog for the Sightmap spec. 4 views, 28 components, 2 requests. Verified 2026-08-06. `sightmap atlas add sightmap-org`'
    )
    // Every domain, singular counts, and a description with no full stop of
    // its own still running into the sentence after it.
    expect(txt).toContain(
      '- [Example & Co](https://sightmap.org/atlas/example-shop.md): example.test, shop.example.test. A storefront. 1 view, 1 component, 1 request. Verified 2026-03-01. `sightmap atlas add example-shop`'
    )
    // The HTML page is linked from the Atlas heading's index pointer only.
    expect(txt).not.toContain('](https://sightmap.org/atlas/sightmap-org)')
  })

  it('points at the machine index so an agent without the CLI has a path too', () => {
    const txt = buildLlmsTxt(POSTS, ATLAS)
    expect(txt).toContain('https://sightmap.org/atlas/index.json')
    expect(txt).toContain("each entry's `domains[]`")
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
      {
        slug: 'multi',
        name: 'Multi\nLine',
        description: 'First line.\n\nSecond line.',
        updated: '2026-01-01',
        domains: ['multi.test'],
        last_verified: '2026-01-01',
        stats: { views: 0, components: 0, requests: 0 },
      },
    ])
    expect(txt).toContain(
      '- [Multi Line](https://sightmap.org/atlas/multi.md): multi.test. First line. Second line. 0 views, 0 components, 0 requests.'
    )
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
