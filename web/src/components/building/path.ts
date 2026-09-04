// The walking paths a journey follows through the building: room to room,
// and room -> core door -> up the shaft -> out the door -> room when the
// floor changes. Split out of geometry.ts so it depends on `model` alone —
// no `furnish` import — which lets furniture placement (furnish.ts) build a
// path and check its own footprints against it without a dependency cycle
// (geometry.ts separately imports furnish.ts to draw furniture on the
// blueprint sheet). `sheetLinePoints` stays in geometry.ts; buildPath and
// pointAt are re-exported from there for the existing call sites.
import * as THREE from 'three'
import { CORE, CORE_DOOR, SLAB_T, findRoom, floorY, roomStand, type Journey } from './model'

type P3 = [number, number, number]

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

/**
 * Tangent at `points[k]` on one axis, for the Catmull-Rom spline through
 * `points`. Computed once per point (not per segment) and shared by both
 * segments that touch it, so the curve stays C1-continuous across joins.
 *
 * Raw Catmull-Rom (the plain secant average `(next-prev)/2`) overshoots
 * whenever a neighbour drags the tangent off-axis while the two segments
 * either side of the point don't actually move that way on this axis —
 * exactly what happens on the elevator's vertical rise, whose neighbours
 * are corridor points either side of the core door. Clamping to zero
 * whenever the tangent disagrees with either adjacent segment (a local
 * extremum, including "doesn't move on this axis") keeps that axis
 * monotonic, so a straight vertical rise stays vertical instead of
 * bowing sideways.
 */
function tangentAt(points: THREE.Vector3[], k: number, axis: 'x' | 'y' | 'z'): number {
  const n = points.length
  const cur = points[k][axis]
  if (k === 0) return points[1][axis] - cur
  if (k === n - 1) return cur - points[k - 1][axis]
  const prev = points[k - 1][axis]
  const next = points[k + 1][axis]
  const before = cur - prev
  const after = next - cur
  let v = (next - prev) * 0.5
  if (v * before <= 0 || v * after <= 0) v = 0
  return v
}

/** Cubic Hermite spline between `p1` and `p2` with tangents `v0`/`v1`, at `t` in [0, 1]. */
function hermite(t: number, p1: number, p2: number, v0: number, v1: number): number {
  const t2 = t * t
  const t3 = t * t2
  return (2 * p1 - 2 * p2 + v0 + v1) * t3 + (-3 * p1 + 3 * p2 - 2 * v0 - v1) * t2 + v0 * t + p1
}

/**
 * Point at arc-length `d` along the path, written into `out`. Interpolates
 * with Catmull-Rom over the bracketing segment's neighbours so corners
 * round instead of kinking at every vertex — a prerequisite for a heading
 * derived from the tangent to read smoothly through turns.
 */
export function pointAt(path: Path, d: number, out: THREE.Vector3): THREE.Vector3 {
  const { points, cum } = path
  if (d <= 0) return out.copy(points[0])
  if (d >= path.length) return out.copy(points[points.length - 1])
  let i = 1
  while (i < cum.length && cum[i] < d) i++
  const seg = cum[i] - cum[i - 1]
  const t = seg > 0 ? (d - cum[i - 1]) / seg : 0
  const p1 = points[i - 1]
  const p2 = points[i]
  out.x = hermite(t, p1.x, p2.x, tangentAt(points, i - 1, 'x'), tangentAt(points, i, 'x'))
  out.y = hermite(t, p1.y, p2.y, tangentAt(points, i - 1, 'y'), tangentAt(points, i, 'y'))
  out.z = hermite(t, p1.z, p2.z, tangentAt(points, i - 1, 'z'), tangentAt(points, i, 'z'))
  return out
}
