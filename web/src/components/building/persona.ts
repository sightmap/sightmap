// What makes a bank look like a bank. `furnish.ts` dresses a room by its
// kind; this dresses a whole building by the persona its listing's category
// earned it, so a storefront reads as a shop and a workshop as a workshop
// from across the room.
//
// Two halves, both returning the same cheap `Item` primitives furnish.ts
// returns (a box, a cylinder, a sphere, a capsule, scaled and coloured), so
// Tower.tsx keeps drawing one instanced mesh per type per floor:
//
//   - street level: what stands in the open front aisle of the ground floor
//     (an awning, a colonnade, a marquee, a departures board, a counter),
//   - room treatments: what fills the rooms the persona cares about (stacks,
//     seats, beds, a vault), on every floor.
//
// `variant` is the blueprint's 0/1/2 and moves at least one thing per
// archetype, so two banks in the same street are not the same bank.
import type { Archetype } from '@/types/blueprint'
import { FLOOR_D, FLOOR_H, FLOOR_W, PLATE, SLAB_T, type Floor, type Room } from './model'
import type { Item, ItemType } from './furnish'

/** Clear height under the slab above, for a storey of the default height. */
const CEIL = FLOOR_H - SLAB_T
/** The open +Z edge of the floor plate: the street side of the dollhouse. */
const FRONT = FLOOR_D / 2
/** Rooms are packed behind this; everything in front of it is the aisle. */
const BAND = 2.3

const WOOD = '#d8bf9a'
const DARKWOOD = '#6f5236'
const DARK = '#2b2d33'
const WHITE = '#f6f2ea'
const CREAM = '#e9e2d6'
const STEEL = '#9aa3ad'
const BOOKS = ['#c9456d', '#6b8aed', '#2d8a5e', '#b8860b', '#9b7ae8', '#e0d6c8', '#3d3929']

function it(
  type: ItemType,
  x: number,
  y: number,
  z: number,
  s: [number, number, number],
  color: string,
  ry = 0,
  rx = 0
): Item {
  return { type, x, y, z, ry, rx, sx: s[0], sy: s[1], sz: s[2], color }
}

// mulberry32 off a string, the same shape furnish.ts uses, so a persona is
// stable per building and per floor.
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

const v3 = (variant: number): 0 | 1 | 2 => (((Math.trunc(variant) % 3) + 3) % 3) as 0 | 1 | 2

/** Deck height inside a room, matching furnish.ts's own `y`. */
const roomY = (room: Room): number => (room.base ? PLATE : 0) + PLATE

/**
 * The rooms a persona dresses: the kinds it asks for first, largest first,
 * falling back to the biggest rooms of any other kind. A scan often finds a
 * building with no room of the kind the metaphor wants — a library of pure
 * `action` tools is still a library — and an empty persona would be worse
 * than one that borrows a room.
 */
function pickRooms(floor: Floor, prefer: Room['kind'][], n: number): Room[] {
  const area = (r: Room) => r.w * r.d
  const big = (a: Room, b: Room) => area(b) - area(a)
  const wanted = floor.rooms.filter((r) => prefer.includes(r.kind)).sort(big)
  if (wanted.length >= n) return wanted.slice(0, n)
  const rest = floor.rooms
    .filter((r) => !prefer.includes(r.kind) && r.kind !== 'action' && area(r) > 1.2)
    .sort(big)
  return [...wanted, ...rest].slice(0, n)
}

function person(out: Item[], x: number, z: number, y: number, ry: number, r: () => number): void {
  const shirt = ['#6e7fa8', '#b8a58c', '#4f5d75', '#a86e7c', '#7c9a86'][Math.floor(r() * 5)]
  const skin = ['#e9c4a3', '#c9976f', '#8d5a3b', '#f0d5bd'][Math.floor(r() * 4)]
  out.push(it('body', x, y + 0.24, z, [0.26, 0.34, 0.22], shirt, ry))
  out.push(it('head', x, y + 0.52, z, [0.17, 0.17, 0.17], skin))
}

