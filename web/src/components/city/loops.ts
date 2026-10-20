// Where a car or a walker is at a moment in time. The plan hands over closed
// polylines and a count; this turns them into evenly spaced, tangent-oriented
// transforms. Nothing here simulates traffic — SimCity's own automata are a
// visual approximation loosely correlated with the simulation, and so is this.
import type { CityLoop } from '@/types/city'

export interface LoopPath {
  points: [number, number][]
  /** Distance along the loop at each point; the last entry is its length. */
  cum: number[]
  length: number
}

export interface LoopPose {
  x: number
  z: number
  /** Y rotation that turns a +Z-facing body along the tangent. */
  angle: number
}

/** Measures a closed polyline. The first point is repeated last by the plan. */
export function loopPath(points: [number, number][]): LoopPath {
  const pts = points.map((p) => [p[0], p[1]] as [number, number])
  if (pts.length > 1) {
    const a = pts[0]
    const b = pts[pts.length - 1]
    if (a[0] !== b[0] || a[1] !== b[1]) pts.push([a[0], a[1]])
  }
  const cum = [0]
  for (let i = 1; i < pts.length; i++) {
    cum.push(cum[i - 1] + Math.hypot(pts[i][0] - pts[i - 1][0], pts[i][1] - pts[i - 1][1]))
  }
  return { points: pts, cum, length: cum[cum.length - 1] }
}

/** The pose at `u` in [0, 1) along the loop, written into `out`. */
export function poseAt(path: LoopPath, u: number, out: LoopPose): LoopPose {
  const n = path.points.length
  if (n < 2 || path.length === 0) {
    out.x = path.points[0]?.[0] ?? 0
    out.z = path.points[0]?.[1] ?? 0
    out.angle = 0
    return out
  }
  const wrapped = u - Math.floor(u)
  const d = wrapped * path.length
  // Linear rather than binary search: the walk is a few dozen segments and it
  // runs once per agent per frame, with no allocation either way.
  let i = 1
  while (i < n - 1 && path.cum[i] < d) i++
  const a = path.points[i - 1]
  const b = path.points[i]
  const span = path.cum[i] - path.cum[i - 1] || 1
  const t = (d - path.cum[i - 1]) / span
  out.x = a[0] + (b[0] - a[0]) * t
  out.z = a[1] + (b[1] - a[1]) * t
  out.angle = Math.atan2(b[0] - a[0], b[1] - a[1])
  return out
}

/** A seeded phase nudges an agent inside its own share of the loop. */
export function phaseShift(phase: number | undefined): number {
  return phase === undefined ? 0 : (phase - 0.5) * 0.35
}

/**
 * The whole loop's worth of agents at time `t`, evenly spaced. A phase nudges
 * each agent inside its own share of the loop so they are not in lockstep,
 * and never past its neighbour's place.
 */
export function loopPoses(
  path: LoopPath,
  count: number,
  t = 0,
  phases: number[] = [],
  out: LoopPose[] = []
): LoopPose[] {
  for (let i = 0; i < count; i++) {
    const jitter = phaseShift(phases[i])
    let pose = out[i]
    if (!pose) {
      pose = { x: 0, z: 0, angle: 0 }
      out[i] = pose
    }
    poseAt(path, (i + jitter) / count + t, pose)
  }
  out.length = count
  return out
}

/** Metres a second for a loop, by the tier of the road it runs on. */
export function loopSpeed(loop: CityLoop): number {
  if (loop.kind === 'pedestrian') return 1.4
  return loop.tier === 2 ? 11 : 7
}
