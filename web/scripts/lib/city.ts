// The city plan and the lot assignment on it. Pure: no filesystem, no clock,
// no `Math.random`, no three.js — `planCity()` is a constant, and
// `assignLots()` is a function of the listings alone.
//
// The plan and the listings are deliberately separate. Buildings that move
// between builds would make the city useless as a place you can learn, so the
// ground is fixed first and listings are laid onto it by a hash-stable probe
// whose result is written back into the listing YAML. A rescan changes a
// building's floors; it never changes its address.
//
// The shape of the place, from the research: a road hierarchy and blocks of
// uneven size read as "city" far more cheaply than curves do, so the streets
// come from recursive splitting rather than a grid; the one diagonal exists to
// break the right angles and to make wedge sites for landmarks; and filler
// massing on the good lots plus fenced empty lots on the poor ones is what
// makes ten listings look like a town rather than ten sheds in a field.
import type { Archetype } from '../../src/types/blueprint'
import type { ListingMeta } from '../../src/types/directory'
import type {
  CityAssignment,
  CityBlock,
  CityDistrict,
  CityEdge,
  CityFill,
  CityLandmark,
  CityLoop,
  CityLot,
  CityPlan,
  CityProp,
  CityRoad,
  CityTier,
} from '../../src/types/city'
import { archetypeFor } from './blueprint'

/** The plan never changes unless this string does. */
export const CITY_SEED = 'sightmap-atlas-city'

// ---------------------------------------------------------------- dimensions

const BOUNDS = { w: 320, d: 240 }
/** Half-diagonal of the bounds: the distance at which centrality reaches 0. */
const HALF_DIAGONAL = Math.hypot(BOUNDS.w / 2, BOUNDS.d / 2)

const AVENUE_W = 8
const STREET_W = 5
const STUB_W = 4
const PATH_W = 4

/** The avenue ring, as the half-extents of its centreline rectangle. */
const RING = { x: 140, z: 100 }
/** Half-side of the paved square where the two cross avenues meet. */
const PLAZA_R = 14

/** A block is split until its longest side fits, and never below the minimum. */
const MIN_BLOCK = 24
const MAX_BLOCK = 44
/** Smallest side that can still be split into two legal blocks plus a street. */
const SPLITTABLE = MIN_BLOCK * 2 + STREET_W

/** "Within one block of" a landmark, for the centrality bonus. */
const NEAR_LANDMARK = MAX_BLOCK

/** Lot frontage and depth run from these at the rim to these at the plaza. */
const RIM_LOT = { frontage: 18, depth: 16 }
const CORE_LOT = { frontage: 14, depth: 12 }
/** Smallest side a lot the diagonal clips may be shrunk to before it is lost. */
const MIN_WEDGE = 5
/** Air between neighbouring lots, so rounded rectangles never touch. */
const LOT_GAP = 0.1

/** Cul-de-sac stubs: where they leave the ring, how far out, and the circle. */
const STUB_LEN = 14
const STUB_CIRCLE = 6

/** Unassigned lots above this centrality get filler massing, below it dirt. */
const FILLER_CENTRALITY = 0.45

// ------------------------------------------------------------------ seeding

/** 32-bit FNV-1a. The same hash the blueprint seeds from, so slugs agree. */
export function fnv1a(input: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < input.length; i++) {
    h ^= input.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return h >>> 0
}

/**
 * A labelled sub-seed. Every seeded choice draws from its own stream so that
 * adding a rule later cannot reshuffle the choices made before it.
 */
function subSeed(label: string): number {
  return fnv1a(`${CITY_SEED}:${label}`)
}