/** A run of shelving with books on two rows. */
function shelving(
  out: Item[],
  x: number,
  y: number,
  z: number,
  len: number,
  h: number,
  depth: number,
  r: () => number,
  ry = 0
): void {
  // Books sit proud of the carcass on the side the camera is on, so a run of
  // shelving reads as books rather than as a plank.
  const face = depth * 0.34
  const along = (u: number): [number, number] =>
    ry === 0 ? [x + u, z + face] : [x + face, z + u]
  const size = (a: number, b: number): [number, number, number] =>
    ry === 0 ? [a, h, b] : [b, h, a]
  out.push(it('shelf', x, y + h / 2, z, size(len, depth), DARKWOOD))
  const shelves = h > 1.3 ? 3 : 2
  const n = Math.floor(len / 0.17)
  for (let i = 0; i < n; i++) {
    if (r() < 0.22) continue
    const u = -len / 2 + 0.09 + i * 0.17
    const [bx, bz] = along(u)
    const bh = 0.22 + r() * 0.14
    for (let k = 0; k < shelves; k++) {
      if (r() < 0.22) continue
      const sy = y + 0.16 + (k * (h - 0.3)) / shelves
      out.push(it('book', bx, sy + bh / 2, bz, size(0.11, depth * 0.6), BOOKS[Math.floor(r() * BOOKS.length)]))
    }
  }
}

// ---------------------------------------------------------------------------
// Street level: the open front aisle of the ground floor.

const AWNING_COLORS = ['#c9456d', '#2f7d5e', '#b8860b']
const CANOPY_COLORS = ['#2f7d8f', '#3f8f6a', '#4a7fb8']

/** A striped canopy hung over the open face, sloping down toward the street. */
function canopy(out: Item[], color: string, stripes: number, width: number, y: number): void {
  const bay = width / stripes
  for (let i = 0; i < stripes; i++) {
    out.push(
      it(
        'awning',
        -width / 2 + bay * (i + 0.5),
        y,
        FRONT + 0.42,
        [bay * 0.98, 0.07, 1.55],
        i % 2 === 0 ? color : CREAM,
        0,
        -0.36
      )
    )
  }
  // Valance along the outer lip, so the canopy has an edge from the front.
  out.push(it('awning', 0, y - 0.42, FRONT + 1.12, [width, 0.26, 0.06], color))
}

