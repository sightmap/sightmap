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
import { CITY_STOREYS } from '../../src/types/city'
import { FLOOR_D, FLOOR_W } from '../../src/components/building/model'
import { archetypeFor } from './blueprint'

/** The plan never changes unless this string does. */
export const CITY_SEED = 'sightmap-atlas-city'

// ---------------------------------------------------------------- dimensions

/**
 * The plan was drawn on a 320 × 240 ground and is built larger. Lots, blocks
 * and roads keep their drawn size — a shopfront is a shopfront, and a street
 * is as wide as two cars — so the only thing that buys the city more addresses
 * is more ground, and it needs them: at the drawn size it holds about 165
 * lots, fewer than the 250 listings the observation tower is reserved for.
 * Everything with a position scales through `s`, so the proportions the plan
 * was drawn in survive.
 */
const SCALE = 1.4
/** A drawn coordinate, on the ground as built. */
const s = (n: number): number => Math.round(n * SCALE * 100) / 100

const BOUNDS = { w: s(320), d: s(240) }
/** Half-diagonal of the bounds: the distance at which centrality reaches 0. */
const HALF_DIAGONAL = Math.hypot(BOUNDS.w / 2, BOUNDS.d / 2)

// Road widths are traffic, not ground, so they are drawn and built alike.
const AVENUE_W = 8
const STREET_W = 5
const STUB_W = 4
const PATH_W = 4

/** The avenue ring, as the half-extents of its centreline rectangle. */
const RING = { x: s(140), z: s(100) }
/** Half-side of the paved square where the two cross avenues meet. */
const PLAZA_R = s(14)
/**
 * Half-side of the avenue ring around that square. Traffic runs round the
 * plaza rather than across it — a car that drives through the fountain reads
 * as a bug long before anyone asks whether the layout is plausible.
 */
const PLAZA_RING_R = PLAZA_R + AVENUE_W / 2

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
/**
 * What a lot needs to hold a building: the floor plate the building page draws
 * (FLOOR_W x FLOOR_D) with a metre of air round it. Anything smaller is a
 * pocket park — a building on it would spill over its own kerb.
 */
const BUILDABLE = { frontage: FLOOR_W + 1, depth: FLOOR_D + 1 }
/** Air between neighbouring lots, so rounded rectangles never touch. */
const LOT_GAP = 0.1

/**
 * Cul-de-sac stubs. A radial neck off the ring reaches the middle of the rim
 * band and then turns to run along it, which is the only shape that fits a
 * close of houses into a band this shallow: a purely radial stub would end in
 * its own turning circle with no room for a single lot beside it.
 */
const STUB_CIRCLE = s(6)
/** Depth of the lots that line a cul-de-sac leg, either side of it. */
const STUB_LOT_DEPTH = 10
/**
 * The neck reaches exactly the depth at which a row of lots fits either side
 * of the leg: the ring's kerb on one side, the edge of the ground on the
 * other. The rim band is twice that depth plus the carriageway, so the two
 * numbers are the same number and are written that way.
 */
const STUB_NECK = AVENUE_W / 2 + STUB_LOT_DEPTH + STUB_W / 2
/** Long enough for three lots a side, which is what makes a close a close. */
const STUB_LEG = 66

/**
 * Where filler gives way to empty lots. Between these two the odds ease, so
 * the built-up part of the city feathers into the rim instead of ending at a
 * line, and a few gap sites survive downtown.
 */
const FILL_FROM = 0.2
const FILL_TO = 0.75

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
 * there before the grid rather than as a construction line. It stops where it
 * meets the plaza ring instead of running into the square.
 */
const DIAGONAL_KINK: Pt = [s(-60), s(44)]
const DIAGONAL_POINTS: Pt[] = [[-RING.x, RING.z], DIAGONAL_KINK, meetsPlazaRing(DIAGONAL_KINK)]

/** Where a line drawn from `from` to the plaza crosses the plaza ring. */
function meetsPlazaRing(from: Pt): Pt {
  const t = PLAZA_RING_R / Math.max(Math.abs(from[0]), Math.abs(from[1]))
  return [r2(from[0] * t), r2(from[1] * t)]
}

