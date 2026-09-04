// Procedural furniture. Every component kind gets a recognisable interior —
// workstations for forms, a counter for navigation, a library lounge for
// content, a meeting room for data — laid out deterministically from the
// room's footprint so the blueprint sheets and the built floors agree.
// Items are cheap primitives (unit box / cylinder / sphere / capsule) with a
// per-item scale and colour; Tower.tsx renders each type as one instanced
// mesh per floor.
//
// Furniture placement and the walk paths (path.ts) used to be generated
// independently and never consulted each other, so a desk or sofa could
// land squarely in someone's route. `resolveGroup` below steers the
// furniture instead of the walker: every DRAWN item (or co-located cluster
// of them, e.g. a sofa and its coffee table) is checked against the actual
// walked paths for this floor before it is placed, nudged clear of them if
// it's in the way, and dropped only if no nearby spot is clear. That keeps
// the fix correct automatically as paths or rooms change, rather than
// hand-tuning positions that the next furnish() edit would undo.
import * as THREE from 'three'
import { FLOOR_H, JOURNEYS, PLATE, type Block, type Room } from './model'
import { buildPath, pointAt } from './path'

export type ItemType =
  | 'desk'
  | 'monitor'
  | 'chair'
  | 'sofa'
  | 'table'
  | 'pot'
  | 'leaf'
  | 'shelf'
  | 'book'
  | 'screen'
  | 'partition'
  | 'rail'
  | 'counter'

export interface Item {
  type: ItemType
  /** Centre, floor-local: x/z in floor coordinates, y above the slab top. */
  x: number
  y: number
  z: number
  ry: number
  sx: number
  sy: number
  sz: number
  color: string
}

/**
 * Someone who lives in a room: at a desk, on a sofa, behind a counter. They
 * are not furniture — People.tsx draws them from the same instanced body-part
 * rig as the walkers, so the building has one population, not two.
 */
export interface Occupant {
  /** Floor-local, like `Item`: x/z across the plate, y above the slab top. */
  x: number
  y: number
  z: number
  ry: number
  seated: boolean
  shirt: string
  skin: string
}

export interface Furnishing {
  items: Item[]
  people: Occupant[]
}

/** Footprints worth drawing on the blueprint sheet. */
export const DRAWN: ItemType[] = ['desk', 'table', 'sofa', 'shelf', 'counter']

