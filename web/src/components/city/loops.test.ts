import { describe, expect, it } from 'vitest'
import { planCity } from '../../../scripts/lib/city'
import { loopPath, loopPoses, loopSpeed, poseAt } from './loops'

const square: [number, number][] = [
  [-10, -10],
  [10, -10],
  [10, 10],
  [-10, 10],
  [-10, -10],
]

describe('loopPath', () => {
  it('measures a closed polyline', () => {
    const path = loopPath(square)
    expect(path.length).toBe(80)
    expect(path.cum[path.cum.length - 1]).toBe(80)
  })

  it('closes a polyline that does not repeat its first point', () => {
    const path = loopPath(square.slice(0, 4))
    expect(path.points[path.points.length - 1]).toEqual(square[0])
    expect(path.length).toBe(80)
  })
})

describe('loopPoses', () => {
  it('spaces agents evenly around the loop', () => {
    const path = loopPath(square)
    const poses = loopPoses(path, 8)
    expect(poses).toHaveLength(8)
    const gaps = poses.map((p, i) => {
      const q = poses[(i + 1) % poses.length]
      return Math.hypot(q.x - p.x, q.z - p.z)
    })
    // Every agent is a tenth of the loop from the next; the corners make the
    // straight-line gap shorter than the distance along the road, so only the
    // ones that share a side are compared.
    for (const gap of gaps) expect(gap).toBeGreaterThan(0)
    for (let i = 0; i < 8; i += 2) expect(gaps[i]).toBeCloseTo(gaps[0], 6)
  })

  it('orients each agent along the road it is on', () => {
    const path = loopPath(square)
    // Eight agents, so four of them sit in the middle of a side rather than
    // on a corner, where either heading would be right.
    const poses = loopPoses(path, 8)
    expect(poses[1].angle).toBeCloseTo(Math.PI / 2, 6)
    expect(poses[3].angle).toBeCloseTo(0, 6)
    expect(poses[5].angle).toBeCloseTo(-Math.PI / 2, 6)
    expect(poses[7].angle).toBeCloseTo(Math.PI, 6)
  })

  it('reuses its output array, so the animation loop allocates nothing', () => {
    const path = loopPath(square)
    const out = loopPoses(path, 3)
    const again = loopPoses(path, 3, 0.25, [], out)
    expect(again).toBe(out)
    expect(again[0]).toBe(out[0])
  })

  it('wraps: a full lap comes back to where it started', () => {
    const path = loopPath(square)
    const a = poseAt(path, 0.2, { x: 0, z: 0, angle: 0 })
    const b = poseAt(path, 1.2, { x: 0, z: 0, angle: 0 })
    expect(b.x).toBeCloseTo(a.x, 6)
    expect(b.z).toBeCloseTo(a.z, 6)
  })
})

describe('the plan’s own loops', () => {
  const plan = planCity()

  it('samples every loop the plan carries', () => {
    expect(plan.loops.length).toBeGreaterThan(0)
    for (const loop of plan.loops) {
      const path = loopPath(loop.points)
      expect(path.length).toBeGreaterThan(0)
      const poses = loopPoses(path, loop.count, 0.4, loop.phases)
      expect(poses).toHaveLength(loop.count)
      for (const pose of poses) expect(Number.isFinite(pose.x + pose.z + pose.angle)).toBe(true)
      expect(loopSpeed(loop)).toBeGreaterThan(0)
    }
  })
})
