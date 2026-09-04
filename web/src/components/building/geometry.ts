// Geometry derived from the model: the white line work drawn on each
// blueprint sheet. Kept apart from the React components so the maths is
// testable. The walk-path builder (buildPath/pointAt) lives in path.ts and
// is re-exported below for the existing call sites — it has to stay out of
// this file because it must depend on `model` alone: furnish.ts builds a
// path too (to keep furniture clear of it), and this file already depends
// on furnish.ts to draw furniture footprints on the sheet, so a walk-path
// builder here would put furnish.ts and geometry.ts in an import cycle.
import { CORE, FLOORS, FLOOR_D, FLOOR_W, SHEET } from './model'
import { DRAWN, furnish } from './furnish'

export { buildPath, pointAt, type Path } from './path'

type P3 = [number, number, number]

/** Pairs of points (LineSegments layout) drawing one floor plan on its sheet. */
export function sheetLinePoints(floorIndex: number): P3[] {
  const y = 0.012
  const pts: P3[] = []
  const rect = (x: number, z: number, w: number, d: number) => {
    const a: P3 = [x - w / 2, y, z - d / 2]
    const b: P3 = [x + w / 2, y, z - d / 2]
    const c: P3 = [x + w / 2, y, z + d / 2]
    const e: P3 = [x - w / 2, y, z + d / 2]
    pts.push(a, b, b, c, c, e, e, a)
  }
  // Sheet border and the footprint of the floor.
  rect(0, 0, SHEET.w - 0.3, SHEET.d - 0.3)
  rect(0, 0, FLOOR_W, FLOOR_D)
  // Title block in the sheet's front-right corner.
  const tbx = SHEET.w / 2 - 0.15 - 1.4
  const tbz = -SHEET.d / 2 + 0.15 + 0.32
  rect(tbx, tbz, 2.8, 0.64)
  pts.push([tbx - 1.4, y, tbz], [tbx + 1.4, y, tbz])
  pts.push([tbx - 0.4, y, tbz - 0.32], [tbx - 0.4, y, tbz + 0.32])
  // Core and its door.
  rect(CORE.x, CORE.z, CORE.w, CORE.d)
  pts.push([CORE.x + CORE.w / 2, y, CORE.z - 0.35], [CORE.x + CORE.w / 2 + 0.45, y, CORE.z - 0.35])
  // Door swing on the core.
  const arc = (cx: number, cz: number, rad: number, a0: number, a1: number) => {
    const n = 6
    for (let k = 0; k < n; k++) {
      const t0 = a0 + ((a1 - a0) * k) / n
      const t1 = a0 + ((a1 - a0) * (k + 1)) / n
      pts.push([cx + Math.cos(t0) * rad, y, cz + Math.sin(t0) * rad], [cx + Math.cos(t1) * rad, y, cz + Math.sin(t1) * rad])
    }
  }
  arc(CORE.x + CORE.w / 2, CORE.z - 0.35, 0.45, 0, Math.PI / 2)
  // Rooms, with the footprints of their furniture drawn lighter in spirit:
  // the same layout the built floor gets, so the drawing is the plan.
  for (const r of FLOORS[floorIndex].rooms) {
    const blocks = r.blocks ?? [{ x: r.x, z: r.z, w: r.w, d: r.d }]
    for (const b of blocks) rect(b.x, b.z, b.w, b.d)
    for (const it of furnish(r, floorIndex).items) {
      if (!DRAWN.includes(it.type)) continue
      const c = Math.cos(it.ry)
      const s = Math.sin(it.ry)
      const hw = it.sx / 2
      const hd = it.sz / 2
      const corners: [number, number][] = [
        [-hw, -hd],
        [hw, -hd],
        [hw, hd],
        [-hw, hd],
      ].map(([dx, dz]) => [it.x + dx * c - dz * s, it.z + dx * s + dz * c])
      for (let k = 0; k < 4; k++) {
        const a = corners[k]
        const b2 = corners[(k + 1) % 4]
        pts.push([a[0], y, a[1]], [b2[0], y, b2[1]])
      }
    }
  }
  // Dimension ticks along the front edge of the sheet.
  for (let x = -FLOOR_W / 2; x <= FLOOR_W / 2 + 0.01; x += 1) {
    pts.push([x, y, FLOOR_D / 2 + 0.25], [x, y, FLOOR_D / 2 + 0.45])
  }
  pts.push([-FLOOR_W / 2, y, FLOOR_D / 2 + 0.35], [FLOOR_W / 2, y, FLOOR_D / 2 + 0.35])
  return pts
}