// mulberry32: tiny seeded RNG so a room always gets the same furniture.
function rng(seed: string): () => number {
  let a = 1779033703
  for (let i = 0; i < seed.length; i++) a = Math.imul(a ^ seed.charCodeAt(i), 3432918353)
  return () => {
    a |= 0
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const WOOD = '#d8bf9a'
const DARK = '#2b2d33'
const WHITE = '#f6f2ea'
const SKIN = ['#e9c4a3', '#c9976f', '#8d5a3b', '#f0d5bd']
const SHIRT = ['#6e7fa8', '#b8a58c', '#4f5d75', '#a86e7c', '#7c9a86', '#e0d6c8']
const BOOKS = ['#c9456d', '#6b8aed', '#2d8a5e', '#b8860b', '#9b7ae8', '#e0d6c8', '#3d3929']
const GREENS = ['#4f8a4c', '#67a05a', '#3d7a45', '#7bb06a']

/** The desk's plan footprint (x/z), shared by the item geometry and the
 *  corridor check so the two can never drift apart. */
const DESK_W = 1.15
const DESK_D = 0.55

function item(type: ItemType, x: number, y: number, z: number, s: [number, number, number], color: string, ry = 0): Item {
  return { type, x, y, z, ry, sx: s[0], sy: s[1], sz: s[2], color }
}

function plant(out: Item[], x: number, z: number, y: number, r: () => number, big = false): void {
  const h = big ? 0.42 : 0.28
  out.push(item('pot', x, y + h / 2, z, [big ? 0.34 : 0.26, h, big ? 0.34 : 0.26], '#c7b299'))
  const g = GREENS[Math.floor(r() * GREENS.length)]
  out.push(item('leaf', x, y + h + (big ? 0.34 : 0.24), z, big ? [0.75, 0.7, 0.75] : [0.5, 0.48, 0.5], g))
  if (big) out.push(item('leaf', x + 0.12, y + h + 0.6, z - 0.08, [0.42, 0.4, 0.42], GREENS[(GREENS.indexOf(g) + 1) % GREENS.length]))
}

function person(out: Occupant[], x: number, z: number, y: number, ry: number, r: () => number, seated = true): void {
  const shirt = SHIRT[Math.floor(r() * SHIRT.length)]
  const skin = SKIN[Math.floor(r() * SKIN.length)]
  out.push({ x, y, z, ry, seated, shirt, skin })
}

/** A desk facing +z (chair on the +z side). `ry` rotates the whole cluster. */
function workstation(out: Item[], who: Occupant[], cx: number, cz: number, y: number, ry: number, r: () => number): void {
  const c = Math.cos(ry)
  const s = Math.sin(ry)
  const at = (dx: number, dz: number): [number, number] => [cx + dx * c - dz * s, cz + dx * s + dz * c]
  const [dx, dz] = at(0, 0)
  out.push(item('desk', dx, y + 0.2, dz, [DESK_W, 0.4, DESK_D], WOOD, ry))
  const [mx, mz] = at(0, -0.12)
  out.push(item('monitor', mx, y + 0.56, mz, [0.46, 0.28, 0.04], DARK, ry))
  const [chx, chz] = at(0, 0.52)
  out.push(item('chair', chx, y + 0.22, chz, [0.4, 0.44, 0.4], '#3b3f4a', ry))
  if (r() < 0.6) person(who, chx, chz, y, ry + Math.PI, r)
}

// --- Keep-clear corridors ---------------------------------------------------
//
// A footprint's half-extents (world x/z), offset `dx`/`dz` from a group's
// anchor point, so a cluster of co-located items (a sofa and its coffee
// table, a meeting table and its tabletop overlay) can be checked and moved
// as one physical object rather than drifting apart.
interface Footprint {
  dx: number
  dz: number
  hw: number
  hd: number
  ry: number
}

/** Half the width of the clear band a walker needs either side of the walk
 *  line: a little over shoulder width, plus a small margin. */
const CORRIDOR_HALF = 0.32
/** Sample spacing along a path — finer than CORRIDOR_HALF so no gap in the
 *  walk is left unchecked between samples. */
const CORRIDOR_STEP = 0.12
const NUDGE_STEP = 0.12
const NUDGE_TRIES = 10
const NUDGE_DIRS: [number, number][] = [
  [1, 0],
  [-1, 0],
  [0, 1],
  [0, -1],
  [1, 1],
  [-1, -1],
  [1, -1],
  [-1, 1],
]

let corridorsByFloor: [number, number][][] | null = null

/**
 * Every walk path, densely sampled and bucketed by the floor it's on
 * (recovered from `y`, since every floor shares the same x/z footprint
 * stacked in y). Built once from `JOURNEYS` — the same walk People.tsx
 * actually renders, healShift and lift both at their walking defaults — so
 * a room's keep-clear corridor is the real route, not an approximation of
 * one that could drift from what people actually walk.
 */
function walkSamples(floor: number): [number, number][] {
  if (!corridorsByFloor) {
    corridorsByFloor = []
    const v = new THREE.Vector3()
    for (const journey of JOURNEYS) {
      const path = buildPath(journey)
      const bucket = (d: number) => {
        pointAt(path, d, v)
        const f = Math.round(v.y / FLOOR_H)
        ;(corridorsByFloor![f] ??= []).push([v.x, v.z])
      }
      for (let d = 0; d < path.length; d += CORRIDOR_STEP) bucket(d)
      bucket(path.length)
    }
  }
  return corridorsByFloor[floor] ?? []
}

/** Is `(px, pz)` within `pad` of the rectangle centred at `(cx, cz)`,
 *  half-extents `(hw, hd)`, rotated by `ry`? */
function withinFootprint(px: number, pz: number, cx: number, cz: number, hw: number, hd: number, ry: number, pad: number): boolean {
  const dx = px - cx
  const dz = pz - cz
  const c = Math.cos(-ry)
  const s = Math.sin(-ry)
  const lx = dx * c - dz * s
  const lz = dx * s + dz * c
  return Math.abs(lx) <= hw + pad && Math.abs(lz) <= hd + pad
}

function blocksAny(floor: number, x: number, z: number, prints: Footprint[]): boolean {
  const samples = walkSamples(floor)
  for (const [px, pz] of samples) {
    for (const p of prints) {
      if (withinFootprint(px, pz, x + p.dx, z + p.dz, p.hw, p.hd, p.ry, CORRIDOR_HALF)) return true
    }
  }
  return false
}

/**
 * Finds a placement for a furniture group — one footprint, or several
 * co-located ones that must move together — that keeps every footprint
 * clear of the floor's walk corridors. Tries the group's own position
 * first, then nudges it outward in a small spiral while every footprint
 * stays inside `bounds`. Returns `null` when nothing within reach is
 * clear, so the caller drops the group rather than let a walker clip it —
 * displacement is tried first because a dropped desk is a small loss of
 * density; a floating desk with no monitor or chair would not be.
 */
function resolveGroup(floor: number, anchor: { x: number; z: number }, prints: Footprint[], bounds: Block): { x: number; z: number } | null {
  if (!blocksAny(floor, anchor.x, anchor.z, prints)) return anchor
  for (let n = 1; n <= NUDGE_TRIES; n++) {
    const delta = n * NUDGE_STEP
    for (const [dirX, dirZ] of NUDGE_DIRS) {
      const x = anchor.x + dirX * delta
      const z = anchor.z + dirZ * delta
      const fits = prints.every(
        (p) => Math.abs(x + p.dx - bounds.x) <= bounds.w / 2 - p.hw && Math.abs(z + p.dz - bounds.z) <= bounds.d / 2 - p.hd
      )
      if (fits && !blocksAny(floor, x, z, prints)) return { x, z }
    }
  }
  return null
}

/** @param floor Index into `FLOORS`, so this room's furniture can be checked
 *  against the walk corridors that cross this specific floor. */
export function furnish(room: Room, floor: number): Furnishing {
  const out: Item[] = []
  const who: Occupant[] = []
  const r = rng(room.name)
  const y = (room.base ? PLATE : 0) + PLATE
  const blocks = room.blocks ?? [{ x: room.x, z: room.z, w: room.w, d: room.d }]
  // Glass partitions on the back two sides of any room big enough to be one.
  if (!room.blocks && room.w >= 2.4 && room.d >= 1.4 && room.kind !== 'action') {
    const h = 1.15
    out.push(item('partition', room.x, y + h / 2, room.z - room.d / 2, [room.w, h, 0.04], '#cfe3f5'))
    out.push(item('rail', room.x, y + h, room.z - room.d / 2, [room.w, 0.05, 0.06], DARK))
    out.push(item('partition', room.x - room.w / 2, y + h / 2, room.z, [0.04, h, room.d], '#cfe3f5'))
    out.push(item('rail', room.x - room.w / 2, y + h, room.z, [0.06, 0.05, room.d], DARK))
  }
  for (const b of blocks) {
    const long = b.w >= b.d
    switch (room.kind) {
      case 'form': {
        // Rows of workstations along the long axis.
        const len = (long ? b.w : b.d) - 0.5
        const dep = (long ? b.d : b.w) - 0.5
        const cols = Math.max(1, Math.floor(len / 1.45))
        const rows = Math.max(1, Math.floor(dep / 1.25))
        for (let i = 0; i < cols; i++) {
          for (let j = 0; j < rows; j++) {
            const u = -len / 2 + (i + 0.5) * (len / cols)
            const v = -dep / 2 + (j + 0.5) * (dep / rows) - 0.15
            const nx = long ? b.x + u : b.x + v
            const nz = long ? b.z + v : b.z + u
            const deskRy = long ? 0 : Math.PI / 2
            const at = resolveGroup(floor, { x: nx, z: nz }, [{ dx: 0, dz: 0, hw: DESK_W / 2, hd: DESK_D / 2, ry: deskRy }], b)
            if (!at) continue
            workstation(out, who, at.x, at.z, y, deskRy, r)
          }
        }
        if (b.w > 2 && b.d > 1.2) plant(out, b.x + b.w / 2 - 0.3, b.z - b.d / 2 + 0.3, y, r)
        break
      }
      case 'nav': {
        // A long counter with a sign, like a reception or a directory.
        const len = (long ? b.w : b.d) - 0.6
        const counterHW = (long ? len + 0.12 : 0.54) / 2
        const counterHD = (long ? 0.54 : len + 0.12) / 2
        const at = resolveGroup(floor, { x: b.x, z: b.z }, [{ dx: 0, dz: 0, hw: counterHW, hd: counterHD, ry: 0 }], b)
        if (at) {
          out.push(item('counter', at.x, y + 0.26, at.z, long ? [len, 0.52, 0.42] : [0.42, 0.52, len], '#6b4a2f'))
          out.push(item('counter', at.x, y + 0.545, at.z, long ? [len + 0.12, 0.05, 0.54] : [0.54, 0.05, len + 0.12], WHITE))
          if (len > 2.5) out.push(item('screen', at.x, y + 0.82, at.z, long ? [0.9, 0.42, 0.04] : [0.04, 0.42, 0.9], DARK))
        }
        plant(out, long ? b.x - len / 2 - 0.05 : b.x, long ? b.z : b.z - len / 2 - 0.05, y, r)
        // Standing, not seated: this one is staffing the counter.
        if (b.w > 1.8 && b.d > 1.0 && r() < 0.8)
          person(who, b.x + (long ? 0.4 : 0.35), b.z + (long ? 0.35 : 0.4), y, Math.PI, r, false)
        break
      }
      case 'content': {
        // Library lounge: shelves along the back, a sofa and coffee table.
        const len = b.w - 0.4
        if (len > 1.2) {
          const shelfAnchor = { x: b.x, z: b.z - b.d / 2 + 0.17 }
          const at = resolveGroup(floor, shelfAnchor, [{ dx: 0, dz: 0, hw: len / 2, hd: 0.15, ry: 0 }], b)
          if (at) {
            const dx = at.x - shelfAnchor.x
            const dz = at.z - shelfAnchor.z
            out.push(item('shelf', at.x, y + 0.5, at.z, [len, 1.0, 0.3], '#8b6a4a'))
            const n = Math.floor(len / 0.16)
            for (let i = 0; i < n; i++) {
              if (r() < 0.2) continue
              const bx = b.x - len / 2 + 0.1 + i * 0.16 + dx
              const bh = 0.2 + r() * 0.12
              const row = r() < 0.5 ? 0.18 : 0.62
              out.push(item('book', bx, y + row + bh / 2, b.z - b.d / 2 + 0.16 + dz, [0.1, bh, 0.22], BOOKS[Math.floor(r() * BOOKS.length)]))
            }
          }
        }
        if (b.d > 1.3) {
          const sz = b.z + b.d / 2 - 0.45
          const sofaW = Math.min(1.5, b.w - 0.6)
          const nookAnchor = { x: b.x - 0.2, z: sz }
          const prints: Footprint[] = [
            { dx: 0, dz: 0, hw: sofaW / 2, hd: 0.3, ry: 0 },
            { dx: 0, dz: -0.75, hw: 0.25, hd: 0.25, ry: 0 },
          ]
          const at = resolveGroup(floor, nookAnchor, prints, b)
          if (at) {
            out.push(item('sofa', at.x, y + 0.22, at.z, [sofaW, 0.44, 0.6], '#7f8fb8'))
            out.push(item('sofa', at.x, y + 0.55, at.z - 0.24, [sofaW, 0.28, 0.12], '#6d7ca3'))
            out.push(item('table', at.x, y + 0.17, at.z - 0.75, [0.5, 0.34, 0.5], WOOD))
            if (r() < 0.7) person(who, at.x - 0.3, at.z, y, 0, r)
          }
        }
        plant(out, b.x + b.w / 2 - 0.32, b.z + b.d / 2 - 0.32, y, r, b.w > 3)
        break
      }
      case 'data': {
        // Meeting room: table, chairs, a wall screen.
        const tl = Math.min(2.6, (long ? b.w : b.d) - 1.1)
        const tw = Math.min(1.0, (long ? b.d : b.w) - 1.0)
        const tableHW = (long ? tl : tw) / 2
        const tableHD = (long ? tw : tl) / 2
        const prints: Footprint[] = [
          { dx: 0, dz: 0, hw: 0.25, hd: 0.25, ry: 0 },
          { dx: 0, dz: 0, hw: tableHW, hd: tableHD, ry: 0 },
        ]
        const at = resolveGroup(floor, { x: b.x, z: b.z }, prints, b)
        if (at) {
          const dx = at.x - b.x
          const dz = at.z - b.z
          out.push(item('table', at.x, y + 0.2, at.z, [0.5, 0.4, 0.5], DARK))
          out.push(item('desk', at.x, y + 0.42, at.z, long ? [tl, 0.06, tw] : [tw, 0.06, tl], WHITE))
          const n = Math.max(2, Math.floor(tl / 0.7))
          for (let i = 0; i < n; i++) {
            const u = -tl / 2 + (i + 0.5) * (tl / n)
            for (const side of [-1, 1]) {
              const v = side * (tw / 2 + 0.32)
              const x = (long ? b.x + u : b.x + v) + dx
              const z = (long ? b.z + v : b.z + u) + dz
              out.push(item('chair', x, y + 0.22, z, [0.38, 0.44, 0.38], '#3b3f4a'))
              if (r() < 0.55) person(who, x, z, y, long ? (side > 0 ? Math.PI : 0) : side > 0 ? -Math.PI / 2 : Math.PI / 2, r)
            }
          }
          out.push(item('screen', at.x + (long ? 0 : -b.w / 2 + 0.12), y + 0.95, at.z + (long ? -b.d / 2 + 0.12 : 0), long ? [1.3, 0.72, 0.05] : [0.05, 0.72, 1.3], DARK))
        }
        plant(out, b.x + b.w / 2 - 0.3, b.z + b.d / 2 - 0.3, y, r)
        break
      }
      case 'action':
        // Kiosks are built by Tower.tsx so they can move (self-healing demo).
        break
    }
  }
  return { items: out, people: who }
}
