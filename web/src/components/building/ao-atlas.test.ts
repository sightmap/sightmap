import { describe, expect, it } from 'vitest'

import { AO_ATLAS, AO_WHITE_CELL, aoCellRect, aoUvArray, aoUvForPoint } from './ao-atlas'

const W = 10
const D = 7

describe('aoCellRect', () => {
  it('gives every floor a disjoint patch of the atlas', () => {
    const rects = Array.from({ length: 6 }, (_, i) => aoCellRect(i))
    for (let a = 0; a < rects.length; a++) {
      for (let b = a + 1; b < rects.length; b++) {
        const overlapU = rects[a].u < rects[b].u + rects[b].du && rects[b].u < rects[a].u + rects[a].du
        const overlapV = rects[a].v < rects[b].v + rects[b].dv && rects[b].v < rects[a].v + rects[a].dv
        expect(overlapU && overlapV).toBe(false)
      }
    }
  })

  it('walks the grid in row-major order', () => {
    // Cell 4 wraps to the second row, directly under cell 0.
    expect(aoCellRect(4).u).toBeCloseTo(aoCellRect(0).u, 6)
    expect(aoCellRect(4).v).toBeGreaterThan(aoCellRect(0).v)
    // Cell 1 is one column across on the same row.
    expect(aoCellRect(1).v).toBeCloseTo(aoCellRect(0).v, 6)
    expect(aoCellRect(1).u).toBeGreaterThan(aoCellRect(0).u)
  })

  it('stays inside the atlas and insets off the cell boundary', () => {
    for (let i = 0; i < AO_ATLAS.cols * AO_ATLAS.rows; i++) {
      const r = aoCellRect(i)
      expect(r.u).toBeGreaterThan(0 - Number.EPSILON)
      expect(r.v).toBeGreaterThan(0 - Number.EPSILON)
      expect(r.u + r.du).toBeLessThanOrEqual(1)
      expect(r.v + r.dv).toBeLessThanOrEqual(1)
    }
    // The inset is what stops a bilinear tap at the seam pulling in the
    // neighbouring floor, so a cell must be strictly narrower than its slot.
    expect(aoCellRect(0).du).toBeLessThan(AO_ATLAS.cell / AO_ATLAS.width)
  })
})

describe('aoUvForPoint', () => {
  it('maps the floor centre to the centre of its cell', () => {
    const r = aoCellRect(2)
    const [u, v] = aoUvForPoint(0, 0, W, D, 2)
    expect(u).toBeCloseTo(r.u + r.du / 2, 6)
    expect(v).toBeCloseTo(r.v + r.dv / 2, 6)
  })

  it('maps the floor corners to the corners of its cell', () => {
    const r = aoCellRect(3)
    expect(aoUvForPoint(-W / 2, -D / 2, W, D, 3)).toEqual([r.u, r.v])
    const [u, v] = aoUvForPoint(W / 2, D / 2, W, D, 3)
    expect(u).toBeCloseTo(r.u + r.du, 6)
    expect(v).toBeCloseTo(r.v + r.dv, 6)
  })

  it('increases u with +x and v with +z, so the bake is not mirrored', () => {
    const [uLeft] = aoUvForPoint(-3, 0, W, D, 0)
    const [uRight] = aoUvForPoint(3, 0, W, D, 0)
    expect(uRight).toBeGreaterThan(uLeft)
    const [, vNear] = aoUvForPoint(0, -2, W, D, 0)
    const [, vFar] = aoUvForPoint(0, 2, W, D, 0)
    expect(vFar).toBeGreaterThan(vNear)
  })

  it('clamps a point beyond the plate into its own cell rather than a neighbour', () => {
    const r = aoCellRect(1)
    const [u, v] = aoUvForPoint(W, D, W, D, 1)
    expect(u).toBeCloseTo(r.u + r.du, 6)
    expect(v).toBeCloseTo(r.v + r.dv, 6)
    const [u2, v2] = aoUvForPoint(-W, -D, W, D, 1)
    expect(u2).toBeCloseTo(r.u, 6)
    expect(v2).toBeCloseTo(r.v, 6)
  })

  it('puts different floors at different texels for the same floor position', () => {
    expect(aoUvForPoint(1, 1, W, D, 0)).not.toEqual(aoUvForPoint(1, 1, W, D, 1))
  })
})

describe('aoUvArray', () => {
  it('produces two floats per vertex, ignoring height', () => {
    // Two vertices at the same XZ but different Y must land on the same texel:
    // the unwrap is top-down, so height carries no information.
    const positions = new Float32Array([1, 0, 2, 1, 5, 2])
    const uv = aoUvArray(positions, W, D, 0)
    expect(uv).toHaveLength(4)
    expect([uv[0], uv[1]]).toEqual([uv[2], uv[3]])
  })

  it('agrees with aoUvForPoint vertex by vertex', () => {
    const positions = new Float32Array([-4, 0, -3, 0, 1, 0, 4.5, 2, 3])
    const uv = aoUvArray(positions, W, D, 5)
    for (let i = 0; i < 3; i++) {
      const [u, v] = aoUvForPoint(positions[i * 3], positions[i * 3 + 2], W, D, 5)
      expect(uv[i * 2]).toBeCloseTo(u, 6)
      expect(uv[i * 2 + 1]).toBeCloseTo(v, 6)
    }
  })

  it('keeps the white cell clear of every floor cell', () => {
    // Non-floor geometry merged beside a slab aims here. If it overlapped a
    // floor's cell, a curtain wall would wear that floor's furniture shadows.
    const white = aoCellRect(AO_WHITE_CELL)
    for (let i = 0; i < 6; i++) {
      const r = aoCellRect(i)
      const overlapU = r.u < white.u + white.du && white.u < r.u + r.du
      const overlapV = r.v < white.v + white.dv && white.v < r.v + r.dv
      expect(overlapU && overlapV).toBe(false)
    }
  })
})
