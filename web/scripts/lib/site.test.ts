import { describe, it, expect } from 'vitest'
import { SITE_URL, SITE_NAME, HOME_TITLE, BLOG_INDEX_TITLE, postTitle, esc, escXml } from './site'

describe('site constants', () => {
  it('has no trailing slash on SITE_URL', () => {
    expect(SITE_URL).toBe('https://sightmap.org')
  })
})

// scripts/prerender.tsx (crawlers/unfurlers) and src/components/Seo.tsx
// (client-side navigation) both build <title> from these exports rather than
// inlining their own template strings — see the comment on HOME_TITLE in
// site.ts. Pinning the literal output here guards against the two silently
// drifting into different titles for the same route.
describe('title builders', () => {
  it('builds the homepage title from SITE_NAME', () => {
    expect(HOME_TITLE).toBe(`${SITE_NAME} — runtime context for agents using your web app`)
  })

  it('builds the blog index title from SITE_NAME', () => {
    expect(BLOG_INDEX_TITLE).toBe(`Blog — ${SITE_NAME}`)
  })

  it('builds a post title by appending SITE_NAME', () => {
    expect(postTitle('Announcing widgets')).toBe(`Announcing widgets — ${SITE_NAME}`)
  })
})

describe('esc', () => {
  it('escapes the four HTML-attribute-unsafe characters', () => {
    expect(esc(`<a href="x">&'`)).toBe('&lt;a href=&quot;x&quot;&gt;&amp;\'')
  })

  it('escapes ampersands before other entities so they are not double-escaped', () => {
    expect(esc('&lt;')).toBe('&amp;lt;')
  })
})

describe('escXml', () => {
  it('escapes apostrophes as well, which esc does not', () => {
    expect(escXml(`it's`)).toBe('it&apos;s')
  })
})
