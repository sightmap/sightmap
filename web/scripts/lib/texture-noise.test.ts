import { describe, expect, it } from 'vitest'
import { fbm, heightFieldAo, heightToNormal, valueNoise } from './texture-noise'

// These four properties are the ones the baked atlases actually depend on. The
// look of the noise is judged by eye against a render; what a test can hold is
// that it tiles, that it is deterministic, and that the two derived maps mean
// what the materials assume they mean.

describe('valueNoise', () => {
  it('is periodic on the unit square, so an atlas tile has no seam', () => {
    for (const t of [0, 0.13, 0.37, 0.62, 0.99]) {
      expect(valueNoise(0, t, 8, 8, 5)).toBeCloseTo(valueNoise(1, t, 8, 8, 5), 12)
      expect(valueNoise(t, 0, 8, 8, 5)).toBeCloseTo(valueNoise(t, 1, 8, 8, 5), 12)
    }
  })

  it('wraps at the period with an anisotropic lattice too', () => {
    // Brushed steel and timber both rely on this: 6 across, 220 along.
    expect(valueNoise(0, 0.4, 6, 220, 137)).toBeCloseTo(valueNoise(1, 0.4, 6, 220, 137), 12)
    expect(valueNoise(0.4, 0, 6, 220, 137)).toBeCloseTo(valueNoise(0.4, 1, 6, 220, 137), 12)
  })

  it('stays inside [0, 1]', () => {
    for (let i = 0; i < 500; i++) {
      const n = valueNoise((i * 7) % 100 / 100, (i * 13) % 100 / 100, 12, 5, 3)
      expect(n).toBeGreaterThanOrEqual(0)
      expect(n).toBeLessThanOrEqual(1)
    }
  })

  it('is deterministic — the committed .ktx2 files must be reproducible', () => {
    expect(valueNoise(0.31, 0.77, 9, 9, 42)).toBe(valueNoise(0.31, 0.77, 9, 9, 42))
    expect(valueNoise(0.31, 0.77, 9, 9, 42)).not.toBe(valueNoise(0.31, 0.77, 9, 9, 43))
  })
})

describe('fbm', () => {
  it('tiles, because every octave doubles a period that already tiled', () => {
    for (const t of [0.08, 0.44, 0.91]) {
      expect(fbm(0, t, 6, 6, 4, 17)).toBeCloseTo(fbm(1, t, 6, 6, 4, 17), 12)
      expect(fbm(t, 0, 6, 6, 4, 17)).toBeCloseTo(fbm(t, 1, 6, 6, 4, 17), 12)
    }
  })

  it('stays normalised as octaves are added', () => {
    for (const octaves of [1, 2, 3, 4, 5]) {
      const n = fbm(0.27, 0.61, 5, 5, octaves, 3)
      expect(n).toBeGreaterThanOrEqual(0)
      expect(n).toBeLessThanOrEqual(1)
    }
  })
})

describe('heightToNormal', () => {
  it('returns the flat normal for a flat surface', () => {
    const [r, g, b] = heightToNormal(() => 0.5, 4, 4, 2)
    // +Z encodes to the middle of R and G and the top of B: the familiar
    // lavender of an unperturbed tangent-space normal map.
    expect(r).toBe(128)
    expect(g).toBe(128)
    expect(b).toBe(255)
  })

  it('tilts away from rising ground', () => {
    // A ramp climbing in +x must produce a normal leaning in -x, or every lit
    // surface in the scene gets its shading gradient backwards.
    const ramp = (x: number): number => x * 0.1
    const [r, g, b] = heightToNormal((x) => ramp(x), 4, 4, 1)
    expect(r).toBeLessThan(128)
    expect(g).toBe(128)
    expect(b).toBeGreaterThan(128)
  })

  it('scales the tilt with strength', () => {
    const ramp = (x: number): number => x * 0.1
    const gentle = heightToNormal(ramp, 4, 4, 1)[0]
    const steep = heightToNormal(ramp, 4, 4, 4)[0]
    expect(steep).toBeLessThan(gentle)
  })
})

describe('heightFieldAo', () => {
  const options = { radii: [2, 5, 9], spokes: 8, strength: 1, saturation: 0.6 }

  it('leaves an open plane unoccluded', () => {
    expect(heightFieldAo(() => 0, 50, 50, 8, options)).toBe(1)
  })

  it('darkens ground that sits beside a tall neighbour', () => {
    // Everything to the right of x=50 is a wall 1 unit high.
    const wall = (x: number): number => (x > 50 ? 1 : 0)
    const atWall = heightFieldAo((x) => wall(x), 49, 50, 8, options)
    const away = heightFieldAo((x) => wall(x), 20, 50, 8, options)
    expect(atWall).toBeLessThan(0.85)
    expect(away).toBe(1)
  })

  it('falls off with distance — this is what reads as contact', () => {
    // The whole point of the bake: the crease is dark and a metre away is not.
    const block = (x: number, y: number): number => (Math.abs(x - 50) < 6 && Math.abs(y - 50) < 6 ? 0.8 : 0)
    const near = heightFieldAo(block, 57, 50, 8, options)
    const far = heightFieldAo(block, 64, 50, 8, options)
    expect(near).toBeLessThan(far)
    expect(far).toBeLessThanOrEqual(1)
  })

  it('never returns a negative multiplier, whatever the occluder', () => {
    const canyon = (x: number, y: number): number => (x === 50 && y === 50 ? 0 : 40)
    const ao = heightFieldAo(canyon, 50, 50, 8, options)
    expect(ao).toBeGreaterThanOrEqual(0)
    expect(ao).toBeLessThanOrEqual(1)
  })

  it('honours strength as the floor on darkening', () => {
    const wall = (x: number): number => (x > 50 ? 1 : 0)
    const full = heightFieldAo((x) => wall(x), 49, 50, 8, options)
    const half = heightFieldAo((x) => wall(x), 49, 50, 8, { ...options, strength: 0.5 })
    expect(half).toBeGreaterThan(full)
    expect(1 - half).toBeCloseTo((1 - full) / 2, 10)
  })
})
