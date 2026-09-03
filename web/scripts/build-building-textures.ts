// Bakes the building's three surface atlases and writes them as KTX2/Basis.
//
// Why procedural rather than photographed: the scene is a white architectural
// model under a studio sky, and what it lacks is not imagery — it is *grain*.
// Every surface currently returns exactly one roughness at every texel, so the
// environment map lands on each one as a single flat sheen and the model reads
// as injection-moulded plastic. A tiling roughness/normal pair per surface
// family breaks that sheen up. Photographic PBR sets would do the same job and
// cost forty times the memory for detail an orthographic camera at this zoom
// never resolves.
//
// Channels follow three's own convention so one texture can serve several map
// slots without a second upload:
//   R — unused (the ambient occlusion in this scene is per-floor and baked
//       separately, against geometry rather than against a tiling surface)
//   G — roughness, read by `roughnessMap`
//   B — metalness, read by `metalnessMap`
// Both are *modulations*, centred so that 255 leaves the material's authored
// value alone. The roughness values already in Tower.tsx were chosen against
// this lighting and are not up for renegotiation by a texture.
//
// Run on demand — NOT part of `pnpm build`. The committed .ktx2 files are the
// artifact; regenerate and commit them when you change the surfaces below:
//   cd web && pnpm tsx scripts/build-building-textures.ts
// Needs the Khronos `ktx` CLI on PATH (or KTX_BIN pointing at it).
import { resolve } from 'node:path'
import { rmSync } from 'node:fs'
import { put, raster, type Raster } from './lib/png'
import { writeKtx2 } from './lib/ktx'
import { fbm, heightToNormal, valueNoise } from './lib/texture-noise'
import {
  ARCH,
  ARCH_TILES,
  FURNITURE,
  FURNITURE_TILES,
  SOFT,
  SOFT_TILES,
  tileBounds,
  type AtlasLayout,
  type Tile,
} from '../src/components/building/texture-atlas'

const OUT_DIR = resolve(import.meta.dirname, '../public/building/textures')

/**
 * One tiling surface.
 *
 * `height` is what the normal map is differentiated from, in arbitrary units;
 * only its gradient matters. `roughness` and `metalness` are multipliers on the
 * material's own value, in [0, 1].
 */
interface Surface {
  tile: Tile
  /** Peak-to-trough scale of the normal. Grain, not displacement. */
  relief: number
  height: (u: number, v: number) => number
  roughness: (u: number, v: number) => number
  metalness?: (u: number, v: number) => number
}

const mix = (a: number, b: number, t: number): number => a + (b - a) * t
/** Distance to the nearest edge of the cell `t` falls in, in cell units. */
const edge = (t: number): number => Math.min(t % 1, 1 - (t % 1))

// ---------------------------------------------------------------------------
// Architectural surfaces.

const concrete: Surface = {
  tile: ARCH_TILES.concrete,
  relief: 1.4,
  // Aggregate: dense fine speckle over a slow tonal drift, so a whole slab
  // never reads as one flat sheet under a grazing sun.
  height: (u, v) => fbm(u, v, 48, 48, 3, 11) * 0.55 + fbm(u, v, 4, 4, 2, 29) * 0.45,
  roughness: (u, v) => mix(0.86, 1, fbm(u, v, 24, 24, 3, 11)),
}

const plaster: Surface = {
  tile: ARCH_TILES.plaster,
  relief: 0.9,
  // Orange peel: one broad wobble, nothing sharp. Spandrel is painted board.
  height: (u, v) => fbm(u, v, 12, 12, 3, 71),
  roughness: (u, v) => mix(0.92, 1, fbm(u, v, 10, 10, 2, 71)),
}

const steel: Surface = {
  tile: ARCH_TILES.steel,
  relief: 0.7,
  // Brushed: high frequency across the grain, almost none along it. The
  // anisotropy is the whole read — an isotropic metal is a mirror ball.
  height: (u, v) => valueNoise(u, v, 6, 220, 137) * 0.7 + fbm(u, v, 3, 24, 2, 151) * 0.3,
  roughness: (u, v) => mix(0.72, 1.0, valueNoise(u, v, 4, 180, 137)),
  metalness: () => 1,
}

const TIMBER_PLANKS = 6

const timber: Surface = {
  tile: ARCH_TILES.timber,
  relief: 2.2,
  // Sawn boards: six planks across v, grain along u. Two things make this read
  // as boards rather than as streaked noise — a groove at each joint, and a
  // small per-plank height offset, because real boards are never laid perfectly
  // flush and the catchlight running along the step is what the eye picks up.
  height: (u, v) => {
    const grain = valueNoise(u, v, 5, 90, 211) * 0.6 + fbm(u, v, 8, 30, 2, 233) * 0.4
    const groove = Math.min(1, edge(v * TIMBER_PLANKS) / 0.02)
    const plank = valueNoise(0.5, (Math.floor(v * TIMBER_PLANKS) + 0.5) / TIMBER_PLANKS, 1, TIMBER_PLANKS, 251)
    return grain * 0.55 + groove * 0.3 + plank * 0.15
  },
  roughness: (u, v) => mix(0.8, 1, valueNoise(u, v, 6, 60, 211)),
}

