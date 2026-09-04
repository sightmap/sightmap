import { describe, expect, it } from 'vitest'
import { furnish } from './furnish'
import { buildPath, routeOnFloor } from './geometry'
import {
  CORE,
  FLOOR_H,
  FLOORS,
  JOURNEYS,
  LANES,
  SLAB_T,
  findRoom,
  roomStand,
} from './model'

const RADIUS = 0.13
const SKIP_TYPES = new Set(['partition', 'rail'])

interface Obstacle {
  x: number
  z: number
  sx: number
  sz: number
  ry: number
  label: string
}

function obstaclesOn(floor: number, skipKiosk?: string): Obstacle[] {
  const out: Obstacle[] = []
  for (const room of FLOORS[floor].rooms) {
    for (const it of furnish(room)) {
      if (SKIP_TYPES.has(it.type)) continue
      if (it.y - it.sy / 2 > 0.75) continue
      out.push({ x: it.x, z: it.z, sx: it.sx, sz: it.sz, ry: it.ry, label: `${room.name}:${it.type}` })
    }
    if (room.kind === 'action' && room.name !== skipKiosk) {
      out.push({ x: room.x, z: room.z, sx: 0.55, sz: 0.55, ry: 0, label: `${room.name}:kiosk` })
    }
  }
  return out
}

function inside(x: number, z: number, minx: number, minz: number, maxx: number, maxz: number): boolean {
  return x >= minx && x <= maxx && z >= minz && z <= maxz
}

function segmentHitsAabb(
  x0: number,
  z0: number,
  x1: number,
  z1: number,
  minx: number,
  minz: number,
  maxx: number,
  maxz: number
): boolean {
  if (inside(x0, z0, minx, minz, maxx, maxz) || inside(x1, z1, minx, minz, maxx, maxz)) return true
  const dx = x1 - x0
  const dz = z1 - z0
  let t0 = 0
  let t1 = 1
  const clip = (p: number, q: number): boolean => {
    if (Math.abs(p) < 1e-9) return q >= 0
    const r = q / p
    if (p < 0) {
      if (r > t1) return false
      if (r > t0) t0 = r
    } else {
      if (r < t0) return false
      if (r < t1) t1 = r
    }
    return true
  }
  return clip(-dx, x0 - minx) && clip(dx, maxx - x0) && clip(-dz, z0 - minz) && clip(dz, maxz - z0)
}

function hits(ax: number, az: number, bx: number, bz: number, it: Obstacle): boolean {
  const c = Math.cos(-it.ry)
  const s = Math.sin(-it.ry)
  const loc = (x: number, z: number): [number, number] => {
    const dx = x - it.x
    const dz = z - it.z
    return [dx * c - dz * s, dx * s + dz * c]
  }
  const [axl, azl] = loc(ax, az)
  const [bxl, bzl] = loc(bx, bz)
  const hw = it.sx / 2 + RADIUS
  const hd = it.sz / 2 + RADIUS
  return segmentHitsAabb(axl, azl, bxl, bzl, -hw, -hd, hw, hd)
}

function collisions(ax: number, az: number, bx: number, bz: number, obs: Obstacle[]): string[] {
  return obs.filter((it) => hits(ax, az, bx, bz, it)).map((it) => it.label)
}

describe('lane graph', () => {
  it('keeps every lane segment clear of low furniture', () => {
    const hitsAll: string[] = []
    LANES.forEach((lanes, floor) => {
      const obs = obstaclesOn(floor)
      for (const lane of lanes) {
        for (let i = 0; i < lane.length - 1; i++) {
          const [ax, az] = lane[i]
          const [bx, bz] = lane[i + 1]
          for (const label of collisions(ax, az, bx, bz, obs)) {
            hitsAll.push(`F${floor} ${ax},${az}→${bx},${bz} ${label}`)
          }
        }
      }
    })
    expect(hitsAll).toEqual([])
  })
})