/**
 * Where the six cul-de-sacs leave the ring, which way the neck points out of
 * it, and which way the leg then runs along the rim band.
 */
const STUBS: { at: Pt; dir: Pt; along: Pt }[] = [
  { at: [s(-80), -RING.z], dir: [0, -1], along: [1, 0] },
  { at: [s(40), -RING.z], dir: [0, -1], along: [-1, 0] },
  { at: [s(-40), RING.z], dir: [0, 1], along: [1, 0] },
  { at: [s(90), RING.z], dir: [0, 1], along: [-1, 0] },
  { at: [-RING.x, s(-40)], dir: [-1, 0], along: [0, 1] },
  { at: [RING.x, s(55)], dir: [1, 0], along: [0, -1] },
]

/** The neck corner and the far end of one cul-de-sac's leg. */
function stubPoints(stub: { at: Pt; dir: Pt; along: Pt }): [Pt, Pt, Pt] {
  const knee: Pt = [r2(stub.at[0] + stub.dir[0] * STUB_NECK), r2(stub.at[1] + stub.dir[1] * STUB_NECK)]
  const end: Pt = [r2(knee[0] + stub.along[0] * STUB_LEG), r2(knee[1] + stub.along[1] * STUB_LEG)]
  return [stub.at, knee, end]
}

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
  roads.push({
    id: 'plaza-ring',
    kind: 'avenue',
    tier: 2,
    width: AVENUE_W,
    points: squarePoints(PLAZA_RING_R),
    closed: true,
  })
  STUBS.forEach((stub, i) => {
    roads.push({
      id: `stub-${i}`,
      kind: 'cul-de-sac',
      tier: 0,
      width: STUB_W,
      points: stubPoints(stub),
      closed: false,
      turningCircle: STUB_CIRCLE,
    })
  })
  roads.push({
    id: 'plaza-path',
    kind: 'path',
    tier: 0,
    width: PATH_W,
    points: squarePoints(PLAZA_R - PATH_W / 2),
    closed: true,
  })
  return roads
}

/**
 * The close of houses each cul-de-sac exists to serve: one block per leg, with
 * a row of lots either side of it fronting the stub. Without them a stub is a
 * road driven into a field, which is what the rim looked like.
 */
function culDeSacs(nextBlockId: number): { blocks: CityBlock[]; lots: (DraftLot & { wedge: boolean })[] } {
  const blocks: CityBlock[] = []
  const lots: (DraftLot & { wedge: boolean })[] = []
  const frontage = RIM_LOT.frontage
  const across = STUB_W + STUB_LOT_DEPTH * 2

  STUBS.forEach((stub, i) => {
    const [, knee] = stubPoints(stub)
    const alongX = stub.along[0] !== 0
    const from = alongX ? knee[0] : knee[1]
    const step = alongX ? stub.along[0] : stub.along[1]
    const fixed = alongX ? knee[1] : knee[0]
    const mid = from + (step * STUB_LEG) / 2

    const id = nextBlockId + i
    blocks.push({
      id,
      x: r2(alongX ? mid : fixed),
      z: r2(alongX ? fixed : mid),
      w: r2(alongX ? STUB_LEG : across),
      d: r2(alongX ? across : STUB_LEG),
      // Its lots front the stub down the middle, not the block's own edges.
      edges: [],
      rim: true,
      centrality: r2(baseCentrality(alongX ? mid : fixed, alongX ? fixed : mid)),
    })

    // Clear of the neck at one end and the turning circle at the other.
    const usable = STUB_LEG - STUB_W / 2 - STUB_CIRCLE
    const n = Math.floor(usable / frontage)
    const slack = (usable - n * frontage) / 2
    const len = r2(frontage - LOT_GAP)
    const dep = r2(STUB_LOT_DEPTH - LOT_GAP)
    for (let side = -1; side <= 1; side += 2) {
      // `side` is which flank of the leg; the lot faces back across it.
      const edge: CityEdge = alongX ? (side > 0 ? 'n' : 's') : side > 0 ? 'w' : 'e'
      const acrossC = fixed + side * (STUB_W / 2 + STUB_LOT_DEPTH / 2)
      for (let k = 0; k < n; k++) {
        const alongC = from + step * (STUB_W / 2 + slack + frontage * (k + 0.5))
        lots.push({
          x: r2(alongX ? alongC : acrossC),
          z: r2(alongX ? acrossC : alongC),
          w: alongX ? len : dep,
          d: alongX ? dep : len,
          rotation: ROTATION[edge],
          block: id,
          edge,
          wedge: false,
        })
      }
    }
  })
  return { blocks, lots }
}