// ---------------------------------------------------------------------------
// Furniture surfaces.

const wood: Surface = {
  tile: FURNITURE_TILES.wood,
  relief: 1.1,
  height: (u, v) => valueNoise(u, v, 4, 64, 307) * 0.7 + fbm(u, v, 6, 20, 2, 311) * 0.3,
  roughness: (u, v) => mix(0.72, 1, valueNoise(u, v, 5, 40, 307)),
}

const plastic: Surface = {
  tile: FURNITURE_TILES.plastic,
  relief: 0.5,
  // Moulded matte: a very fine even stipple, the texture of a moulding tool.
  height: (u, v) => fbm(u, v, 64, 64, 2, 401),
  roughness: (u, v) => mix(0.9, 1, fbm(u, v, 32, 32, 2, 401)),
}

const bezel: Surface = {
  tile: FURNITURE_TILES.bezel,
  relief: 0.4,
  height: (u, v) => fbm(u, v, 96, 96, 2, 503),
  roughness: (u, v) => mix(0.82, 1, fbm(u, v, 48, 48, 2, 503)),
}

const board: Surface = {
  tile: FURNITURE_TILES.board,
  relief: 0.9,
  height: (u, v) => fbm(u, v, 20, 20, 3, 601),
  roughness: (u, v) => mix(0.85, 1, fbm(u, v, 16, 16, 2, 601)),
}

// ---------------------------------------------------------------------------
// Foliage and fabric.
//
// The carpet tile covers more square metres than anything else in the scene:
// every component's zone plate is one, on all six floors.

const weave = (u: number, v: number, period: number, seed: number): number => {
  // Two out-of-phase ridge trains crossing at right angles: a loop pile read
  // from above. Cheaper and more regular than noise, which is the point —
  // a woven surface is regular, and noise would read as dirt.
  const a = Math.sin(u * period * Math.PI * 2) * 0.5 + 0.5
  const b = Math.sin(v * period * Math.PI * 2) * 0.5 + 0.5
  return (a * b * 0.7 + fbm(u, v, period, period, 2, seed) * 0.3)
}

// Weave periods are deliberately kept well clear of the texel rate. At 40 loops
// across a 256² tile a loop is six texels wide, which is close enough to Nyquist
// that the mip chain turns it into crawling moiré under a moving camera — the
// same shimmer the spec rejects screen-space AO to avoid, arriving by a
// different door. Twenty-four loops is eleven texels each and filters cleanly.
const carpet: Surface = {
  tile: SOFT_TILES.carpet,
  relief: 1.3,
  height: (u, v) => weave(u, v, 24, 701),
  roughness: () => 1,
}

const upholstery: Surface = {
  tile: SOFT_TILES.upholstery,
  relief: 1.0,
  // Twill: the ridge train runs on the diagonal.
  height: (u, v) => weave(u + v, u - v, 16, 809) * 0.8 + fbm(u, v, 32, 32, 2, 811) * 0.2,
  roughness: (u, v) => mix(0.93, 1, fbm(u, v, 24, 24, 2, 809)),
}

// The two foliage tiles have to differ in *scale*, not just in seed. A potted
// plant is read from a metre away and a roof bush from ten, so the pot gets fine
// separated leaves and the bush gets a few large clumps; seeded variations of
// one frequency would land on the model as the same surface twice.
const leaf: Surface = {
  tile: SOFT_TILES.leaf,
  relief: 2.0,
  height: (u, v) => fbm(u, v, 11, 11, 2, 907) * 0.45 + fbm(u, v, 34, 34, 2, 911) * 0.55,
  roughness: () => 1,
}

const bush: Surface = {
  tile: SOFT_TILES.bush,
  relief: 3.4,
  height: (u, v) => fbm(u, v, 3, 3, 2, 1009) * 0.75 + fbm(u, v, 12, 12, 3, 1013) * 0.25,
  roughness: () => 1,
}

// ---------------------------------------------------------------------------

/**
 * Draw `surfaces` into a fresh pair of atlas images: one packed
 * roughness/metalness map and one tangent-space normal map.
 *
 * The height field is evaluated once per texel into a scratch buffer, then
 * differentiated, because the normal at a texel needs its neighbours and
 * re-evaluating fbm four times per texel is four times the bake for the same
 * bytes. Neighbour reads wrap within the tile, which is what keeps the normal
 * seamless across the `fract()` boundary the shader samples at.
 */
