import { describe, it, expect } from 'vitest'
import { buildRss, buildSitemap, type FeedPost } from './generate-feeds'

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
  it('lists the homepage, the blog index, and every post', () => {
    const xml = buildSitemap(POSTS, NOW)
    expect(xml).toContain('<loc>https://sightmap.org/</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/blog</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/blog/sightmaps</loc>')
    expect(xml.match(/<url>/g)).toHaveLength(4)
  })

  it('uses the post date as lastmod for post URLs', () => {
    expect(buildSitemap(POSTS, NOW)).toContain('<lastmod>2026-07-28</lastmod>')
  })

  it('still lists the homepage and blog index when there are no published posts', () => {
    // Same zero-posts edge case as above: /blog's lastmod falls back to
    // `today` via `posts[0]?.date ?? today` and must not be "undefined".
    const xml = buildSitemap([], NOW)
    expect(xml).toContain('<loc>https://sightmap.org/</loc>')
    expect(xml).toContain('<loc>https://sightmap.org/blog</loc>')
    expect(xml.match(/<url>/g)).toHaveLength(2)
    expect(xml).not.toContain('undefined')
    expect(xml).toContain('<lastmod>2026-07-28</lastmod>')
  })
})
