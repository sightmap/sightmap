import { describe, expect, it } from 'vitest'
import { drawSignAtlas, packSigns, type SignContext, type SignItem } from './signs'

const items = (n: number): SignItem[] =>
  Array.from({ length: n }, (_, i) => ({
    text: `listing-${i}`,
    // Deliberately uneven: a shelf packer that only works on equal plates is
    // not a packer, and real signs are as long as the site's name.
    width: 90 + ((i * 37) % 260),
    height: 56,
  }))

const overlaps = (
  a: { x: number; y: number; width: number; height: number },
  b: { x: number; y: number; width: number; height: number }
): boolean =>
  a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height

describe('packSigns', () => {
  it('places every sign inside the texture without overlapping another', () => {
    const atlas = packSigns(items(60), { width: 1024, pad: 2 })
    expect(atlas.regions).toHaveLength(60)
    for (const r of atlas.regions) {
      expect(r.x).toBeGreaterThanOrEqual(0)
      expect(r.y).toBeGreaterThanOrEqual(0)
      expect(r.x + r.width).toBeLessThanOrEqual(atlas.width)
      expect(r.y + r.height).toBeLessThanOrEqual(atlas.height)
    }
    for (let i = 0; i < atlas.regions.length; i++) {
      for (let j = i + 1; j < atlas.regions.length; j++) {
        expect(overlaps(atlas.regions[i], atlas.regions[j])).toBe(false)
      }
    }
  })

  it('gives each sign texture coordinates for its own region', () => {
    const atlas = packSigns(items(8), { width: 256, pad: 2 })
    for (const r of atlas.regions) {
      expect(r.u).toBeCloseTo(r.x / atlas.width, 6)
      expect(r.uw).toBeCloseTo(r.width / atlas.width, 6)
      expect(r.v + r.vh).toBeCloseTo(1 - r.y / atlas.height, 6)
      expect(atlas.regions[atlas.index.get(r.text) as number].text).toBe(r.text)
    }
    expect(atlas.height).toBe(2 ** Math.round(Math.log2(atlas.height)))
  })

  it('keeps the order it was given, so a new listing moves no other sign', () => {
    const first = packSigns(items(5))
    const second = packSigns([...items(5), { text: 'newcomer', width: 120, height: 56 }])
    for (let i = 0; i < 5; i++) {
      expect(second.regions[i].x).toBe(first.regions[i].x)
      expect(second.regions[i].y).toBe(first.regions[i].y)
    }
  })
})

describe('drawSignAtlas', () => {
  it('draws a plate and its text for every sign', () => {
    const drawn: string[] = []
    const ctx: SignContext = {
      fillStyle: '',
      font: '',
      textAlign: 'start',
      textBaseline: 'alphabetic',
      clearRect: () => void drawn.push('clear'),
      fillRect: () => void drawn.push('plate'),
      fillText: (text) => void drawn.push(`text:${text}`),
    }
    const atlas = packSigns(items(4))
    drawSignAtlas(ctx, atlas, { font: '16px mono', plate: '#000', ink: '#fff' })
    expect(drawn.filter((d) => d === 'plate')).toHaveLength(4)
    expect(drawn).toContain('text:listing-3')
  })
})
