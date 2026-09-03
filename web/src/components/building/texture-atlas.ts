// Where each authored surface lives inside its atlas.
//
// Shared deliberately between the bake scripts and the running scene: the bake
// writes a tile at these coordinates and the material samples it at these
// coordinates, and two copies of that agreement is two copies that drift into
// a floor rendered with the brushed-steel roughness of the tile next door.
//
// Pure data and arithmetic — no three.js import — so the bake scripts can pull
// it in under tsx without loading WebGL code.

/** A cell in a square grid of tiles, addressed from the top-left. */
export interface Tile {
  readonly col: number
  readonly row: number
}

export interface AtlasLayout {
  /** Edge length of the packed roughness/metalness atlas, in texels. */
  readonly size: number
  /** Tiles per axis. `size / grid` must be a whole number. */
  readonly grid: number
  /**
   * Edge length of the companion normal atlas, in texels. Half `size`.
   *
   * Not an aesthetic choice — a budget one, made against a measurement. The
   * pair transcodes to whatever block format the device offers, and the
   * *worst* case is 8 bits per texel (BC7 on desktop, ASTC 4x4 on iOS): at
   * full resolution the six images land on 4.00 MB, which is not under a 4 MB
   * mobile cap, it *is* the cap. Halving the normals brings the set to about
   * 2.5 MB and leaves room for a device whose transcode target is worse than
   * anything measured here.
   *
   * Halving the normals rather than the ORM maps because of what each one
   * does at this camera. The ORM map is read per texel and its roughness
   * directly drives how the skylight breaks up across a surface. The normal
   * map perturbs shading on surfaces the orthographic camera never brings
   * close enough to resolve as relief — it is here to stop a wall reading as
   * one flat sheen, and it does that just as well at half the texels.
   */
  readonly normalSize: number
}

/**
 * The architectural atlas: the four surfaces the building's fabric is made of.
 * 1024², so each tile is a 512² tiling surface.
 */
export const ARCH: AtlasLayout = { size: 1024, grid: 2, normalSize: 512 }
export const ARCH_TILES = {
  /** Slab decks and the forecourt: cast concrete, fine aggregate speckle. */
  concrete: { col: 0, row: 0 },
  /** Curtain-wall spandrel: painted plaster with a soft orange-peel. */
  plaster: { col: 1, row: 0 },
  /** Mullions, head beams, slab edges, frame: brushed steel, streaked. */
  steel: { col: 0, row: 1 },
  /** Roof deck boards and lounge timber: sawn plank grain. */
  timber: { col: 1, row: 1 },
} as const satisfies Record<string, Tile>

/** The furniture atlas: 512², four 256² tiling surfaces. */
export const FURNITURE: AtlasLayout = { size: 512, grid: 2, normalSize: 256 }
export const FURNITURE_TILES = {
  /** Desks, shelves, counters, table tops. */
  wood: { col: 0, row: 0 },
  /** Pots, rails, partitions, kiosk bodies: matte injection-moulded plastic. */
  plastic: { col: 1, row: 0 },
  /** Monitor and screen bezels: fine-moulded dark polymer. */
  bezel: { col: 0, row: 1 },
  /** Book spines and small props: pressed board. */
  board: { col: 1, row: 1 },
} as const satisfies Record<string, Tile>

/** The foliage-and-fabric atlas: 512², four 256² tiling surfaces. */
export const SOFT: AtlasLayout = { size: 512, grid: 2, normalSize: 256 }
export const SOFT_TILES = {
  /** Component carpets: loop-pile weave. This is the biggest area in the scene. */
  carpet: { col: 0, row: 0 },
  /** Sofas and chairs: upholstery twill. */
  upholstery: { col: 1, row: 0 },
  /** Potted leaves: a mottled canopy. */
  leaf: { col: 0, row: 1 },
  /** Roof-garden bushes: coarser, clumpier foliage. */
  bush: { col: 1, row: 1 },
} as const satisfies Record<string, Tile>

/**
 * How far a tile is inset from its cell edge, in texels at mip 0.
 *
 * An atlased tile sampled with `fract()` has no wrap mode to protect it: the
 * bilinear tap at the tile's edge reaches into its neighbour, and every mip
 * level halves the atlas while the cell boundary stays put, so the reach grows.
 * Insetting the sampled region by a few texels keeps the common mips clean.
 * It does not survive the very coarsest mips — at mip 6 a 1024² atlas is 16²
 * and a cell is 8² — which is the honest cost of one texture over four, and
 * is invisible here because these are low-contrast grain maps on surfaces that
 * never get that small on screen.
 */
export const TILE_INSET_TEXELS = 4

/**
 * The affine transform taking a tiled surface coordinate to an atlas
 * coordinate: `atlasUv = offset + fract(uv * tiling) * scale`.
 *
 * Returned as plain numbers so the shader patch can inline them as constants
 * and the bake can assert against them in a test.
 */
export function tileTransform(layout: AtlasLayout, tile: Tile): { offset: [number, number]; scale: number } {
  const cell = 1 / layout.grid
  // One transform serves both atlases, because the shader computes a single
  // tile coordinate and samples roughness and normal with it. So the inset
  // has to be measured against the *smaller* of the pair: four texels of
  // margin on the 1024 map is only two on the 512 normal beside it, and the
  // inset exists to keep a bilinear tap from reaching into the neighbouring
  // tile on either image.
  const inset = TILE_INSET_TEXELS / Math.min(layout.size, layout.normalSize)
  return {
    offset: [tile.col * cell + inset, tile.row * cell + inset],
    scale: cell - inset * 2,
  }
}

/**
 * The texel rectangle a tile owns, for the bake to fill.
 *
 * `size` overrides the atlas edge length so the same tile can be located on
 * the half-resolution normal atlas as well as the ORM one.
 */
export function tileBounds(
  layout: AtlasLayout,
  tile: Tile,
  atlasSize: number = layout.size
): { x: number; y: number; size: number } {
  const size = atlasSize / layout.grid
  return { x: tile.col * size, y: tile.row * size, size }
}