describe('buildPath', () => {
  it('does not emit consecutive duplicate points', () => {
    for (const j of JOURNEYS) {
      const path = buildPath(j)
      for (let i = 1; i < path.points.length; i++) {
        expect(path.points[i].distanceToSquared(path.points[i - 1]), `${j.name} dup at ${i}`).toBeGreaterThan(1e-6)
      }
    }
  })

  it('routes floor changes through the core at both floor heights', () => {
    for (const j of JOURNEYS) {
      const path = buildPath(j)
      for (let k = 1; k < j.stops.length; k++) {
        const [pf] = j.stops[k - 1]
        const [f] = j.stops[k]
        if (pf === f) continue
        const ys = path.points.filter((p) => Math.hypot(p.x - CORE.x, p.z - CORE.z) < 1e-3).map((p) => p.y)
        expect(ys.some((y) => Math.abs(y - (pf * FLOOR_H + SLAB_T)) < 0.2), `${j.name} missing core at F${pf}`).toBe(true)
        expect(ys.some((y) => Math.abs(y - (f * FLOOR_H + SLAB_T)) < 0.2), `${j.name} missing core at F${f}`).toBe(true)
      }
    }
  })

  it('keeps every journey leg clear of low furniture', () => {
    const hitsAll: string[] = []
    for (const j of JOURNEYS) {
      const path = buildPath(j)
      for (let k = 1; k < j.stops.length; k++) {
        const [pf, fromName] = j.stops[k - 1]
        const [f, toName] = j.stops[k]
        const a = path.stops[k - 1]
        const b = path.stops[k]
        for (let i = a; i < b; i++) {
          const p = path.points[i]
          const q = path.points[i + 1]
          if (Math.abs(p.y - q.y) > 0.3) continue
          const useFloor = Math.abs(p.y - (pf * FLOOR_H + SLAB_T)) < Math.abs(p.y - (f * FLOOR_H + SLAB_T)) ? pf : f
          const obs = obstaclesOn(useFloor, toName)
          for (const label of collisions(p.x, p.z, q.x, q.z, obs)) {
            hitsAll.push(`${j.name} ${fromName}→${toName} ${p.x.toFixed(2)},${p.z.toFixed(2)}→${q.x.toFixed(2)},${q.z.toFixed(2)} ${label}`)
          }
        }
      }
    }
    expect(hitsAll).toEqual([])
  })
})

describe('HealDemo legs', () => {
  it('routes PaymentForm to both ContinueButton stands without clipping', () => {
    const hitsAll: string[] = []
    const from = roomStand(3, findRoom(3, 'PaymentForm'))
    const room = findRoom(3, 'ContinueButton')
    const oldPos = roomStand(3, room, 0)
    const newPos = roomStand(3, room, 1)
    const legs: [string, [number, number, number], [number, number, number]][] = [
      ['pay→old', from, oldPos],
      ['old→new', oldPos, newPos],
    ]
    for (const [label, a, b] of legs) {
      const route = routeOnFloor(3, a[0], a[2], b[0], b[2])
      const obs = obstaclesOn(3, 'ContinueButton')
      for (let i = 0; i < route.length - 1; i++) {
        const p = route[i]
        const q = route[i + 1]
        for (const hit of collisions(p.x, p.z, q.x, q.z, obs)) {
          hitsAll.push(`${label} ${p.x.toFixed(2)},${p.z.toFixed(2)}→${q.x.toFixed(2)},${q.z.toFixed(2)} ${hit}`)
        }
      }
    }
    expect(hitsAll).toEqual([])
  })
})

describe('roomStand', () => {
  it('places walkers on the slab, not below it', () => {
    for (const j of JOURNEYS) {
      for (const [f, name] of j.stops) {
        const stand = roomStand(f, findRoom(f, name))
        expect(stand[1], `${name} F${f}`).toBeGreaterThanOrEqual(f * FLOOR_H + SLAB_T - 1e-6)
      }
    }
  })
})