function streetLevel(archetype: Archetype, variant: number, seed: string, ceil: number): Item[] {
  const out: Item[] = []
  const r = rng(`${seed}|${archetype}|${variant}`)
  const v = v3(variant)
  switch (archetype) {
    case 'storefront': {
      canopy(out, AWNING_COLORS[v], 5 + v * 2, FLOOR_W * 0.86, 1.78)
      // The counter, with a till, and the shopkeeper behind it.
      out.push(it('counter', -2.1, 0.26, BAND + 0.5, [4.2, 0.52, 0.6], DARKWOOD))
      out.push(it('counter', -2.1, 0.545, BAND + 0.5, [4.34, 0.05, 0.72], WHITE))
      out.push(it('screen', -3.5, 0.72, BAND + 0.5, [0.44, 0.3, 0.3], DARK))
      person(out, -2.1, BAND + 0.15, 0, Math.PI, r)
      // Shelving rows facing the aisle, stocked.
      for (let i = 0; i < 2; i++) {
        shelving(out, 2.5, 0, BAND + 0.35 + i * 0.95, 3.6, 1.35, 0.34, r)
      }
      person(out, 3.6, BAND + 1.05, 0, 0.4, r)
      break
    }
    case 'bank': {
      const n = [4, 6, 8][v]
      const h = CEIL - 0.3
      const span = FLOOR_W - 1.6
      for (let i = 0; i < n; i++) {
        const x = -span / 2 + (span / (n - 1)) * i
        out.push(it('column', x, h / 2, FRONT - 0.45, [0.42, h, 0.42], CREAM))
        out.push(it('column', x, h - 0.07, FRONT - 0.45, [0.56, 0.14, 0.56], WHITE))
        out.push(it('column', x, 0.09, FRONT - 0.45, [0.58, 0.18, 0.58], WHITE))
      }
      // Entablature over the colonnade, and the steps up to it.
      out.push(it('awning', 0, h + 0.16, FRONT - 0.45, [span + 1.0, 0.3, 0.7], CREAM))
      out.push(it('awning', 0, h + 0.42, FRONT - 0.45, [span + 1.2, 0.22, 0.9], WHITE))
      for (let s = 0; s < 2; s++) {
        out.push(it('awning', 0, -0.04 - s * 0.08, FRONT - 1.15 - s * 0.22, [span + 1.2, 0.1, 0.5], '#d3cdc2'))
      }
      break
    }
    case 'workshop': {
      // Benches down the aisle, with tools laid out on them.
      const benches = 2 + (v === 2 ? 1 : 0)
      for (let i = 0; i < benches; i++) {
        const z = BAND + 0.3 + i * 0.62
        out.push(it('desk', -1.6, 0.42, z, [5.6, 0.12, 0.52], WOOD))
        for (const side of [-1, 1]) {
          for (const dx of [-2.4, 0, 2.4]) {
            out.push(it('column', -1.6 + dx, 0.18, z + side * 0.18, [0.09, 0.36, 0.09], STEEL))
          }
        }
        for (let k = 0; k < 4; k++) {
          out.push(
            it('crate', -3.9 + k * 1.45, 0.56, z, [0.3, 0.16, 0.26], ['#b8860b', '#6b8aed', '#c9456d', '#9aa3ad'][k])
          )
        }
      }
      // Crates stacked by the door.
      const stack = [
        [3.4, 0.26, BAND + 0.45, 0.52],
        [3.4, 0.78, BAND + 0.45, 0.5],
        [4.1, 0.3, BAND + 1.1, 0.6],
        [2.7, 0.3, BAND + 1.15, 0.6],
      ] as const
      stack.forEach(([x, y, z, s], k) => {
        out.push(it('crate', x, y, z, [s, s, s], k % 2 === 0 ? WOOD : '#c7b299', k * 0.3))
      })
      // The sawtooth skylight line, hung under the slab above.
      const teeth = [3, 4, 5][v]
      const bay = (FLOOR_W - 1.2) / teeth
      for (let i = 0; i < teeth; i++) {
        const x = -(FLOOR_W - 1.2) / 2 + bay * (i + 0.5)
        // The skylight is in the roof, not the furniture, so it hangs from
        // whatever ceiling this storey actually has.
        out.push(it('awning', x, ceil - 0.3, -0.2, [bay * 0.92, 0.06, FLOOR_D - 1.0], '#9ec6ea', 0, -0.46))
        out.push(it('column', x - bay * 0.46, ceil - 0.5, -0.2, [0.09, 0.52, FLOOR_D - 1.0], DARK))
        out.push(it('column', x + bay * 0.46, ceil - 0.2, -0.2, [0.09, 0.16, FLOOR_D - 1.0], DARK))
      }
      break
    }
    case 'theatre': {
      // The marquee: a canopy over the door with a rank of lamps under it.
      const w = FLOOR_W * 0.58
      out.push(it('awning', 0, 1.92, FRONT + 0.25, [w, 0.22, 1.5], '#3a2333'))
      out.push(it('awning', 0, 2.12, FRONT + 0.25, [w * 0.92, 0.18, 1.3], '#d9b45a'))
      out.push(it('awning', 0, 1.74, FRONT + 0.95, [w, 0.3, 0.1], '#d9b45a'))
      const bulbs = [8, 10, 12][v]
      for (let i = 0; i < bulbs; i++) {
        const x = -w / 2 + (w / (bulbs - 1)) * i
        out.push(it('screen', x, 1.74, FRONT + 0.98, [0.16, 0.16, 0.06], '#ffe6a8'))
        out.push(it('screen', x, 1.74, FRONT - 0.45, [0.16, 0.16, 0.06], '#ffe6a8'))
      }
      // Box office in the aisle.
      out.push(it('counter', -3.3, 0.55, BAND + 0.6, [1.5, 1.1, 0.8], '#3a2333'))
      out.push(it('screen', -3.3, 0.82, BAND + 1.02, [1.0, 0.4, 0.05], '#ffe6a8'))
      person(out, -3.3, BAND + 0.15, 0, Math.PI, r)
      // A rope line, because there is always a queue.
      for (let i = 0; i < 4; i++) {
        out.push(it('column', 0.4 + i * 1.1, 0.45, BAND + 1.0, [0.1, 0.9, 0.1], '#b8860b'))
      }
      out.push(it('rail', 1.05 + 1.1, 0.78, BAND + 1.0, [3.3, 0.05, 0.05], '#7a1f34'))
      break
    }
    case 'terminal': {
      // The departures board, on legs, facing the street.
      const bw = 5.2
      out.push(it('column', -bw / 2 + 0.2, 0.85, BAND + 0.2, [0.12, 1.7, 0.12], STEEL))
      out.push(it('column', bw / 2 - 0.2, 0.85, BAND + 0.2, [0.12, 1.7, 0.12], STEEL))
      out.push(it('screen', 0, 2.0, BAND + 0.2, [bw, 1.1, 0.12], '#14161c'))
      const rows = 4
      const cols = 5
      for (let i = 0; i < rows; i++) {
        for (let j = 0; j < cols; j++) {
          if (r() < 0.15) continue
          out.push(
            it(
              'screen',
              -bw / 2 + 0.45 + j * ((bw - 0.9) / (cols - 1)),
              2.42 - i * 0.24,
              BAND + 0.28,
              [0.5 + r() * 0.3, 0.1, 0.04],
              j === cols - 1 ? '#ffd58a' : '#8fd0a8'
            )
          )
        }
      }
      // The queue: bollards on a zigzag, roped together.
      const n = [5, 6, 7][v]
      const pts: [number, number][] = []
      for (let i = 0; i < n; i++) {
        pts.push([-4.0 + i * (8.0 / (n - 1)), BAND + 0.95 + (i % 2 === 0 ? 0 : 0.6)])
      }
      for (const [x, z] of pts) {
        out.push(it('column', x, 0.45, z, [0.14, 0.9, 0.14], STEEL))
        out.push(it('column', x, 0.92, z, [0.2, 0.1, 0.2], DARK))
      }
      for (let i = 0; i + 1 < pts.length; i++) {
        const [ax, az] = pts[i]
        const [bx, bz] = pts[i + 1]
        const len = Math.hypot(bx - ax, bz - az)
        out.push(it('rail', (ax + bx) / 2, 0.78, (az + bz) / 2, [len, 0.04, 0.04], '#2f4f6b', Math.atan2(bz - az, bx - ax)))
      }
      person(out, pts[1][0], pts[1][1] - 0.4, 0, 0.2, r)
      person(out, pts[2][0], pts[2][1] - 0.4, 0, -0.3, r)
      break
    }
    case 'clinic': {
      // A clean canopy over the door, and reception under it.
      canopy(out, CANOPY_COLORS[v], 3, FLOOR_W * 0.46, 1.72)
      // The one mark a clinic is known by, painted on top of the canopy where
      // the view from above reads it.
      out.push(it('awning', 0, 1.79, FRONT + 0.42, [1.3, 0.05, 0.4], WHITE, 0, -0.36))
      out.push(it('awning', 0, 1.79, FRONT + 0.42, [0.44, 0.05, 1.2], WHITE, 0, -0.36))
      out.push(it('counter', -1.9, 0.28, BAND + 0.55, [3.8, 0.56, 0.7], WHITE))
      out.push(it('counter', -1.9, 0.59, BAND + 0.55, [3.96, 0.06, 0.84], CANOPY_COLORS[v]))
      out.push(it('counter', 0.35, 0.28, BAND + 1.15, [0.7, 0.56, 1.3], WHITE))
      out.push(it('monitor', -2.6, 0.74, BAND + 0.45, [0.42, 0.26, 0.04], DARK, Math.PI))
      person(out, -1.9, BAND + 0.15, 0, Math.PI, r)
      // The waiting row.
      for (let i = 0; i < 4; i++) {
        const x = 1.9 + i * 0.62
        out.push(it('chair', x, 0.21, BAND + 0.8, [0.52, 0.42, 0.52], '#b4c6c4'))
        out.push(it('chair', x, 0.5, BAND + 1.04, [0.52, 0.36, 0.1], '#9fb4b2'))
      }
      person(out, 2.52, BAND + 0.8, 0.2, 0, r)
      break
    }
    case 'library': {
      // A reading table under the front light, with a lamp and readers.
      out.push(it('table', -1.2, 0.2, BAND + 0.75, [0.5, 0.4, 0.5], DARK))
      out.push(it('desk', -1.2, 0.44, BAND + 0.75, [3.4, 0.1, 1.2], DARKWOOD))
      out.push(it('pot', -1.2, 0.58, BAND + 0.75, [0.22, 0.18, 0.22], '#2f4f6b'))
      out.push(it('leaf', -1.2, 0.78, BAND + 0.75, [0.4, 0.34, 0.4], '#d9b45a'))
      for (let i = 0; i < 3; i++) {
        const x = -2.4 + i * 1.2
        for (const side of [-1, 1]) {
          out.push(it('chair', x, 0.22, BAND + 0.75 + side * 0.82, [0.4, 0.44, 0.4], '#6b4a2f'))
        }
      }
      person(out, -2.4, BAND + 0.75 - 0.82, 0.2, 0, r)
      person(out, -1.2, BAND + 0.75 + 0.82, 0.2, Math.PI, r)
      break
    }
    case 'office':
      break
  }
  return out
}

