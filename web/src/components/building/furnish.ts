// Procedural furniture. Every component kind gets a recognisable interior —
// workstations for forms, a counter for navigation, a library lounge for
// content, a meeting room for data — laid out deterministically from the
// room's footprint so the blueprint sheets and the built floors agree.
// Items are cheap primitives (unit box / cylinder / sphere / capsule) with a
// per-item scale and colour; Tower.tsx renders each type as one instanced
// mesh per floor.
import { PLATE, type Room } from './model'

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
  | 'body'
  | 'head'
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

function person(out: Item[], x: number, z: number, y: number, ry: number, r: () => number): void {
  const shirt = SHIRT[Math.floor(r() * SHIRT.length)]
  const skin = SKIN[Math.floor(r() * SKIN.length)]
  out.push(item('body', x, y + 0.24, z, [0.26, 0.34, 0.22], shirt, ry))
  out.push(item('head', x, y + 0.52, z, [0.17, 0.17, 0.17], skin))
}

/** A desk facing +z (chair on the +z side). `ry` rotates the whole cluster. */
function workstation(out: Item[], cx: number, cz: number, y: number, ry: number, r: () => number): void {
  const c = Math.cos(ry)
  const s = Math.sin(ry)
  const at = (dx: number, dz: number): [number, number] => [cx + dx * c - dz * s, cz + dx * s + dz * c]
  const [dx, dz] = at(0, 0)
  out.push(item('desk', dx, y + 0.2, dz, [1.15, 0.4, 0.55], WOOD, ry))
  const [mx, mz] = at(0, -0.12)
  out.push(item('monitor', mx, y + 0.56, mz, [0.46, 0.28, 0.04], DARK, ry))
  const [chx, chz] = at(0, 0.52)
  out.push(item('chair', chx, y + 0.22, chz, [0.4, 0.44, 0.4], '#3b3f4a', ry))
  if (r() < 0.6) person(out, chx, chz, y + 0.2, ry + Math.PI, r)
}

export function furnish(room: Room): Item[] {
  const out: Item[] = []
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
            const x = long ? b.x + u : b.x + v
            const z = long ? b.z + v : b.z + u
            workstation(out, x, z, y, long ? 0 : Math.PI / 2, r)
          }
        }
        if (b.w > 2 && b.d > 1.2) plant(out, b.x + b.w / 2 - 0.3, b.z - b.d / 2 + 0.3, y, r)
        break
      }
      case 'nav': {
        // A long counter with a sign, like a reception or a directory.
        const len = (long ? b.w : b.d) - 0.6
        out.push(item('counter', b.x, y + 0.26, b.z, long ? [len, 0.52, 0.42] : [0.42, 0.52, len], '#6b4a2f'))
        out.push(item('counter', b.x, y + 0.545, b.z, long ? [len + 0.12, 0.05, 0.54] : [0.54, 0.05, len + 0.12], WHITE))
        if (len > 2.5) out.push(item('screen', b.x, y + 0.82, b.z, long ? [0.9, 0.42, 0.04] : [0.04, 0.42, 0.9], DARK))
        plant(out, long ? b.x - len / 2 - 0.05 : b.x, long ? b.z : b.z - len / 2 - 0.05, y, r)
        if (b.w > 1.8 && b.d > 1.0 && r() < 0.8) person(out, b.x + (long ? 0.4 : 0.35), b.z + (long ? 0.35 : 0.4), y, Math.PI, r)
        break
      }
      case 'content': {
        // Library lounge: shelves along the back, a sofa and coffee table.
        const len = b.w - 0.4
        if (len > 1.2) {
          out.push(item('shelf', b.x, y + 0.5, b.z - b.d / 2 + 0.17, [len, 1.0, 0.3], '#8b6a4a'))
          const n = Math.floor(len / 0.16)
          for (let i = 0; i < n; i++) {
            if (r() < 0.2) continue
            const bx = b.x - len / 2 + 0.1 + i * 0.16
            const bh = 0.2 + r() * 0.12
            const row = r() < 0.5 ? 0.18 : 0.62
            out.push(item('book', bx, y + row + bh / 2, b.z - b.d / 2 + 0.16, [0.1, bh, 0.22], BOOKS[Math.floor(r() * BOOKS.length)]))
          }
        }
        if (b.d > 1.3) {
          const sz = b.z + b.d / 2 - 0.45
          const sofaW = Math.min(1.5, b.w - 0.6)
          out.push(item('sofa', b.x - 0.2, y + 0.22, sz, [sofaW, 0.44, 0.6], '#7f8fb8'))
          out.push(item('sofa', b.x - 0.2, y + 0.55, sz - 0.24, [sofaW, 0.28, 0.12], '#6d7ca3'))
          out.push(item('table', b.x - 0.2, y + 0.17, sz - 0.75, [0.5, 0.34, 0.5], WOOD))
          if (r() < 0.7) person(out, b.x - 0.5, sz, y + 0.2, 0, r)
        }
        plant(out, b.x + b.w / 2 - 0.32, b.z + b.d / 2 - 0.32, y, r, b.w > 3)
        break
      }
      case 'data': {
        // Meeting room: table, chairs, a wall screen.
        const tl = Math.min(2.6, (long ? b.w : b.d) - 1.1)
        const tw = Math.min(1.0, (long ? b.d : b.w) - 1.0)
        out.push(item('table', b.x, y + 0.2, b.z, [0.5, 0.4, 0.5], DARK))
        out.push(item('desk', b.x, y + 0.42, b.z, long ? [tl, 0.06, tw] : [tw, 0.06, tl], WHITE))
        const n = Math.max(2, Math.floor(tl / 0.7))
        for (let i = 0; i < n; i++) {
          const u = -tl / 2 + (i + 0.5) * (tl / n)
          for (const side of [-1, 1]) {
            const v = side * (tw / 2 + 0.32)
            const x = long ? b.x + u : b.x + v
            const z = long ? b.z + v : b.z + u
            out.push(item('chair', x, y + 0.22, z, [0.38, 0.44, 0.38], '#3b3f4a'))
            if (r() < 0.55) person(out, x, z, y + 0.2, long ? (side > 0 ? Math.PI : 0) : side > 0 ? -Math.PI / 2 : Math.PI / 2, r)
          }
        }
        out.push(item('screen', b.x + (long ? 0 : -b.w / 2 + 0.12), y + 0.95, b.z + (long ? -b.d / 2 + 0.12 : 0), long ? [1.3, 0.72, 0.05] : [0.05, 0.72, 1.3], DARK))
        plant(out, b.x + b.w / 2 - 0.3, b.z + b.d / 2 - 0.3, y, r)
        break
      }
      case 'action':
        // Kiosks are built by Tower.tsx so they can move (self-healing demo).
        break
    }
  }
  return out
}
