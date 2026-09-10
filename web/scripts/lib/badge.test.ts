import { describe, expect, it } from 'vitest'
import { badgeSvg, escapeSvg, listingBadgeSvg, textWidth } from './badge'

describe('escapeSvg', () => {
  it('escapes every character that could close a tag or an attribute', () => {
    expect(escapeSvg(`<g fill="x">&'`)).toBe('&lt;g fill=&quot;x&quot;&gt;&amp;&apos;')
  })

  it('escapes ampersands first so entities are not double-escaped', () => {
    expect(escapeSvg('&lt;')).toBe('&amp;lt;')
  })
})

describe('textWidth', () => {
  it('grows with the string and never returns a fraction', () => {
    const short = textWidth('9 tools')
    const long = textWidth('9 tools · scanned 2026-09-09')
    expect(long).toBeGreaterThan(short)
    expect(Number.isInteger(long)).toBe(true)
  })

  it('gives narrow glyphs less room than wide ones', () => {
    expect(textWidth('iiii')).toBeLessThan(textWidth('MMMM'))
  })
})

describe('badgeSvg', () => {
  it('emits one flat SVG whose blocks add up to its width', () => {
    const svg = badgeSvg({ label: 'WebMCP tools', value: '9 tools' })
    expect(svg.startsWith('<svg xmlns="http://www.w3.org/2000/svg"')).toBe(true)
    expect(svg.trimEnd().endsWith('</svg>')).toBe(true)

    const total = Number(/<svg[^>]*\swidth="(\d+)"/.exec(svg)?.[1])
    const rects = [...svg.matchAll(/<rect(?: x="(\d+)")? width="(\d+)"/g)]
    const left = Number(rects[1][2])
    const right = Number(rects[2][2])
    expect(Number(rects[2][1])).toBe(left)
    expect(left + right).toBe(total)
  })

  it('carries the badge text as a title and an aria-label for screen readers', () => {
    const svg = badgeSvg({ label: 'Atlas', value: 'no tools' })
    expect(svg).toContain('aria-label="Atlas: no tools"')
    expect(svg).toContain('<title>Atlas: no tools</title>')
  })

  it('escapes text rather than letting it become markup', () => {
    const svg = badgeSvg({ label: '<script>', value: 'a & b"' })
    expect(svg).not.toContain('<script>')
    expect(svg).toContain('&lt;script&gt;')
    expect(svg).toContain('a &amp; b&quot;')
  })

  it('collapses whitespace so a newline cannot break the layout', () => {
    const svg = badgeSvg({ label: 'Web\nMCP', value: ' 9  tools ' })
    expect(svg).toContain('>Web MCP<')
    expect(svg).toContain('>9 tools<')
  })

  it('uses a different right-hand colour per tone', () => {
    const withTools = badgeSvg({ label: 'a', value: 'b', tone: 'tools' })
    const without = badgeSvg({ label: 'a', value: 'b', tone: 'none' })
    expect(withTools).not.toBe(without)
  })
})

describe('listingBadgeSvg', () => {
  it('claims WebMCP tools with a count and the scan date', () => {
    const svg = listingBadgeSvg({ counts: { tools: 9 }, scannedAt: '2026-09-09T20:00:00.000Z' })
    expect(svg).toContain('>WebMCP tools<')
    expect(svg).toContain('>9 tools · scanned 2026-09-09<')
  })

  it('says "1 tool", not "1 tools"', () => {
    const svg = listingBadgeSvg({ counts: { tools: 1 }, scannedAt: '2026-09-09T20:00:00.000Z' })
    expect(svg).toContain('>1 tool · scanned 2026-09-09<')
  })

  it('does not advertise WebMCP when the scan found no tools', () => {
    const svg = listingBadgeSvg({ counts: { tools: 0 }, scannedAt: '2026-09-09T20:00:00.000Z' })
    expect(svg).toContain('>Atlas<')
    expect(svg).toContain('>no tools · scanned 2026-09-09<')
    expect(svg).not.toContain('WebMCP')
  })
})
