import { describe, expect, it } from 'vitest'
import {
  isPassthroughPath,
  markdownTwinPath,
  mergeVary,
  negotiate,
  normalizePathname,
  parseAccept,
} from './accept'

describe('parseAccept', () => {
  it('defaults q to 1 and lowercases types', () => {
    expect(parseAccept('Text/Markdown, text/html;q=0.8')).toEqual([
      { type: 'text', subtype: 'markdown', q: 1, specificity: 3 },
      { type: 'text', subtype: 'html', q: 0.8, specificity: 3 },
    ])
  })

  it('treats a missing or empty header as no entries', () => {
    expect(parseAccept(null)).toEqual([])
    expect(parseAccept(undefined)).toEqual([])
    expect(parseAccept('')).toEqual([])
    expect(parseAccept('   ')).toEqual([])
  })
})

describe('negotiate', () => {
  const mdHtml = ['text/markdown', 'text/html'] as const

  it('serves markdown when Accept prefers it', () => {
    expect(negotiate('text/markdown', mdHtml, 'text/html')).toBe('text/markdown')
    expect(negotiate('text/markdown, text/html;q=0.8', mdHtml, 'text/html')).toBe(
      'text/markdown'
    )
  })

  it('serves html when Accept prefers it', () => {
    expect(negotiate('text/html', mdHtml, 'text/html')).toBe('text/html')
  })

  it('honors q=0 refusal of markdown', () => {
    expect(negotiate('text/markdown;q=0, text/html', mdHtml, 'text/html')).toBe('text/html')
  })

  it('returns null (406) when every offered type is refused', () => {
    expect(negotiate('text/markdown;q=0', ['text/markdown'], 'text/markdown')).toBeNull()
    expect(negotiate('application/pdf', mdHtml, 'text/html')).toBeNull()
  })

  it('serves the default when Accept is missing or */*', () => {
    expect(negotiate(null, mdHtml, 'text/html')).toBe('text/html')
    expect(negotiate('*/*', mdHtml, 'text/html')).toBe('text/html')
    expect(negotiate('text/*, */*;q=0.1', mdHtml, 'text/html')).toBe('text/html')
  })

  it('prefers a more specific type at the same q', () => {
    expect(
      negotiate('text/markdown, text/*;q=1, */*;q=1', mdHtml, 'text/html')
    ).toBe('text/markdown')
  })
})

describe('mergeVary', () => {
  it('adds Accept next to an existing Accept-Encoding token', () => {
    expect(mergeVary('accept-encoding', ['Accept'])).toBe('Accept-Encoding, Accept')
  })

  it('does not duplicate Accept', () => {
    expect(mergeVary('Accept, Accept-Encoding', ['Accept', 'Accept-Encoding'])).toBe(
      'Accept, Accept-Encoding'
    )
  })
})

describe('path helpers', () => {
  it('treats hashed assets and extension-bearing files as passthrough', () => {
    expect(isPassthroughPath('/assets/index-abc.js')).toBe(true)
    expect(isPassthroughPath('/openapi.json')).toBe(true)
    expect(isPassthroughPath('/atlas/airbnb.md')).toBe(true)
    expect(isPassthroughPath('/llms.txt')).toBe(true)
    expect(isPassthroughPath('/blog')).toBe(false)
    expect(isPassthroughPath('/')).toBe(false)
  })

  it('maps HTML routes onto their markdown twins', () => {
    expect(markdownTwinPath('/')).toBe('/index.md')
    expect(markdownTwinPath('/blog')).toBe('/blog.md')
    expect(markdownTwinPath('/blog/')).toBe('/blog.md')
    expect(markdownTwinPath('/blog/sightmap')).toBe('/blog/sightmap.md')
    expect(markdownTwinPath('/developers')).toBe('/developers.md')
  })

  it('strips a trailing slash except on the homepage', () => {
    expect(normalizePathname('/')).toBe('/')
    expect(normalizePathname('/developers/')).toBe('/developers')
  })
})
