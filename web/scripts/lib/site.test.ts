import { describe, it, expect } from 'vitest'
import { SITE_URL, esc, escXml } from './site'

describe('site constants', () => {
  it('has no trailing slash on SITE_URL', () => {
    expect(SITE_URL).toBe('https://sightmap.org')
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
