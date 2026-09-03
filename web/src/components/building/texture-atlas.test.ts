import { describe, expect, it } from 'vitest'
import {
  ARCH,
  ARCH_TILES,
  FURNITURE,
  FURNITURE_TILES,
  SOFT,
  SOFT_TILES,
  TILE_INSET_TEXELS,
  tileBounds,
  tileTransform,
  type AtlasLayout,
  type Tile,
} from './texture-atlas'

const ATLASES: [string, AtlasLayout, Record<string, Tile>][] = [
  ['arch', ARCH, ARCH_TILES],
  ['furniture', FURNITURE, FURNITURE_TILES],
  ['soft', SOFT, SOFT_TILES],
]

describe.each(ATLASES)('%s atlas', (_name, layout, tiles) => {
  it('divides into whole texels', () => {
    expect(layout.size % layout.grid).toBe(0)
  })

  it('gives every surface its own cell', () => {
    const cells = Object.values(tiles).map((t) => `${t.col},${t.row}`)
    expect(new Set(cells).size).toBe(cells.length)
  })

  it('keeps every surface inside the atlas', () => {
    for (const tile of Object.values(tiles)) {
      expect(tile.col).toBeLessThan(layout.grid)
      expect(tile.row).toBeLessThan(layout.grid)
    }
  })

  it('never samples outside the tile it addresses', () => {
    // The property the whole atlas rests on: for any surface coordinate, the
    // transform lands inside this tile's cell and not in its neighbour's. A
    // violation here is a floor rendered with the roughness of a bush.
    const cell = 1 / layout.grid
    for (const tile of Object.values(tiles)) {
      const { offset, scale } = tileTransform(layout, tile)
      for (const f of [0, 0.5, 0.999999]) {
        const u = offset[0] + f * scale
        const v = offset[1] + f * scale
        expect(u).toBeGreaterThanOrEqual(tile.col * cell)
        expect(u).toBeLessThanOrEqual((tile.col + 1) * cell)
        expect(v).toBeGreaterThanOrEqual(tile.row * cell)
        expect(v).toBeLessThanOrEqual((tile.row + 1) * cell)
      }
    }
  })

  it('insets by the stated margin on both sides', () => {
    const { offset, scale } = tileTransform(layout, { col: 0, row: 0 })
    const inset = TILE_INSET_TEXELS / layout.size
    expect(offset[0]).toBeCloseTo(inset, 12)
    expect(scale).toBeCloseTo(1 / layout.grid - inset * 2, 12)
  })

  it('bounds the bake to the same cell the shader reads', () => {
    for (const tile of Object.values(tiles)) {
      const { x, y, size } = tileBounds(layout, tile)
      const { offset } = tileTransform(layout, tile)
      // The bake fills from the cell corner; the shader starts one inset in.
      expect(offset[0] * layout.size).toBeCloseTo(x + TILE_INSET_TEXELS, 6)
      expect(offset[1] * layout.size).toBeCloseTo(y + TILE_INSET_TEXELS, 6)
      expect(x + size).toBeLessThanOrEqual(layout.size)
      expect(y + size).toBeLessThanOrEqual(layout.size)
    }
  })
})

describe('atlas budget', () => {
  it('holds the sizes the memory budget was calculated from', () => {
    // Raising any of these raises resident texture memory quadratically, and
    // the mobile ceiling is 4 MB decoded. Change deliberately, and re-measure
    // with scripts/capture-building.mjs before committing.
    expect(ARCH.size).toBe(1024)
    expect(FURNITURE.size).toBe(512)
    expect(SOFT.size).toBe(512)
  })
})