// ---------------------------------------------------------------------------
// Room treatments: what the persona puts inside the rooms, on every floor.

function roomTreatment(archetype: Archetype, variant: number, floor: Floor, seed: string): Item[] {
  const out: Item[] = []
  const r = rng(`${seed}|rooms|${archetype}`)
  const v = v3(variant)
  switch (archetype) {
    case 'library': {
      // Stacks: rows of thin tall boxes, denser on the higher variants.
      // One run down the left of every floor. Shorter than the floor and no
      // taller than a person, so the stacks read from outside without walling
      // the rooms behind them off.
      shelving(out, -1.5, 0, 2.62, FLOOR_W - 4.2, 1.24, 0.3, r)
      const rows = 3 + v
      for (const room of pickRooms(floor, ['content', 'data'], 6)) {
        const y = roomY(room)
        const long = room.w >= room.d
        const len = (long ? room.w : room.d) - 0.35
        const dep = (long ? room.d : room.w) - 0.3
        const n = Math.max(2, Math.min(rows, Math.floor(dep / 0.38)))
        for (let i = 0; i < n; i++) {
          const off = -dep / 2 + (dep / n) * (i + 0.5)
          const x = long ? room.x : room.x + off
          const z = long ? room.z + off : room.z
          shelving(out, x, y, z, len, 1.62, 0.3, r, long ? 0 : Math.PI / 2)
        }
      }
      break
    }
    case 'theatre': {
      // Seats facing a low stage at the back of the largest room.
      const rows = 3 + v
      for (const room of pickRooms(floor, ['content', 'data'], 1)) {
        const y = roomY(room)
        out.push(it('awning', room.x, y + 0.09, room.z - room.d / 2 + 0.28, [room.w - 0.3, 0.18, 0.5], '#3a2333'))
        const cols = Math.max(2, Math.floor((room.w - 0.5) / 0.5))
        const depth = room.d - 0.9
        for (let i = 0; i < rows; i++) {
          const z = room.z - room.d / 2 + 0.72 + (depth / rows) * (i + 0.5)
          for (let j = 0; j < cols; j++) {
            const x = room.x - (room.w - 0.5) / 2 + ((room.w - 0.5) / cols) * (j + 0.5)
            out.push(it('chair', x, y + 0.16, z, [0.4, 0.32, 0.34], '#8e2740'))
            out.push(it('chair', x, y + 0.46, z + 0.15, [0.4, 0.36, 0.1], '#7a1f34'))
          }
        }
      }
      break
    }
    case 'clinic': {
      // Beds, curtained, two or three to a room.
      const beds = 2 + (v === 2 ? 1 : 0)
      for (const room of pickRooms(floor, ['content', 'data'], 2)) {
        const y = roomY(room)
        const n = Math.max(1, Math.min(beds, Math.floor((room.w - 0.3) / 0.95)))
        for (let i = 0; i < n; i++) {
          const x = room.x - (room.w - 0.5) / 2 + ((room.w - 0.5) / n) * (i + 0.5)
          const z = room.z - 0.1
          out.push(it('bed', x, y + 0.24, z, [0.7, 0.2, 1.5], WHITE))
          out.push(it('column', x, y + 0.12, z, [0.6, 0.24, 1.4], STEEL))
          out.push(it('crate', x, y + 0.38, z - 0.52, [0.5, 0.12, 0.3], '#cdd8d6'))
          out.push(it('rail', x + 0.45, y + 0.55, z, [0.05, 1.1, 0.05], STEEL))
          out.push(it('partition', x + 0.45, y + 0.75, z, [0.03, 0.9, 1.5], '#dbe7e5'))
        }
      }
      break
    }
    case 'bank': {
      // A strongroom: thick dark walls round the data room, open at the
      // front, with the door leaf swung back against one of them.
      for (const room of pickRooms(floor, ['data', 'content'], 1)) {
        const y = roomY(room)
        const h = 1.5
        const t = 0.2
        out.push(it('crate', room.x, y + h / 2, room.z - room.d / 2 + t / 2, [room.w, h, t], '#4a4d57'))
        for (const side of [-1, 1]) {
          out.push(it('crate', room.x + (side * (room.w - t)) / 2, y + h / 2, room.z, [t, h, room.d], '#41444d'))
        }
        out.push(it('column', room.x + room.w / 2 - 0.55, y + 0.62, room.z + room.d / 2 - 0.12, [1.05, 0.16, 1.05], '#6f7480', 0, Math.PI / 2))
        out.push(it('column', room.x + room.w / 2 - 0.55, y + 0.62, room.z + room.d / 2 - 0.02, [0.3, 0.1, 0.3], '#d9b45a', 0, Math.PI / 2))
        // Bullion, stacked where the door can see it.
        for (let i = 0; i < 6; i++) {
          out.push(
            it('crate', room.x - room.w / 2 + 0.45 + (i % 3) * 0.34, y + 0.06 + Math.floor(i / 3) * 0.13, room.z - 0.1, [0.3, 0.12, 0.18], '#d9b45a')
          )
        }
      }
      break
    }
    case 'workshop': {
      // Crates against the back wall of a couple of rooms.
      for (const room of pickRooms(floor, ['content', 'data'], 2)) {
        const y = roomY(room)
        for (let i = 0; i < 3; i++) {
          const s = 0.42 + r() * 0.16
          out.push(
            it('crate', room.x - room.w / 2 + 0.4 + i * 0.62, y + s / 2, room.z - room.d / 2 + 0.35, [s, s, s], i % 2 ? WOOD : '#c7b299', r() * 0.4)
          )
        }
      }
      break
    }
    case 'storefront': {
      // Display shelving along the back of the display rooms.
      for (const room of pickRooms(floor, ['content'], 2)) {
        const y = roomY(room)
        shelving(out, room.x, y, room.z - room.d / 2 + 0.2, room.w - 0.3, 1.2, 0.3, r)
      }
      break
    }
    case 'terminal':
    case 'office':
      break
  }
  return out
}

/**
 * Everything the persona adds to one floor. `floorIndex` 0 gets the street
 * level as well; every floor gets the room treatments.
 */
export function personaFurnish(
  archetype: Archetype,
  variant: number,
  floor: Floor,
  floorIndex: number,
  seed: string,
  /** Clear height of this storey, for the pieces that hang from its ceiling. */
  ceil: number = CEIL
): Item[] {
  const out = roomTreatment(archetype, variant, floor, seed)
  if (floorIndex === 0) out.push(...streetLevel(archetype, variant, seed, ceil))
  return out
}
