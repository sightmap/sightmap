// Where each floor's baked ambient occlusion lives, and how a surface finds it.
//
// Nothing in this scene darkens the crease where a desk leg meets the carpet,
// and that crease is the whole trick of a dollhouse model: without it every
// object reads as hovering a centimetre above the floor it is standing on.
//
// The occlusion is baked rather than computed. Screen-space AO on this scene
// would cost 4–8ms on mid-range mobile before blur, and with ~180 hard-edged
// primitives under a scroll-driven camera it would crawl and shimmer along
// every edge. A bake is ~0ms at runtime and holds still.
//
// One texture serves all six floors. Each floor owns a cell of a small atlas,
// and the cell offset is baked into the geometry's second UV set, so three's
// stock `aoMap` path samples the right cell with no shader patch and no
// per-floor material. The floors keep sharing one material, which is what
// keeps the merge's draw-call win intact.
//
// Pure data and arithmetic — no three.js import — so the bake script can pull
// it in under tsx without loading WebGL code, and so the bake and the runtime
// cannot disagree about which cell is whose.

/** Atlas dimensions. 4×2 grid of 128² cells: six floors, a white cell, a spare. */
export const AO_ATLAS = { width: 512, height: 256, cols: 4, rows: 2, cell: 128 } as const

/**
 * The cell every non-floor surface points at: solid white, meaning unoccluded.
 *
 * `mergeGeometries` needs one attribute set across the whole batch, so once a
 * floor's slab carries a `uv1` every part merged beside it carries one too.
 * Rather than leave those UVs at zero — which would sample floor 0's occlusion
 * onto a curtain wall — they aim here.
 */
export const AO_WHITE_CELL = 6

/**
 * Half a texel, in cell-local units. Cells are packed edge to edge, so a
 * bilinear tap at a cell boundary would otherwise pull in the neighbouring
 * floor's occlusion. Insetting by half a texel keeps every tap inside its own
 * cell; the AO bake fades to white at the cell edge anyway, so the sliver of
 * range given up carries nothing.
 */
const INSET = 0.5 / AO_ATLAS.cell

/** The sub-rectangle of the atlas that `cell` owns, in 0..1 atlas UV. */
export function aoCellRect(cell: number): { u: number; v: number; du: number; dv: number } {
  const col = cell % AO_ATLAS.cols
  const row = Math.floor(cell / AO_ATLAS.cols)
  const cw = AO_ATLAS.cell / AO_ATLAS.width
  const ch = AO_ATLAS.cell / AO_ATLAS.height
  return {
    u: col * cw + INSET * cw,
    v: row * ch + INSET * ch,
    du: cw * (1 - INSET * 2),
    dv: ch * (1 - INSET * 2),
  }
}

/**
 * Map a floor-space XZ position into `cell`'s patch of the atlas.
 *
 * The plate is unwrapped top-down, which is the one unwrap that needs no
 * seams: every surface that receives this occlusion — the slab deck, the
 * component carpets sitting on it — is a horizontal plane, so its texel
 * footprint is its footprint on the floor plan.
 */
export function aoUvForPoint(
  x: number,
  z: number,
  floorW: number,
  floorD: number,
  cell: number
): [number, number] {
  const r = aoCellRect(cell)
  const fx = Math.min(1, Math.max(0, (x + floorW / 2) / floorW))
  const fz = Math.min(1, Math.max(0, (z + floorD / 2) / floorD))
  return [r.u + fx * r.du, r.v + fz * r.dv]
}

/**
 * Build a `uv1` array for a geometry whose vertices are already in floor space.
 *
 * Takes the raw position attribute rather than a BufferGeometry so the bake
 * scripts can exercise the same arithmetic the scene runs.
 */
export function aoUvArray(
  positions: ArrayLike<number>,
  floorW: number,
  floorD: number,
  cell: number
): Float32Array {
  const count = positions.length / 3
  const uv = new Float32Array(count * 2)
  for (let i = 0; i < count; i++) {
    const [u, v] = aoUvForPoint(positions[i * 3], positions[i * 3 + 2], floorW, floorD, cell)
    uv[i * 2] = u
    uv[i * 2 + 1] = v
  }
  return uv
}
