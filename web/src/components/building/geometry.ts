// Geometry derived from the model: the white line work drawn on each
// blueprint sheet, and the walking paths a journey follows through the
// building. Kept apart from the React components so the maths is testable
// and so the same path feeds both the crowd and the trajectory highlight.
import * as THREE from 'three'
import {
  CORE,
  CORE_DOOR,
  FLOORS,
  FLOOR_D,
  FLOOR_W,
  SHEET,
  SLAB_T,
  findRoom,
  floorY,
  roomStand,
  type Journey,
} from './model'
import { DRAWN, furnish } from './furnish'

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
    for (const it of furnish(r)) {
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

export interface Path {
  points: THREE.Vector3[]
  /** Cumulative length at each point. */
  cum: number[]
  /** Index into points for each stop of the journey. */
  stops: number[]
  length: number
}

/**
 * The walk for a journey: room to room on the same floor, or room → core
 * door → up the shaft → out the door → room when the floor changes.
 * `lift` raises the whole path (the trajectory ribbon floats a little).
 */
export function buildPath(journey: Journey, lift = 0, healShift = 0): Path {
  const points: THREE.Vector3[] = []
  const stops: number[] = []
  const push = (p: P3) => {
    const v = new THREE.Vector3(p[0], p[1] + lift, p[2])
    const last = points[points.length - 1]
    if (!last || last.distanceToSquared(v) > 1e-6) points.push(v)
    return points.length - 1
  }
  for (let k = 0; k < journey.stops.length; k++) {
    const [f, name] = journey.stops[k]
    const room = findRoom(f, name)
    const stand = roomStand(f, room, room.alt ? healShift : 0)
    if (k === 0) {
      stops.push(push(stand))
      continue
    }
    const [pf] = journey.stops[k - 1]
    if (pf !== f) {
      const yA = floorY(pf) + SLAB_T
      const yB = floorY(f) + SLAB_T
      push([CORE_DOOR.x, yA, CORE_DOOR.z])
      push([CORE.x, yA, CORE.z])
      push([CORE.x, yB, CORE.z])
      push([CORE_DOOR.x, yB, CORE_DOOR.z])
    }
    stops.push(push(stand))
  }
  const cum = [0]
  for (let i = 1; i < points.length; i++) cum.push(cum[i - 1] + points[i].distanceTo(points[i - 1]))
  return { points, cum, stops, length: cum[cum.length - 1] }
}

/** Point at arc-length `d` along the path, written into `out`. */
export function pointAt(path: Path, d: number, out: THREE.Vector3): THREE.Vector3 {
  const { points, cum } = path
  if (d <= 0) return out.copy(points[0])
  if (d >= path.length) return out.copy(points[points.length - 1])
  let i = 1
  while (i < cum.length && cum[i] < d) i++
  const seg = cum[i] - cum[i - 1]
  const t = seg > 0 ? (d - cum[i - 1]) / seg : 0
  return out.copy(points[i - 1]).lerp(points[i], t)
}