function bakeAtlas(layout: AtlasLayout, surfaces: Surface[]): { orm: Raster; normal: Raster } {
  const orm = raster(layout.size, layout.size, 3)
  const normal = raster(layout.normalSize, layout.normalSize, 3)
  for (const s of surfaces) {
    const ormCell = tileBounds(layout, s.tile)
    for (let y = 0; y < ormCell.size; y++) {
      for (let x = 0; x < ormCell.size; x++) {
        const u = (x + 0.5) / ormCell.size
        const v = (y + 0.5) / ormCell.size
        put(orm, ormCell.x + x, ormCell.y + y, [255, s.roughness(u, v) * 255, (s.metalness?.(u, v) ?? 1) * 255])
      }
    }

    // The normal is re-derived at its own resolution rather than downsampled
    // from a full-resolution one. Averaging neighbouring normals shortens the
    // vector and flattens the surface unevenly; re-evaluating the height field
    // on the coarser grid gives an honest gradient at that scale.
    const nCell = tileBounds(layout, s.tile, layout.normalSize)
    const heights = new Float32Array(nCell.size * nCell.size)
    for (let y = 0; y < nCell.size; y++) {
      for (let x = 0; x < nCell.size; x++) {
        heights[y * nCell.size + x] = s.height((x + 0.5) / nCell.size, (y + 0.5) / nCell.size)
      }
    }
    const at = (x: number, y: number): number => {
      const wx = ((Math.round(x) % nCell.size) + nCell.size) % nCell.size
      const wy = ((Math.round(y) % nCell.size) + nCell.size) % nCell.size
      return heights[wy * nCell.size + wx]
    }
    for (let y = 0; y < nCell.size; y++) {
      for (let x = 0; x < nCell.size; x++) {
        // Halving the grid halves every finite difference, so the relief is
        // scaled back up to keep the surface as deep as it was authored.
        put(normal, nCell.x + x, nCell.y + y, heightToNormal(at, x, y, s.relief * (layout.size / layout.normalSize)))
      }
    }
  }
  return { orm, normal }
}

interface Baked {
  name: string
  layout: AtlasLayout
  ormBytes: number
  normalBytes: number
}

function bake(name: string, layout: AtlasLayout, surfaces: Surface[]): Baked {
  const { orm, normal } = bakeAtlas(layout, surfaces)
  // Both are non-colour data: an sRGB curve applied to a roughness value is
  // simply a different, wrong roughness value.
  const ormBytes = writeKtx2(orm, `${OUT_DIR}/${name}-orm.ktx2`, 'linear')
  const normalBytes = writeKtx2(normal, `${OUT_DIR}/${name}-normal.ktx2`, 'linear')
  return { name, layout, ormBytes, normalBytes }
}

/**
 * What an atlas pair costs once transcoded and resident, in bytes.
 *
 * One byte per texel, plus a third for the mip chain. A byte is the *worst*
 * case and therefore the one worth budgeting against: an ETC1S payload
 * transcodes to whatever the device supports, and the 8-bits-per-texel targets
 * (BC7, ASTC 4x4) sit at the top of three's preference list, so they are what
 * desktop and modern iOS actually get. Measured, not assumed — a capture of
 * the running page reported 0x8E8C (BC7) for every atlas.
 *
 * Advisory only: the number that governs the budget is the one measured at the
 * GL upload boundary in the browser, not predicted here.
 */
const decodedEstimate = (layout: AtlasLayout): number =>
  (layout.size * layout.size + layout.normalSize * layout.normalSize) * (4 / 3)

const baked = [
  bake('arch', ARCH, [concrete, plaster, steel, timber]),
  bake('furniture', FURNITURE, [wood, plastic, bezel, board]),
  bake('soft', SOFT, [carpet, upholstery, leaf, bush]),
]

// The PNGs are an intermediate for the encoder, not an artifact.
for (const b of baked) {
  for (const kind of ['orm', 'normal']) rmSync(`${OUT_DIR}/${b.name}-${kind}.ktx2.src.png`, { force: true })
}

const kb = (n: number): string => `${(n / 1024).toFixed(1)} KiB`
let onDisk = 0
let decoded = 0
for (const b of baked) {
  onDisk += b.ormBytes + b.normalBytes
  // No doubling: decodedEstimate already covers both images of the pair.
  decoded += decodedEstimate(b.layout)
  console.log(
    `${b.name.padEnd(10)} ${String(b.layout.size).padStart(4)}²  orm ${kb(b.ormBytes).padStart(10)}  normal ${kb(b.normalBytes).padStart(10)}`
  )
}
console.log(`\ntotal on disk ${kb(onDisk)}`)
console.log(`estimated decoded ${(decoded / 1048576).toFixed(2)} MB (advisory — measure in the browser)`)