/** A closed axis-aligned square of half-side `r`, clockwise from its NW corner. */
function squarePoints(r: number): Pt[] {
  return [
    [-r, -r],
    [r, -r],
    [r, r],
    [-r, r],
    [-r, -r],
  ]
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
  { archetype: 'bank', name: 'Exchange', x: s(12), z: s(-12), accent: 0 },
  { archetype: 'storefront', name: 'Market Row', x: s(-56), z: s(8), accent: 1 },
  { archetype: 'theatre', name: 'The Strand', x: s(10), z: s(56), accent: 2 },
  { archetype: 'workshop', name: 'Foundry', x: s(-78), z: s(58), accent: 3 },
  { archetype: 'terminal', name: 'Eastgate', x: s(124), z: s(40), accent: 4 },
  { archetype: 'clinic', name: 'Northfield', x: s(-104), z: s(-64), accent: 5 },
  { archetype: 'library', name: 'Scholars', x: s(86), z: s(-70), accent: 6 },
  { archetype: 'office', name: 'Midtown', x: s(56), z: s(18), accent: 7 },
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

/**
 * A lot's frontage and depth in its own frame, whichever way round it faces.
 */
export function orientedSize(lot: Pick<CityLot, 'w' | 'd' | 'rotation'>): { frontage: number; depth: number } {
  const sideways = Math.abs(Math.round(Math.sin(lot.rotation))) === 1
  return sideways ? { frontage: lot.d, depth: lot.w } : { frontage: lot.w, depth: lot.d }
}

/** True when no building fits: the lot is drawn as a pocket park instead. */
function isTiny(lot: Pick<CityLot, 'w' | 'd' | 'rotation'>): boolean {
  const { frontage, depth } = orientedSize(lot)
  return frontage < BUILDABLE.frontage || depth < BUILDABLE.depth
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

  // 2. Lots, minus the ones the plaza, the paths and the cul-de-sacs pave
  //    over, and with the ones the diagonal clips shrunk back inside their
  //    block. The cul-de-sacs then line their own legs with the close of
  //    houses that is the whole point of driving a road out there.
  const keepOut = PLAZA_RING_R + AVENUE_W / 2
  const plazaKeepOut: Rect = { x: 0, z: 0, w: keepOut * 2, d: keepOut * 2 }
  const stubs = roads.filter((r) => r.kind === 'cul-de-sac')
  const paved = (lot: Rect): boolean => {
    if (rectsOverlap(lot, plazaKeepOut)) return true
    for (const stub of stubs) {
      const end = stub.points[stub.points.length - 1]
      if (polylineRectDist(stub.points, lot) < stub.width / 2) return true
      if (pointRectDist(end[0], end[1], lot) < STUB_CIRCLE) return true
    }
    return false
  }
  const drafts: (DraftLot & { wedge: boolean })[] = []
  for (const block of blocks) {
    for (const lot of lotsForBlock(block)) {
      if (paved(lot)) continue
      const clipped = clipToDiagonal(lot, diagonal)
      if (clipped) drafts.push(clipped)
    }
  }
  const closes = culDeSacs(blocks.length)
  blocks.push(...closes.blocks)
  for (const lot of closes.lots) {
    if (!drafts.some((d) => rectsOverlap(d, lot))) drafts.push(lot)
  }

  // 3. The launch landmarks, settled before any lot is measured because they
  //    bias the centrality every lot is then measured with. Each stands on a
  //    lot of its own, so nothing is ever built through one; a lot's id is its
  //    index in `drafts`, so a landmark can claim one here.
  const landmarks: CityLandmark[] = [
    { id: 'plaza', kind: 'plaza', name: 'Fountain Square', x: 0, z: 0, r: PLAZA_R },
  ]
  const claimed = new Set<number>()
  const claim = (score: (lot: DraftLot & { wedge: boolean }) => number): number => {
    const i = pickBest(drafts, (d, j) => (claimed.has(j) || isTiny(d) ? -Infinity : score(d)))
    if (i >= 0) claimed.add(i)
    return i
  }
  const site = (id: string, kind: CityLandmark['kind'], name: string, i: number, r: number) => {
    if (i >= 0) landmarks.push({ id, kind, name, x: drafts[i].x, z: drafts[i].z, r, lot: i })
  }
  site('park', 'park', 'Wedge Park', claim((d) => (d.wedge ? baseCentrality(d.x, d.z) : -Infinity)), 8)
  const corner = { x: BOUNDS.w / 2, z: -BOUNDS.d / 2 }
  site('mast', 'mast', 'Beacon Mast', claim((d) => -Math.hypot(d.x - corner.x, d.z - corner.z)), 4)
  // The water tower needs a lot of its own too: it used to be dropped at a rim
  // block's centre, which put it straddling whatever was built either side.
  const rimBlocks = new Set(blocks.filter((b) => b.rim).map((b) => b.id))
  site('water-tower', 'water-tower', 'Water Tower', claim((d) => (rimBlocks.has(d.block) ? baseCentrality(d.x, d.z) : -Infinity)), 5)

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
      tiny: isTiny(draft),
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
function pickBest<T>(items: T[], score: (item: T, index: number) => number): number {
  let best = -1
  let bestScore = -Infinity
  for (let i = 0; i < items.length; i++) {
    const s = score(items[i], i)
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
      if (taken.has(lot.id) || lot.tiny) continue
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
  const push = (kind: CityProp['kind'], x: number, z: number, rotation = 0, lot?: number) => {
    props.push({ id: props.length, kind, x: r2(x), z: r2(z), rotation: r2(rotation), district: districtAt(x, z), lot })
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

  // A billboard stands on the kerb in front of a wedge, facing the traffic the
  // diagonal brings past it — not in the middle of the lot, where it used to
  // sit inside the park and on top of the clock tower.
  const spoken = new Set(plan.landmarks.map((l) => l.lot))
  for (const lot of plan.lots) {
    if (!lot.wedge || lot.tiny || spoken.has(lot.id)) continue
    const [fx, fz] = frontPoint(lot)
    push('billboard', fx + Math.sin(lot.rotation), fz + Math.cos(lot.rotation), lot.rotation, lot.id)
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

  const tower = plan.landmarks.find((l) => l.id === 'water-tower')
  if (tower) push('water-tower', tower.x, tower.z, 0, tower.lot)
  return props
}

/**
 * The plan's props minus the ones standing in front of a lot a listing has
 * taken: a billboard belongs to an empty wedge, not to somebody's front door.
 */
export function propsFor(plan: CityPlan, assignments: CityAssignment[]): CityProp[] {
  const used = new Set(assignments.map((a) => a.lot))
  return plan.props.filter((p) => p.lot === undefined || !used.has(p.lot))
}

// ------------------------------------------------------------------- loops

/** Agents run one lane in and one lane back, inside the carriageway. */
const LANE_OFFSET = 2

function planLoops(plan: CityPlan): CityLoop[] {
  const loops: CityLoop[] = []
  const road = (id: string) => plan.roads.find((r) => r.id === id)
  const add = (id: string, kind: CityLoop['kind'], on: string[], count: number, points: Pt[]) => {
    const src = road(on[0])
    if (!src) return
    const next = rng(subSeed(`loop:${id}`))
    loops.push({
      id,
      kind,
      road: on[0],
      roads: on,
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
    add('cars-ring', 'car', ['ring'], 12, [
      [-o, -p],
      [o, -p],
      [o, p],
      [-o, p],
      [-o, -p],
    ])
  }
  // The cross avenues run out to the plaza ring, round it, and back out the
  // far side: the square is a place, not a junction, and the fountain is not a
  // roundabout.
  const p = PLAZA_RING_R
  const o = LANE_OFFSET
  add('cars-avenue-ns', 'car', ['avenue-ns', 'plaza-ring'], 6, [
    [o, -RING.z],
    [o, -p],
    [p, -p],
    [p, p],
    [o, p],
    [o, RING.z],
    [-o, RING.z],
    [-o, p],
    [-p, p],
    [-p, -p],
    [-o, -p],
    [-o, -RING.z],
    [o, -RING.z],
  ])
  add('cars-avenue-ew', 'car', ['avenue-ew', 'plaza-ring'], 6, [
    [-RING.x, o],
    [-p, o],
    [-p, p],
    [p, p],
    [p, o],
    [RING.x, o],
    [RING.x, -o],
    [p, -o],
    [p, -p],
    [-p, -p],
    [-p, -o],
    [-RING.x, -o],
    [-RING.x, o],
  ])
  add('cars-diagonal', 'car', ['diagonal'], 6, outAndBack(DIAGONAL_POINTS))

  const plaza = road('plaza-path')
  if (plaza) add('walk-plaza', 'pedestrian', ['plaza-path'], 24, plaza.points.map((q) => [...q] as Pt))
  const park = road('park-path')
  if (park) add('walk-park', 'pedestrian', ['park-path'], 16, park.points.map((q) => [...q] as Pt))
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
  /** Floors on the listing's blueprint; the build reads it from there. */
  floors?: number
}

/**
 * The lots a district offers, best address first. The probe walks this rather
 * than the plan's own order, which is the order the ground was surveyed in and
 * puts the rim band next to downtown: eight listings used to scatter, three of
 * them landing on the far side of the ring from their own district.
 */
function addressBook(plan: CityPlan): Map<Archetype | null, CityLot[]> {
  const book = new Map<Archetype | null, CityLot[]>()
  const rank = (a: CityLot, b: CityLot) => b.centrality - a.centrality || a.id - b.id
  const open = plan.lots.filter((l) => !l.tiny)
  book.set(null, [...open].sort(rank))
  for (const district of plan.districts) {
    book.set(district.archetype, open.filter((l) => l.district === district.archetype).sort(rank))
  }
  return book
}

/** How far down the best addresses a slug may start looking. */
const PROBE_WINDOW = 8

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
  const open = new Set(plan.lots.filter((l) => !l.tiny).map((l) => l.id))
  const placed = new Map<string, number>()

  for (const listing of ordered) {
    if (listing.lot === undefined) continue
    if (!open.has(listing.lot) || taken.has(listing.lot)) continue
    taken.add(listing.lot)
    placed.set(listing.slug, listing.lot)
  }

  const book = addressBook(plan)
  for (const listing of ordered) {
    if (placed.has(listing.slug)) continue
    const want = archetypeFor(listing.category)
    const seed = fnv1a(listing.slug)
    const lot = probe(book.get(want) ?? [], taken, seed) ?? probe(book.get(null) ?? [], taken, seed)
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

  const byId = new Map(plan.lots.map((l) => [l.id, l]))
  const settled = ordered
    .filter((l) => placed.has(l.slug))
    .map((l) => {
      const lot = placed.get(l.slug)!
      return { slug: l.slug, lot, peak: l.slug === peak, storeys: storeysFor(byId.get(lot)!, l.floors, false) }
    })
  // The peak is the high point of the whole skyline, not just half again its
  // own height: a one-floor site on a tall lot must not out-top it.
  const tallest = settled.reduce((m, a) => Math.max(m, a.storeys), 0)
  for (const a of settled) {
    if (a.peak) a.storeys = Math.max(a.storeys, Math.round(tallest * CITY_STOREYS.peak))
  }
  return settled
}

/**
 * Storeys a listing draws on its lot: its real floors, never fewer than two so
 * a one-page site is still a building, raised by the lot's height scale, and
 * half again for the one peak.
 */
export function storeysFor(lot: CityLot, floors: number | undefined, peak: boolean): number {
  const base = Math.max(floors ?? 0, CITY_STOREYS.minFloors) * lot.heightScale
  return peak ? Math.round(base * CITY_STOREYS.peak) : base
}

/**
 * First free lot in an address book, starting somewhere in the best few so two
 * slugs do not both want the same front door, then walking down the list.
 */
function probe(lots: CityLot[], taken: Set<number>, seed: number): number | undefined {
  if (lots.length === 0) return undefined
  const start = seed % Math.min(PROBE_WINDOW, lots.length)
  for (let i = 0; i < lots.length; i++) {
    const lot = lots[(start + i) % lots.length]
    if (!taken.has(lot.id)) return lot.id
  }
  return undefined
}

// ------------------------------------------------------------------ filler

/** A building's 32-bit window-light mask, from its lot alone. */
export function windowMask(lotId: number): number {
  return fnv1a(`window:${lotId}`)
}

/** Hermite ease between two edges; 0 below `e0`, 1 above `e1`. */
function smoothstep(e0: number, e1: number, x: number): number {
  const t = clamp01((x - e0) / (e1 - e0))
  return t * t * (3 - 2 * t)
}

/** How likely an unclaimed lot is to have been built on already. */
export function fillChance(centrality: number): number {
  return smoothstep(FILL_FROM, FILL_TO, centrality)
}

/** Storeys a filler massing draws on a lot, before the block's cap. */
export function fillStoreys(lotId: number, heightScale: number): number {
  const span = CITY_STOREYS.fillMax - CITY_STOREYS.fillMin + 1
  const n = CITY_STOREYS.fillMin + Math.floor(rng(fnv1a(`filler:${lotId}`))() * span)
  return n * heightScale
}

/**
 * What every lot with no listing on it draws. A good address is likely to be
 * built on already so the street has two sides; a poor one is more likely to
 * be dirt and a fence, so the room the city has to grow is visible. The odds
 * ease between the two rather than switching at a threshold, which drew the
 * built-up part as a disc with a razor edge and no gap sites downtown at all.
 *
 * Filler never out-tops its own street by more than a storey, and a milestone
 * lot is held empty however good its address: the point of it is the gap.
 */
export function fillerFor(plan: CityPlan, assignments: CityAssignment[]): CityFill[] {
  const used = new Set(assignments.map((a) => a.lot))
  const built = new Set(
    plan.landmarks
      .filter((l) => l.minListings === undefined || assignments.length >= l.minListings)
      .map((l) => l.lot)
  )
  const waiting = new Set(plan.landmarks.filter((l) => l.minListings !== undefined).map((l) => l.lot))

  // Tallest listing on each block, so filler can be kept in its place.
  const byId = new Map(plan.lots.map((l) => [l.id, l]))
  const tallest = new Map<number, number>()
  for (const a of assignments) {
    const lot = byId.get(a.lot)
    if (!lot) continue
    tallest.set(lot.block, Math.max(tallest.get(lot.block) ?? 0, a.storeys))
  }

  const fills: CityFill[] = []
  for (const lot of plan.lots) {
    if (lot.tiny || used.has(lot.id) || built.has(lot.id)) continue
    const chance = waiting.has(lot.id) ? 0 : fillChance(lot.centrality)
    if (rng(fnv1a(`fill:${lot.id}`))() >= chance) {
      fills.push({ lot: lot.id, kind: 'empty', storeys: 0, mask: 0 })
      continue
    }
    const cap = tallest.get(lot.block)
    const storeys = Math.max(1, Math.min(fillStoreys(lot.id, lot.heightScale), cap === undefined ? Infinity : cap + CITY_STOREYS.fillHeadroom))
    fills.push({ lot: lot.id, kind: 'filler', storeys, mask: windowMask(lot.id) })
  }
  return fills
}
