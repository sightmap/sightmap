// Bakes the per-floor ambient occlusion atlas for the building scene.
//
// Run: pnpm build:building-ao   (needs the `ktx` encoder on PATH)
//
// The occluders are read from `model.ts` and `furnish.ts` — the same modules
// Tower.tsx renders from, not a transcription of them. That is the point:
// furniture here is procedural and deterministic, so the bake can call
// `furnish(room)` and get exactly the desks the scene will draw. A hand-copied
// table of footprints would be a second source of truth that drifts the first
// time someone moves a sofa, and the failure would be a shadow under nothing.
//
// What is modelled is contact occlusion, not a full hemisphere integration:
// how much a horizontal surface is darkened by the things resting on it. That
// is the cue the scene is missing and the only one this projection can carry
// honestly — a top-down unwrap has nothing to say about vertical surfaces.
import { mkdirSync, rmSync } from 'node:fs'
import { AO_ATLAS, AO_WHITE_CELL, aoCellRect } from '../src/components/building/ao-atlas'
import { CORE, FLOOR_D, FLOOR_W, FLOORS } from '../src/components/building/model'
import { furnish, type Item } from '../src/components/building/furnish'
import { raster, put } from './lib/png'
import { writeKtx2 } from './lib/ktx'

const OUT_DIR = 'public/building/textures'

/** A thing resting on (or near) the floor plane that darkens it. */
interface Occluder {
  x: number
  z: number
  /** Half-extents on each axis, before rotation. */
  hx: number
  hz: number
  /** Rotation about Y, radians. */
  ry: number
  /** Height of the occluder itself. */
  h: number
  /** Gap between the floor plane and the occluder's underside. */
  gap: number
}

const occluderFromItem = (it: Item): Occluder => ({
  x: it.x,
  z: it.z,
  hx: Math.abs(it.sx) / 2,
  hz: Math.abs(it.sz) / 2,
  ry: it.ry,
  h: Math.abs(it.sy),
  // `y` is the item's centre above the slab top, so its underside is half its
  // height below that. Clamped at zero: an item sunk into the floor is resting
  // on it, not floating under it.
  gap: Math.max(0, it.y - Math.abs(it.sy) / 2),
})

/**
 * Distance from a point to a rotated rectangle: zero inside, positive outside.
 *
 * The standard rotate-into-local-space then clamp-per-axis trick. Rotating the
 * query point by -ry is cheaper and steadier than rotating four corners and
 * testing edges, and it degrades gracefully for a point deep inside.
 */
function distanceToRect(px: number, pz: number, o: Occluder): number {
  const c = Math.cos(-o.ry)
  const s = Math.sin(-o.ry)
  const dx = px - o.x
  const dz = pz - o.z
  const lx = dx * c - dz * s
  const lz = dx * s + dz * c
  const ox = Math.max(0, Math.abs(lx) - o.hx)
  const oz = Math.max(0, Math.abs(lz) - o.hz)
  return Math.hypot(ox, oz)
}

/**
 * How much one occluder darkens the floor at a given distance from it.
 *
 * Two behaviours worth stating, because they are what makes this read as
 * contact rather than as a drop shadow:
 *
 *  - Strength falls off with the gap under the object. A desk *top* at 0.7 m
 *    should not stamp a hard rectangle on the carpet; a desk *leg* should.
 *    Objects more than `GAP_REACH` above the floor contribute nothing.
 *  - Radius grows with both height and gap. A tall thing occludes more of the
 *    sky near its base, and a raised thing spreads what it occludes wider and
 *    softer — the same reason a real contact shadow blurs as it leaves the
 *    contact point.
 */
const GAP_REACH = 0.5

function occlusionAt(px: number, pz: number, o: Occluder): number {
  if (o.gap >= GAP_REACH) return 0
  const gapFade = 1 - o.gap / GAP_REACH
  const strength = Math.min(0.85, 0.35 + o.h * 0.45) * gapFade * gapFade
  const radius = Math.min(0.7, 0.09 + o.h * 0.22 + o.gap * 0.9)
  const d = distanceToRect(px, pz, o)
  if (d >= radius) return 0
  // Smoothstep rather than linear: a linear ramp leaves a visible crease at
  // the outer edge of every shadow, which reads as a decal.
  const t = 1 - d / radius
  return strength * t * t * (3 - 2 * t)
}