/** mulberry32: small, fast, and good enough for placing kerbs. */
function rng(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** Coordinates are rounded so the JSON is small and two builds compare equal. */
const r2 = (n: number): number => Math.round(n * 100) / 100
const clamp01 = (n: number): number => (n < 0 ? 0 : n > 1 ? 1 : n)

// ----------------------------------------------------------------- geometry

type Pt = [number, number]
interface Rect {
  x: number
  z: number
  w: number
  d: number
}

function pointSegDist(px: number, pz: number, a: Pt, b: Pt): number {
  const dx = b[0] - a[0]
  const dz = b[1] - a[1]
  const len2 = dx * dx + dz * dz
  if (len2 === 0) return Math.hypot(px - a[0], pz - a[1])
  let t = ((px - a[0]) * dx + (pz - a[1]) * dz) / len2
  t = t < 0 ? 0 : t > 1 ? 1 : t
  return Math.hypot(px - (a[0] + t * dx), pz - (a[1] + t * dz))
}

export function pointPolylineDist(px: number, pz: number, points: Pt[]): number {
  let best = Infinity
  for (let i = 0; i + 1 < points.length; i++) {
    const d = pointSegDist(px, pz, points[i], points[i + 1])
    if (d < best) best = d
  }
  return best
}

function pointRectDist(px: number, pz: number, r: Rect): number {
  const dx = Math.max(Math.abs(px - r.x) - r.w / 2, 0)
  const dz = Math.max(Math.abs(pz - r.z) - r.d / 2, 0)
  return Math.hypot(dx, dz)
}

function segsCross(a: Pt, b: Pt, c: Pt, d: Pt): boolean {
  const side = (p: Pt, q: Pt, r: Pt) => (q[0] - p[0]) * (r[1] - p[1]) - (q[1] - p[1]) * (r[0] - p[0])
  const d1 = side(a, b, c)
  const d2 = side(a, b, d)
  const d3 = side(c, d, a)
  const d4 = side(c, d, b)
  return ((d1 > 0) !== (d2 > 0)) && ((d3 > 0) !== (d4 > 0))
}

/**
 * Shortest distance between a segment and an axis-aligned rectangle. When they
 * do not touch, the closest pair is always a rectangle corner against the
 * segment or a segment endpoint against the rectangle, so those are the only
 * candidates worth testing.
 */
function segRectDist(a: Pt, b: Pt, r: Rect): number {
  const x0 = r.x - r.w / 2
  const x1 = r.x + r.w / 2
  const z0 = r.z - r.d / 2
  const z1 = r.z + r.d / 2
  const corners: Pt[] = [
    [x0, z0],
    [x1, z0],
    [x1, z1],
    [x0, z1],
  ]
  for (let i = 0; i < 4; i++) {
    if (segsCross(a, b, corners[i], corners[(i + 1) % 4])) return 0
  }
  let best = Math.min(pointRectDist(a[0], a[1], r), pointRectDist(b[0], b[1], r))
  for (const c of corners) best = Math.min(best, pointSegDist(c[0], c[1], a, b))
  return best
}

function polylineRectDist(points: Pt[], r: Rect): number {
  let best = Infinity
  for (let i = 0; i + 1 < points.length; i++) {
    const d = segRectDist(points[i], points[i + 1], r)
    if (d < best) best = d
    if (best === 0) break
  }
  return best
}

function rectsOverlap(a: Rect, b: Rect): boolean {
  return (
    Math.abs(a.x - b.x) < (a.w + b.w) / 2 - 1e-9 && Math.abs(a.z - b.z) < (a.d + b.d) / 2 - 1e-9
  )
}

/** Offsets a polyline sideways by `by`, mitring each interior vertex. */
function offsetPolyline(points: Pt[], by: number): Pt[] {
  const out: Pt[] = []
  for (let i = 0; i < points.length; i++) {
    const prev = points[Math.max(i - 1, 0)]
    const next = points[Math.min(i + 1, points.length - 1)]
    const dx = next[0] - prev[0]
    const dz = next[1] - prev[1]
    const len = Math.hypot(dx, dz) || 1
    out.push([r2(points[i][0] + (dz / len) * by), r2(points[i][1] - (dx / len) * by)])
  }
  return out
}

// ------------------------------------------------------------------- roads

const RING_POINTS: Pt[] = [
  [-RING.x, -RING.z],
  [RING.x, -RING.z],
  [RING.x, RING.z],
  [-RING.x, RING.z],
  [-RING.x, -RING.z],
]

/**
 * The diagonal: a real polyline with one kink, so it reads as a road that was
 * there before the grid rather than as a construction line.
 */
const DIAGONAL_POINTS: Pt[] = [
  [-RING.x, RING.z],
  [-60, 44],
  [0, 0],
]

/** Where the six cul-de-sac stubs leave the ring, and which way they point. */
const STUBS: { at: Pt; dir: Pt }[] = [
  { at: [-80, -RING.z], dir: [0, -1] },
  { at: [40, -RING.z], dir: [0, -1] },
  { at: [-40, RING.z], dir: [0, 1] },
  { at: [90, RING.z], dir: [0, 1] },
  { at: [-RING.x, -40], dir: [-1, 0] },
  { at: [RING.x, 55], dir: [1, 0] },
]

function baseRoads(): CityRoad[] {
  // Cloned, so a caller that edits one plan's roads cannot reach into the next.
  const clone = (pts: Pt[]): Pt[] => pts.map((p) => [p[0], p[1]] as Pt)
  const roads: CityRoad[] = [
    { id: 'ring', kind: 'avenue', tier: 2, width: AVENUE_W, points: clone(RING_POINTS), closed: true },
    {
      id: 'avenue-ns',
      kind: 'avenue',
      tier: 2,
      width: AVENUE_W,
      points: [
        [0, -RING.z],
        [0, RING.z],
      ],
      closed: false,
    },
    {
      id: 'avenue-ew',
      kind: 'avenue',
      tier: 2,
      width: AVENUE_W,
      points: [
        [-RING.x, 0],
        [RING.x, 0],
      ],
      closed: false,
    },
    { id: 'diagonal', kind: 'avenue', tier: 2, width: AVENUE_W, points: clone(DIAGONAL_POINTS), closed: false },
  ]
  STUBS.forEach((stub, i) => {
    const end: Pt = [stub.at[0] + stub.dir[0] * STUB_LEN, stub.at[1] + stub.dir[1] * STUB_LEN]
    roads.push({
      id: `stub-${i}`,
      kind: 'cul-de-sac',
      tier: 0,
      width: STUB_W,
      points: [stub.at, end],
      closed: false,
      turningCircle: STUB_CIRCLE,
    })
  })
  roads.push({
    id: 'plaza-path',
    kind: 'path',
    tier: 0,
    width: PATH_W,
    points: [
      [-PLAZA_R, -PLAZA_R],
      [PLAZA_R, -PLAZA_R],
      [PLAZA_R, PLAZA_R],
      [-PLAZA_R, PLAZA_R],
      [-PLAZA_R, -PLAZA_R],
    ],
    closed: true,
  })
  return roads
}

// ------------------------------------------------------------ block splitting

/** A side is done when it fits a block, or splittable when it can halve again. */
function feasible(side: number): boolean {
  return (side >= MIN_BLOCK && side <= MAX_BLOCK) || side >= SPLITTABLE
}

/**
 * Where to put the street inside a side of length `usable`. The seeded ratio
 * leads; when it lands in the dead band between "too big to keep" and "too
 * small to split again" it is snapped to the nearer end of that band, and only
 * then do the fixed fallbacks run.
 */
function splitAt(usable: number, ratio: number): number | null {
  const want = usable * ratio
  const snapped = want > MAX_BLOCK && want < SPLITTABLE ? (want - MAX_BLOCK < SPLITTABLE - want ? MAX_BLOCK : SPLITTABLE) : want
  const candidates = [want, snapped, MAX_BLOCK, usable - MAX_BLOCK, SPLITTABLE, usable - SPLITTABLE, usable / 2]
  for (const c of candidates) {
    if (feasible(c) && feasible(usable - c)) return c
  }
  return null
}

function splitQuadrant(rect: Rect, next: () => number, streets: Pt[][], out: Rect[]): void {
  const horizontal = rect.w >= rect.d
  const size = horizontal ? rect.w : rect.d
  if (size <= MAX_BLOCK) {
    out.push(rect)
    return
  }
  const usable = size - STREET_W
  const p = splitAt(usable, 0.35 + next() * 0.3)
  if (p === null) {
    out.push(rect)
    return
  }
  const b = size - STREET_W - p
  if (horizontal) {
    const x0 = rect.x - rect.w / 2
    const cut = x0 + p + STREET_W / 2
    streets.push([
      [r2(cut), r2(rect.z - rect.d / 2)],
      [r2(cut), r2(rect.z + rect.d / 2)],
    ])
    splitQuadrant({ x: r2(x0 + p / 2), z: rect.z, w: r2(p), d: rect.d }, next, streets, out)
    splitQuadrant({ x: r2(cut + STREET_W / 2 + b / 2), z: rect.z, w: r2(b), d: rect.d }, next, streets, out)
  } else {
    const z0 = rect.z - rect.d / 2
    const cut = z0 + p + STREET_W / 2
    streets.push([
      [r2(rect.x - rect.w / 2), r2(cut)],
      [r2(rect.x + rect.w / 2), r2(cut)],
    ])
    splitQuadrant({ x: rect.x, z: r2(z0 + p / 2), w: rect.w, d: r2(p) }, next, streets, out)
    splitQuadrant({ x: rect.x, z: r2(cut + STREET_W / 2 + b / 2), w: rect.w, d: r2(b) }, next, streets, out)
  }
}

const ALL_EDGES: CityEdge[] = ['n', 's', 'e', 'w']

/** The four quadrants the ring and the two cross avenues leave. */
function quadrantRects(): Rect[] {
  const inner = { x: RING.x - AVENUE_W / 2, z: RING.z - AVENUE_W / 2 }
  const half = AVENUE_W / 2
  const w = inner.x - half
  const d = inner.z - half
  const cx = half + w / 2
  const cz = half + d / 2
  return [
    { x: -cx, z: -cz, w, d },
    { x: cx, z: -cz, w, d },
    { x: -cx, z: cz, w, d },
    { x: cx, z: cz, w, d },
  ]
}

/**
 * The four bands between the ring and the edge of the ground. Each band stops
 * where the ring's straight runs do, so every rim lot has the kerb it fronts
 * directly in front of it and the four corners stay open ground.
 */
function rimBands(): { rect: Rect; edge: CityEdge; along: 'x' | 'z' }[] {
  const outer = { x: RING.x + AVENUE_W / 2, z: RING.z + AVENUE_W / 2 }
  const depth = BOUNDS.d / 2 - outer.z
  const width = BOUNDS.w / 2 - outer.x
  const bandZ = outer.z + depth / 2
  const bandX = outer.x + width / 2
  return [
    { rect: { x: 0, z: -bandZ, w: RING.x * 2, d: depth }, edge: 's', along: 'x' },
    { rect: { x: bandX, z: 0, w: width, d: RING.z * 2 }, edge: 'w', along: 'z' },
    { rect: { x: 0, z: bandZ, w: RING.x * 2, d: depth }, edge: 'n', along: 'x' },
    { rect: { x: -bandX, z: 0, w: width, d: RING.z * 2 }, edge: 'e', along: 'z' },
  ]
}

/** Chops a rim band into blocks no longer than a block, without adding streets. */
function chopBand(rect: Rect, along: 'x' | 'z'): Rect[] {
  const length = along === 'x' ? rect.w : rect.d
  const n = Math.max(1, Math.ceil(length / MAX_BLOCK))
  const step = length / n
  const start = (along === 'x' ? rect.x - rect.w / 2 : rect.z - rect.d / 2) + step / 2
  const out: Rect[] = []
  for (let i = 0; i < n; i++) {
    const c = r2(start + i * step)
    out.push(along === 'x' ? { x: c, z: rect.z, w: r2(step), d: rect.d } : { x: rect.x, z: c, w: rect.w, d: r2(step) })
  }
  return out
}

// --------------------------------------------------------------- districts

/**
 * Eight hand-placed seeds, one per archetype. A lot's district is its nearest
 * seed, so the placement — not a radius — is what gives each archetype its
 * share of the ground: the plaza to the bank, the avenues to the shopfronts
 * and the theatre, the diagonal and the east rim to the makers and the depot,
 * the quiet quadrants to the clinic and the library, and the rest to offices.
 */
const DISTRICTS: CityDistrict[] = [
  { archetype: 'bank', name: 'Exchange', x: 12, z: -12, accent: 0 },
  { archetype: 'storefront', name: 'Market Row', x: -56, z: 8, accent: 1 },
  { archetype: 'theatre', name: 'The Strand', x: 10, z: 56, accent: 2 },
  { archetype: 'workshop', name: 'Foundry', x: -78, z: 58, accent: 3 },
  { archetype: 'terminal', name: 'Eastgate', x: 124, z: 40, accent: 4 },
  { archetype: 'clinic', name: 'Northfield', x: -104, z: -64, accent: 5 },
  { archetype: 'library', name: 'Scholars', x: 86, z: -70, accent: 6 },
  { archetype: 'office', name: 'Midtown', x: 56, z: 18, accent: 7 },
]

function districtAt(x: number, z: number): Archetype {
  let best = DISTRICTS[0]
  let bestD = Infinity
  for (const d of DISTRICTS) {
    const dist = Math.hypot(x - d.x, z - d.z)
    if (dist < bestD - 1e-9) {
      bestD = dist
      best = d
    }
  }
  return best.archetype
}

// -------------------------------------------------------------- centrality

/** Distance-to-plaza falloff, before the landmark bonus. */
function baseCentrality(x: number, z: number): number {
  return clamp01(1 - Math.hypot(x, z) / HALF_DIAGONAL)
}

/**
 * Centrality at a point: the plaza falloff plus 0.15 within one block of a
 * launch landmark, clamped. It is what makes the skyline a mound rather than a
 * slab, and what decides which lots are worth filling in.
 */
export function centralityAt(plan: CityPlan, x: number, z: number): number {
  let c = baseCentrality(x, z)
  for (const l of plan.landmarks) {
    if (l.minListings !== undefined) continue
    if (Math.hypot(x - l.x, z - l.z) <= NEAR_LANDMARK) {
      c += 0.15
      break
    }
  }
  return clamp01(c)
}

export function heightScaleFor(centrality: number): 1 | 2 | 3 {
  if (centrality > 0.8) return 3
  if (centrality >= 0.5) return 2
  return 1
}

// -------------------------------------------------------------------- lots

const ROTATION: Record<CityEdge, number> = {
  n: Math.PI,
  s: 0,
  e: Math.PI / 2,
  w: -Math.PI / 2,
}

interface DraftLot extends Rect {
  rotation: number
  block: number
  edge: CityEdge
}

/**
 * Lots ring the block, one row per edge that fronts a street. The rows on the
 * long faces take the corners and the side rows are inset behind them, which
 * is how a real block packs and which keeps every rectangle axis-aligned.
 * Frontage and depth are exact — the slack goes to the ends of the row as side
 * alleys — so a lot near the plaza is a narrow shopfront and a lot at the rim
 * is a wide shed, as the plan calls for.
 */
function lotsForBlock(block: CityBlock): DraftLot[] {
  const t = block.centrality
  const frontage = r2(RIM_LOT.frontage + (CORE_LOT.frontage - RIM_LOT.frontage) * t)
  const depth = r2(RIM_LOT.depth + (CORE_LOT.depth - RIM_LOT.depth) * t)
  const x0 = block.x - block.w / 2
  const x1 = block.x + block.w / 2
  const z0 = block.z - block.d / 2
  const z1 = block.z + block.d / 2
  const has = (e: CityEdge) => block.edges.includes(e)
  const out: DraftLot[] = []

  // Frontage and depth are the pitch of the row; the lot itself is a hair
  // smaller, so neighbours have air between them and rounding the coordinates
  // to two decimals can never make two rectangles touch.
  const row = (edge: CityEdge, from: number, to: number, dp: number) => {
    const span = to - from
    const n = Math.floor(span / frontage)
    const len = r2(frontage - LOT_GAP)
    const dep = r2(dp - LOT_GAP)
    if (n < 1 || dep < MIN_WEDGE) return
    const slack = (span - n * frontage) / 2
    for (let i = 0; i < n; i++) {
      const c = r2(from + slack + frontage * (i + 0.5))
      if (edge === 'n') out.push({ x: c, z: r2(z0 + dp / 2), w: len, d: dep, rotation: ROTATION.n, block: block.id, edge })
      else if (edge === 's') out.push({ x: c, z: r2(z1 - dp / 2), w: len, d: dep, rotation: ROTATION.s, block: block.id, edge })
      else if (edge === 'w') out.push({ x: r2(x0 + dp / 2), z: c, w: dep, d: len, rotation: ROTATION.w, block: block.id, edge })
      else out.push({ x: r2(x1 - dp / 2), z: c, w: dep, d: len, rotation: ROTATION.e, block: block.id, edge })
    }
  }

  // Rows back onto each other, so a block too shallow for two full-depth rows
  // gets two short ones rather than one row and a strip of waste ground.
  const nsRows = (has('n') ? 1 : 0) + (has('s') ? 1 : 0)
  const ewRows = (has('e') ? 1 : 0) + (has('w') ? 1 : 0)
  const dpNS = Math.min(depth, block.d / Math.max(nsRows, 1))
  const dpEW = Math.min(depth, block.w / Math.max(ewRows, 1))
  // The block's long faces take the corners and the short faces sit inside
  // them, which is what gives the long street its unbroken parade of frontage.
  if (block.d > block.w) {
    if (has('w')) row('w', z0, z1, dpEW)
    if (has('e')) row('e', z0, z1, dpEW)
    const from = x0 + (has('w') ? dpEW : 0)
    const to = x1 - (has('e') ? dpEW : 0)
    if (has('n')) row('n', from, to, dpNS)
    if (has('s')) row('s', from, to, dpNS)
  } else {
    if (has('n')) row('n', x0, x1, dpNS)
    if (has('s')) row('s', x0, x1, dpNS)
    const from = z0 + (has('n') ? dpNS : 0)
    const to = z1 - (has('s') ? dpNS : 0)
    if (has('w')) row('w', from, to, dpEW)
    if (has('e')) row('e', from, to, dpEW)
  }
  return out
}

/** Midpoint of the edge a lot faces, which is where it meets its street. */
export function frontPoint(lot: Pick<CityLot, 'x' | 'z' | 'w' | 'd' | 'rotation'>): [number, number] {
  const s = Math.round(Math.sin(lot.rotation))
  const c = Math.round(Math.cos(lot.rotation))
  return [r2(lot.x + (s * lot.w) / 2), r2(lot.z + (c * lot.d) / 2)]
}

/**
 * The road a lot fronts: the one whose kerb — its centreline offset by half its
 * width — passes closest to the middle of the lot's front edge. `gap` is how
 * far off that kerb the lot sits, and is 0 for a lot flush against it.
 */
export function frontingRoad(roads: CityRoad[], lot: Pick<CityLot, 'x' | 'z' | 'w' | 'd' | 'rotation'>): { road: CityRoad; tier: CityTier; gap: number } {
  const [fx, fz] = frontPoint(lot)
  let best = roads[0]
  let gap = Infinity
  for (const road of roads) {
    const off = Math.abs(pointPolylineDist(fx, fz, road.points) - road.width / 2)
    if (off < gap) {
      gap = off
      best = road
    }
  }
  return { road: best, tier: best.tier, gap }
}

// ------------------------------------------------------------------ planning

/** Builds the city. Constant: two calls return structurally identical plans. */
export function planCity(): CityPlan {
  const roads = baseRoads()
  const diagonal = roads.find((r) => r.id === 'diagonal')!

  // 1. Streets and blocks, by recursive splitting of the four quadrants.
  const blocks: CityBlock[] = []
  const streetLines: Pt[][] = []
  const split = rng(subSeed('split'))
  const rects: { rect: Rect; edges: CityEdge[]; rim: boolean }[] = []
  for (const q of quadrantRects()) {
    const out: Rect[] = []
    splitQuadrant(q, split, streetLines, out)
    for (const rect of out) rects.push({ rect, edges: ALL_EDGES, rim: false })
  }
  for (const band of rimBands()) {
    for (const rect of chopBand(band.rect, band.along)) rects.push({ rect, edges: [band.edge], rim: true })
  }
  streetLines.forEach((points, i) => {
    roads.push({ id: `street-${i}`, kind: 'street', tier: 1, width: STREET_W, points, closed: false })
  })
  rects.forEach(({ rect, edges, rim }, id) => {
    blocks.push({ id, x: rect.x, z: rect.z, w: rect.w, d: rect.d, edges, rim, centrality: r2(baseCentrality(rect.x, rect.z)) })
  })

  // 2. Lots, minus the ones the plaza, the paths and the stubs pave over, and
  //    with the ones the diagonal clips shrunk back inside their block.
  const plazaKeepOut: Rect = { x: 0, z: 0, w: (PLAZA_R + PATH_W / 2) * 2, d: (PLAZA_R + PATH_W / 2) * 2 }
  const stubs = roads.filter((r) => r.kind === 'cul-de-sac')
  const drafts: (DraftLot & { wedge: boolean })[] = []
  for (const block of blocks) {
    for (const lot of lotsForBlock(block)) {
      if (rectsOverlap(lot, plazaKeepOut)) continue
      let blocked = false
      for (const stub of stubs) {
        const end = stub.points[stub.points.length - 1]
        if (polylineRectDist(stub.points, lot) < stub.width / 2 || pointRectDist(end[0], end[1], lot) < STUB_CIRCLE) {
          blocked = true
          break
        }
      }
      if (blocked) continue
      const clipped = clipToDiagonal(lot, diagonal)
      if (clipped) drafts.push(clipped)
    }
  }

  // 3. The three launch landmarks, settled before any lot is measured because
  //    they bias the centrality every lot is then measured with. A lot's id is
  //    its index in `drafts`, so a landmark can claim one here.
  const landmarks: CityLandmark[] = [
    { id: 'plaza', kind: 'plaza', name: 'Fountain Square', x: 0, z: 0, r: PLAZA_R },
  ]
  const parkIdx = pickBest(drafts, (d) => (d.wedge ? baseCentrality(d.x, d.z) : -Infinity))
  if (parkIdx >= 0) {
    landmarks.push({ id: 'park', kind: 'park', name: 'Wedge Park', x: drafts[parkIdx].x, z: drafts[parkIdx].z, r: 8, lot: parkIdx })
  }
  const corner = { x: BOUNDS.w / 2, z: -BOUNDS.d / 2 }
  const mastIdx = pickBest(drafts, (d) => (d === drafts[parkIdx] ? -Infinity : -Math.hypot(d.x - corner.x, d.z - corner.z)))
  if (mastIdx >= 0) {
    landmarks.push({ id: 'mast', kind: 'mast', name: 'Beacon Mast', x: drafts[mastIdx].x, z: drafts[mastIdx].z, r: 4, lot: mastIdx })
  }

  const plan: CityPlan = {
    v: 1,
    seed: fnv1a(CITY_SEED),
    bounds: { x: 0, z: 0, w: BOUNDS.w, d: BOUNDS.d },
    roads,
    blocks,
    lots: [],
    districts: DISTRICTS.map((d) => ({ ...d })),
    landmarks,
    props: [],
    loops: [],
  }

  // 4. Settle each lot: what it fronts, where it belongs, how tall it may be.
  plan.lots = drafts.map((draft, id) => {
    const centrality = r2(centralityAt(plan, draft.x, draft.z))
    return {
      id,
      x: draft.x,
      z: draft.z,
      w: draft.w,
      d: draft.d,
      rotation: r2(draft.rotation),
      block: draft.block,
      tier: frontingRoad(roads, { ...draft, rotation: draft.rotation }).tier,
      district: districtAt(draft.x, draft.z),
      centrality,
      heightScale: heightScaleFor(centrality),
      wedge: draft.wedge,
    }
  })

  // 5. The three milestone lots, held empty until the directory earns them.
  reserveMilestones(plan)

  // 6. Paths, props and the agents that run them.
  const parkLandmark = landmarks.find((l) => l.id === 'park')
  if (parkLandmark) {
    roads.push({
      id: 'park-path',
      kind: 'path',
      tier: 0,
      width: PATH_W,
      points: ringPoints(parkLandmark.x, parkLandmark.z, parkLandmark.r ?? 8, 8),
      closed: true,
    })
  }
  plan.props = planProps(plan)
  plan.loops = planLoops(plan)
  return plan
}

/** A closed regular polygon, used for the park path and its pedestrian loop. */
function ringPoints(cx: number, cz: number, r: number, n: number): Pt[] {
  const pts: Pt[] = []
  for (let i = 0; i < n; i++) {
    const a = (i / n) * Math.PI * 2
    pts.push([r2(cx + Math.cos(a) * r), r2(cz + Math.sin(a) * r)])
  }
  pts.push(pts[0])
  return pts
}

/**
 * A lot the diagonal crosses is pulled back along its street until it clears
 * the carriageway. True triangles would buy nothing a renderer can use, so the
 * lot stays a rectangle inside its block and is marked `wedge` instead; a lot
 * that cannot keep a usable frontage is dropped and the diagonal keeps the
 * ground. Wedges are where the landmarks go.
 */
function clipToDiagonal(lot: DraftLot, diagonal: CityRoad): (DraftLot & { wedge: boolean }) | null {
  const half = diagonal.width / 2
  if (polylineRectDist(diagonal.points, lot) >= half) return { ...lot, wedge: false }

  // The front edge stays on its kerb — a lot that leaves its street is not a
  // lot — so only the back edge and the two ends of the frontage move.
  const alongX = lot.edge === 'n' || lot.edge === 's'
  const back = lot.edge === 'n' || lot.edge === 'w' ? 1 : -1
  const frontC = alongX ? lot.z - (back * lot.d) / 2 : lot.x - (back * lot.w) / 2
  const frontage = alongX ? lot.w : lot.d
  const depth = alongX ? lot.d : lot.w

  let best: Rect | null = null
  let bestArea = 0
  for (const end of [-1, 1]) {
    const endC = alongX ? lot.x + (end * lot.w) / 2 : lot.z + (end * lot.d) / 2
    for (let f = frontage; f >= MIN_WEDGE; f -= 0.5) {
      for (let dp = depth; dp >= MIN_WEDGE; dp -= 0.5) {
        const along = endC - (end * f) / 2
        const across = frontC + (back * dp) / 2
        const rect: Rect = alongX ? { x: along, z: across, w: f, d: dp } : { x: across, z: along, w: dp, d: f }
        if (polylineRectDist(diagonal.points, rect) < half) continue
        if (f * dp > bestArea) {
          bestArea = f * dp
          best = rect
        }
        break
      }
    }
  }
  if (!best) return null
  return { ...lot, x: r2(best.x), z: r2(best.z), w: r2(best.w), d: r2(best.d), wedge: true }
}

/** Index of the highest-scoring item, or -1 when every score is -Infinity. */
function pickBest<T>(items: T[], score: (item: T) => number): number {
  let best = -1
  let bestScore = -Infinity
  for (let i = 0; i < items.length; i++) {
    const s = score(items[i])
    if (s > bestScore) {
      bestScore = s
      best = i
    }
  }
  return best
}

/**
 * Three lots the city keeps empty until it has earned them: the best address on
 * the plaza, a wedge on the diagonal, and the far rim. Drawn as empty lots
 * below their threshold, built at it — and never offered to a listing, so the
 * city cannot grow into its own monuments.
 */
function reserveMilestones(plan: CityPlan): void {
  const mast = plan.landmarks.find((l) => l.id === 'mast') ?? { x: BOUNDS.w / 2, z: -BOUNDS.d / 2 }
  const taken = new Set(plan.landmarks.map((l) => l.lot).filter((l): l is number => l !== undefined))
  const pick = (score: (lot: CityLot) => number): CityLot | undefined => {
    let best: CityLot | undefined
    let bestScore = -Infinity
    for (const lot of plan.lots) {
      if (taken.has(lot.id)) continue
      const s = score(lot)
      if (s > bestScore) {
        bestScore = s
        best = lot
      }
    }
    if (best) taken.add(best.id)
    return best
  }
  const hall = pick((l) => l.centrality)
  if (hall) plan.landmarks.push({ id: 'city-hall', kind: 'city-hall', name: 'City Hall', x: hall.x, z: hall.z, lot: hall.id, minListings: 25 })
  const clock = pick((l) => (l.wedge ? l.centrality : -Infinity))
  if (clock) plan.landmarks.push({ id: 'clock-tower', kind: 'clock-tower', name: 'Clock Tower', x: clock.x, z: clock.z, lot: clock.id, minListings: 100 })
  const view = pick((l) => -Math.hypot(l.x - mast.x, l.z - mast.z))
  if (view) plan.landmarks.push({ id: 'observation-tower', kind: 'observation-tower', name: 'Observation Tower', x: view.x, z: view.z, lot: view.id, minListings: 250 })
}

/** Lots a listing may never take: the ones the milestones are holding. */
export function reservedLots(plan: CityPlan): Set<number> {
  return new Set(plan.landmarks.map((l) => l.lot).filter((l): l is number => l !== undefined))
}

/** The milestone landmarks a directory of `count` listings has unlocked. */
export function activeMilestones(plan: CityPlan, count: number): CityLandmark[] {
  return plan.landmarks.filter((l) => l.minListings !== undefined && count >= l.minListings)
}

// ------------------------------------------------------------------- props

function planProps(plan: CityPlan): CityProp[] {
  const props: CityProp[] = []
  const push = (kind: CityProp['kind'], x: number, z: number, rotation = 0) => {
    props.push({ id: props.length, kind, x: r2(x), z: r2(z), rotation: r2(rotation), district: districtAt(x, z) })
  }
  const avenues = plan.roads.filter((r) => r.kind === 'avenue')

  // A bus shelter every fourth block that fronts an avenue, on the edge that
  // faces it, so the stops thin out as the streets get quieter.
  let onAvenue = 0
  for (const block of plan.blocks) {
    let best: { road: CityRoad; edge: CityEdge; gap: number } | null = null
    for (const edge of block.edges) {
      const px = edge === 'w' ? block.x - block.w / 2 : edge === 'e' ? block.x + block.w / 2 : block.x
      const pz = edge === 'n' ? block.z - block.d / 2 : edge === 's' ? block.z + block.d / 2 : block.z
      for (const road of avenues) {
        const gap = Math.abs(pointPolylineDist(px, pz, road.points) - road.width / 2)
        if (gap <= 1 && (!best || gap < best.gap)) best = { road, edge, gap }
      }
    }
    if (!best) continue
    if (onAvenue % 4 === 0) {
      const edge = best.edge
      const px = edge === 'w' ? block.x - block.w / 2 - 2 : edge === 'e' ? block.x + block.w / 2 + 2 : block.x
      const pz = edge === 'n' ? block.z - block.d / 2 - 2 : edge === 's' ? block.z + block.d / 2 + 2 : block.z
      push('bus-shelter', px, pz, ROTATION[edge])
    }
    onAvenue++
  }

  for (const lot of plan.lots) {
    if (lot.wedge) push('billboard', lot.x, lot.z, lot.rotation)
  }

  // Benches and trees ring the two places people stand still.
  for (const id of ['plaza', 'park'] as const) {
    const l = plan.landmarks.find((x) => x.id === id)
    if (!l) continue
    const r = (l.r ?? 8) - 2
    const benches = id === 'plaza' ? 8 : 6
    const trees = id === 'plaza' ? 12 : 10
    for (let i = 0; i < benches; i++) {
      const a = (i / benches) * Math.PI * 2
      push('bench', l.x + Math.cos(a) * r, l.z + Math.sin(a) * r, -a)
    }
    for (let i = 0; i < trees; i++) {
      const a = ((i + 0.5) / trees) * Math.PI * 2
      push('tree', l.x + Math.cos(a) * (r + 3), l.z + Math.sin(a) * (r + 3))
    }
  }
  const plaza = plan.landmarks.find((l) => l.id === 'plaza')
  if (plaza) push('fountain', plaza.x, plaza.z)
  const mast = plan.landmarks.find((l) => l.id === 'mast')
  if (mast) push('beacon', mast.x, mast.z)

  let tower: CityBlock | null = null
  for (const block of plan.blocks) {
    if (!block.rim) continue
    if (!tower || block.centrality > tower.centrality) tower = block
  }
  if (tower) push('water-tower', tower.x, tower.z)
  return props
}

// ------------------------------------------------------------------- loops

/** Agents run one lane in and one lane back, inside the carriageway. */
const LANE_OFFSET = 2

function planLoops(plan: CityPlan): CityLoop[] {
  const loops: CityLoop[] = []
  const road = (id: string) => plan.roads.find((r) => r.id === id)
  const add = (id: string, kind: CityLoop['kind'], roadId: string, count: number, points: Pt[]) => {
    const src = road(roadId)
    if (!src) return
    const next = rng(subSeed(`loop:${id}`))
    loops.push({
      id,
      kind,
      road: roadId,
      tier: src.tier,
      count,
      phases: Array.from({ length: count }, () => r2(next())),
      points,
    })
  }

  const ring = road('ring')
  if (ring) {
    const o = RING.x - LANE_OFFSET
    const p = RING.z - LANE_OFFSET
    add('cars-ring', 'car', 'ring', 12, [
      [-o, -p],
      [o, -p],
      [o, p],
      [-o, p],
      [-o, -p],
    ])
  }
  add('cars-avenue-ns', 'car', 'avenue-ns', 6, outAndBack([
    [0, -RING.z],
    [0, RING.z],
  ]))
  add('cars-avenue-ew', 'car', 'avenue-ew', 6, outAndBack([
    [-RING.x, 0],
    [RING.x, 0],
  ]))
  add('cars-diagonal', 'car', 'diagonal', 6, outAndBack(DIAGONAL_POINTS))

  const plaza = road('plaza-path')
  if (plaza) add('walk-plaza', 'pedestrian', 'plaza-path', 24, plaza.points.map((p) => [...p] as Pt))
  const park = road('park-path')
  if (park) add('walk-park', 'pedestrian', 'park-path', 16, park.points.map((p) => [...p] as Pt))
  return loops
}

/** Turns a two-way avenue into a closed loop: out on one lane, back on the other. */
function outAndBack(points: Pt[]): Pt[] {
  const out = offsetPolyline(points, LANE_OFFSET)
  const back = offsetPolyline(points, -LANE_OFFSET).reverse()
  return [...out, ...back, out[0]]
}

// -------------------------------------------------------------- assignment

export type CityListingInput = Pick<ListingMeta, 'slug' | 'category' | 'added' | 'tools'> & {
  /** The lot the listing's YAML already records, if it has one. */
  lot?: number
}

/**
 * Places listings on the plan. Two rules keep a building where people left it:
 * a listing whose YAML already carries a lot keeps it, whatever the probe would
 * say; and a listing without one probes from `fnv1a(slug)`, so its address
 * depends on its slug and on which lots are already taken, never on how many
 * listings happen to be in the directory. Listings are placed in `(added,
 * slug)` order, which is append-only, so a new listing cannot displace an old
 * one.
 */
export function assignLots(plan: CityPlan, listings: CityListingInput[]): CityAssignment[] {
  const ordered = [...listings].sort((a, b) => (a.added === b.added ? a.slug.localeCompare(b.slug) : a.added.localeCompare(b.added)))
  const taken = reservedLots(plan)
  const byId = new Map(plan.lots.map((l) => [l.id, l]))
  const placed = new Map<string, number>()

  for (const listing of ordered) {
    if (listing.lot === undefined) continue
    if (!byId.has(listing.lot) || taken.has(listing.lot)) continue
    taken.add(listing.lot)
    placed.set(listing.slug, listing.lot)
  }

  for (const listing of ordered) {
    if (placed.has(listing.slug)) continue
    const want = archetypeFor(listing.category)
    const start = fnv1a(listing.slug) % Math.max(plan.lots.length, 1)
    const lot = probe(plan, taken, start, want) ?? probe(plan, taken, start, null)
    if (lot === undefined) continue
    taken.add(lot)
    placed.set(listing.slug, lot)
  }

  let peak: string | null = null
  let peakTools = -1
  for (const listing of ordered) {
    if (!placed.has(listing.slug)) continue
    const n = listing.tools.length
    if (n > peakTools) {
      peakTools = n
      peak = listing.slug
    }
  }

  return ordered
    .filter((l) => placed.has(l.slug))
    .map((l) => ({ slug: l.slug, lot: placed.get(l.slug)!, peak: l.slug === peak }))
}

/** First free lot from `start`, wrapping once; `district` narrows the search. */
function probe(plan: CityPlan, taken: Set<number>, start: number, district: Archetype | null): number | undefined {
  const n = plan.lots.length
  for (let i = 0; i < n; i++) {
    const lot = plan.lots[(start + i) % n]
    if (taken.has(lot.id)) continue
    if (district !== null && lot.district !== district) continue
    return lot.id
  }
  return undefined
}

// ------------------------------------------------------------------ filler

/** A building's 32-bit window-light mask, from its lot alone. */
export function windowMask(lotId: number): number {
  return fnv1a(`window:${lotId}`)
}

/**
 * What every lot with no listing on it draws. A good address gets a filler
 * massing so the street has two sides; a poor one gets dirt and a fence, so it
 * is obvious the city has room to grow. A milestone lot the directory has not
 * earned yet is empty; once earned it is a landmark and not a fill at all.
 */
export function fillerFor(plan: CityPlan, assignments: CityAssignment[]): CityFill[] {
  const used = new Set(assignments.map((a) => a.lot))
  const built = new Set(
    plan.landmarks
      .filter((l) => l.minListings === undefined || assignments.length >= l.minListings)
      .map((l) => l.lot)
  )
  const waiting = new Set(plan.landmarks.filter((l) => l.minListings !== undefined).map((l) => l.lot))
  const fills: CityFill[] = []
  for (const lot of plan.lots) {
    if (used.has(lot.id) || built.has(lot.id)) continue
    // A milestone lot is held empty however good its address, because the
    // point of it is that the gap in the street is visible.
    if (waiting.has(lot.id) || lot.centrality <= FILLER_CENTRALITY) {
      fills.push({ lot: lot.id, kind: 'empty', storeys: 0, mask: 0 })
      continue
    }
    const storeys = (1 + Math.floor(rng(fnv1a(`filler:${lot.id}`))() * 4)) * lot.heightScale
    fills.push({ lot: lot.id, kind: 'filler', storeys, mask: windowMask(lot.id) })
  }
  return fills
}
