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

  it('insets by the stated margin measured on the smaller of the two atlases', () => {
    const { offset, scale } = tileTransform(layout, { col: 0, row: 0 })
    // One tile coordinate samples both the ORM atlas and the half-resolution
    // normal atlas, so the margin has to be big enough on the normal one.
    const inset = TILE_INSET_TEXELS / layout.normalSize
    expect(offset[0]).toBeCloseTo(inset, 12)
    expect(scale).toBeCloseTo(1 / layout.grid - inset * 2, 12)
  })

  it('bounds the bake to the same cell the shader reads, on both atlases', () => {
    for (const tile of Object.values(tiles)) {
      const { offset } = tileTransform(layout, tile)
      for (const atlasSize of [layout.size, layout.normalSize]) {
        const { x, y, size } = tileBounds(layout, tile, atlasSize)
        // The bake fills from the cell corner; the shader starts one inset in.
        // Expressed in this atlas's own texels, that inset is whatever the
        // shared fractional margin comes to here — at least a full texel on
        // the smaller atlas, which is the point of measuring it there.
        expect(offset[0] * atlasSize).toBeGreaterThanOrEqual(x + 1)
        expect(offset[1] * atlasSize).toBeGreaterThanOrEqual(y + 1)
        expect((offset[0] + scaleOf(layout, tile)) * atlasSize).toBeLessThanOrEqual(x + size - 1)
        expect(x + size).toBeLessThanOrEqual(atlasSize)
        expect(y + size).toBeLessThanOrEqual(atlasSize)
      }
    }
  })

  it('keeps the normal atlas at half the ORM atlas, tiled the same way', () => {
    expect(layout.normalSize).toBe(layout.size / 2)
    expect(layout.normalSize % layout.grid).toBe(0)
  })
})

const scaleOf = (l: Parameters<typeof tileTransform>[0], t: Parameters<typeof tileTransform>[1]) =>
  tileTransform(l, t).scale

describe('atlas budget', () => {
  it('holds the sizes the memory budget was calculated from', () => {
    // Raising any of these raises resident texture memory quadratically, and
    // the mobile ceiling is 4 MB decoded. Change deliberately, and re-measure
    // with scripts/capture-building.mjs before committing.
    expect(ARCH.size).toBe(1024)
    expect(FURNITURE.size).toBe(512)
    expect(SOFT.size).toBe(512)
  })

  it('stays under the 4 MB mobile ceiling at the worst-case block rate', () => {
    // Every atlas transcodes to whatever the device supports, and the formats
    // at the top of three's preference list — BC7 on desktop, ASTC 4x4 on
    // modern iOS — are 8 bits per texel. A full mip chain adds a third. This
    // is the number that has to fit, not the 303 KiB on disk.
    const bytes = [ARCH, FURNITURE, SOFT].reduce(
      (n, l) => n + (l.size * l.size + l.normalSize * l.normalSize) * (4 / 3),
      0
    )
    expect(bytes).toBeLessThan(4 * 1024 * 1024)
    // And with enough room left that a future surface does not silently bust
    // the cap: 2.50 MB when this was written.
    expect(bytes).toBeLessThan(3 * 1024 * 1024)
  })
})