/**
 * Combine occluders multiplicatively.
 *
 * Summing would drive the crowded floors — where a chair tucks under a desk —
 * straight to black and flatten every one of them to the same slab of ink.
 * Multiplying visibilities keeps overlaps darker than either alone while the
 * result stays in range on its own.
 */
function bakeCell(occluders: Occluder[]): Float32Array {
  const n = AO_ATLAS.cell
  const out = new Float32Array(n * n)
  for (let row = 0; row < n; row++) {
    // Texel centres, mapped back through the same top-down unwrap the runtime
    // uses: u spans FLOOR_W, v spans FLOOR_D, both centred on the origin.
    const pz = ((row + 0.5) / n - 0.5) * FLOOR_D
    for (let col = 0; col < n; col++) {
      const px = ((col + 0.5) / n - 0.5) * FLOOR_W
      let visibility = 1
      for (const o of occluders) {
        const occ = occlusionAt(px, pz, o)
        if (occ > 0) visibility *= 1 - occ
      }
      out[row * n + col] = visibility
    }
  }
  return out
}

/** The service core runs floor to ceiling in the back corner: a hard contact. */
const CORE_OCCLUDER: Occluder = {
  x: CORE.x,
  z: CORE.z,
  hx: CORE.w / 2,
  hz: CORE.d / 2,
  ry: 0,
  h: 1.2,
  gap: 0,
}

function main(): void {
  mkdirSync(OUT_DIR, { recursive: true })

  const atlas = raster(AO_ATLAS.width, AO_ATLAS.height, 1)
  // White everywhere first, so the spare cells — and the cell every
  // non-floor surface points at — mean "unoccluded" rather than "black".
  for (let y = 0; y < AO_ATLAS.height; y++) {
    for (let x = 0; x < AO_ATLAS.width; x++) put(atlas, x, y, [255])
  }

  const report: string[] = []
  FLOORS.forEach((floor, i) => {
    const occluders = [CORE_OCCLUDER, ...floor.rooms.flatMap((r) => furnish(r).items.map(occluderFromItem))]
    const cell = bakeCell(occluders)
    const col = i % AO_ATLAS.cols
    const row = Math.floor(i / AO_ATLAS.cols)
    let darkest = 1
    let sum = 0
    for (let y = 0; y < AO_ATLAS.cell; y++) {
      for (let x = 0; x < AO_ATLAS.cell; x++) {
        const v = cell[y * AO_ATLAS.cell + x]
        darkest = Math.min(darkest, v)
        sum += v
        put(atlas, col * AO_ATLAS.cell + x, row * AO_ATLAS.cell + y, [Math.round(v * 255)])
      }
    }
    const mean = sum / (AO_ATLAS.cell * AO_ATLAS.cell)
    report.push(
      `${String(i)} ${floor.name.padEnd(20)} occluders ${String(occluders.length).padStart(3)}  ` +
        `mean ${mean.toFixed(3)}  darkest ${darkest.toFixed(3)}`
    )
  })

  if (FLOORS.length > AO_WHITE_CELL) {
    throw new Error(
      `AO atlas has ${String(AO_WHITE_CELL)} floor cells before the white cell, but the model has ` +
        `${String(FLOORS.length)} floors. Grow AO_ATLAS before adding another floor.`
    )
  }

  // Greyscale, and linear: an occlusion factor is a multiplier, not a colour,
  // so an sRGB curve on the way in is simply the wrong multiplier on the way out.
  const bytes = writeKtx2(atlas, `${OUT_DIR}/floor-ao.ktx2`, 'linear')
  rmSync(`${OUT_DIR}/floor-ao.ktx2.src.png`, { force: true })

  for (const line of report) console.log(line)
  const white = aoCellRect(AO_WHITE_CELL)
  console.log(
    `\nfloor-ao.ktx2  ${String(AO_ATLAS.width)}×${String(AO_ATLAS.height)}  ${(bytes / 1024).toFixed(1)} KiB on disk` +
      `\nwhite cell ${String(AO_WHITE_CELL)} at uv ${white.u.toFixed(3)},${white.v.toFixed(3)}`
  )
}

main()
