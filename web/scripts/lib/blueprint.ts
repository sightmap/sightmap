// Derives a listing's building from its scan and review. Pure and total: the
// same listing always produces the same blueprint, and no listing — however
// odd its scan — can fail the build.
//
// The metaphor is the one src/components/building draws: a page is a floor, a
// tool is a room, a journey is a walk. The geometry constants come from that
// model so a derived floor and a hand-authored one are the same rectangle,
// with the same service core and the same front aisle.
//
// Nothing here is a judgement about the site. A room exists because a scan saw
// a tool register on a page, and its size is that tool's declared parameter
// count — no more.
import { AISLE_Z, CORE, CORE_DOOR, FLOOR_D, FLOOR_W, KIOSK_H } from '../../src/components/building/model'
import type {
  Archetype,
  Blueprint,
  BlueprintFacade,
  BlueprintFloor,
  BlueprintKind,
  BlueprintRoom,
  BlueprintWalk,
  RoofStyle,
} from '../../src/types/blueprint'
import type { DirectoryListing, ScanPage, ScanTool, ToolKind } from '../../src/types/directory'

// ---------------------------------------------------------------- seeding

/** 32-bit FNV-1a. The slug is the only input, so the building never moves. */
export function fnv1a(input: string, basis = 0x811c9dc5): number {
  let h = basis >>> 0
  for (let i = 0; i < input.length; i++) {
    h ^= input.charCodeAt(i) & 0xff
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return h >>> 0
}

/**
 * A sub-seed for one labelled choice. Every seeded decision draws from its own
 * stream, so adding a rule later cannot reshuffle the choices made before it.
 */
export function subSeed(seed: number, label: string): number {
  return fnv1a(label, seed)
}

/** mulberry32: small, fast, and stable across engines. */
export function mulberry32(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** One seeded choice out of `n`, drawn from the `label` stream. */
export function pick(seed: number, label: string, n: number): number {
  return Math.min(n - 1, Math.floor(mulberry32(subSeed(seed, label))() * n))
}

// ---------------------------------------------------------------- facade

/**
 * Category → persona. Ordered, and matched on the whole category id: an id
 * that is not on the list is an office, which is the honest default for a site
 * whose business the directory has not classified.
 */
export const ARCHETYPE_CATEGORIES: [Archetype, string[]][] = [
  ['storefront', ['commerce', 'shopping', 'marketplace', 'retail']],
  ['bank', ['finance', 'fintech', 'crypto', 'payments']],
  ['workshop', ['devtools', 'developer', 'infrastructure', 'api', 'tooling']],
  ['theatre', ['media', 'entertainment', 'video', 'music', 'news']],
  ['terminal', ['travel', 'logistics', 'transport', 'maps']],
  ['clinic', ['health', 'medical', 'wellness']],
  ['library', ['data', 'reference', 'productivity', 'education', 'research']],
]

export function archetypeFor(category: string): Archetype {
  const id = category.trim().toLowerCase()
  for (const [archetype, ids] of ARCHETYPE_CATEGORIES) if (ids.includes(id)) return archetype
  return 'office'
}

/** The three roofs each persona wears, indexed by variant. */
export const ROOFS: Record<Archetype, [RoofStyle, RoofStyle, RoofStyle]> = {
  storefront: ['parapet', 'flat', 'gable'],
  bank: ['dome', 'hip', 'parapet'],
  workshop: ['sawtooth', 'flat', 'gable'],
  theatre: ['mansard', 'parapet', 'dome'],
  terminal: ['flat', 'hip', 'sawtooth'],
  clinic: ['flat', 'hip', 'parapet'],
  library: ['hip', 'mansard', 'dome'],
  office: ['flat', 'parapet', 'hip'],
}

export const VARIANTS = 3
export const PALETTES = 4

export function facadeFor(listing: DirectoryListing, seed: number): BlueprintFacade {
  const archetype = archetypeFor(listing.category)
  const variant = pick(seed, 'variant', VARIANTS)
  return {
    archetype,
    variant,
    roof: ROOFS[archetype][variant],
    palette: pick(seed, 'palette', PALETTES),
    sign: listing.name,
    sightkick: listing.built_with_sightkick,
  }
}

// ---------------------------------------------------------------- rooms

/** How many parameters a tool declares. A schema that is not an object declares none. */
export function paramCount(inputSchema: unknown): number {
  if (!inputSchema || typeof inputSchema !== 'object') return 0
  const props = (inputSchema as { properties?: unknown }).properties
  if (!props || typeof props !== 'object') return 0
  return Object.keys(props as Record<string, unknown>).length
}

/**
 * A tool's room kind. `read` splits by how much the tool asks for: a schema
 * with two or more fields is a form, one with none is a doorway, anything
 * between is a page of content.
 */
export function roomKind(kind: ToolKind, params: number): BlueprintKind {
  if (kind === 'action') return 'action'
  if (kind === 'sensitive') return 'data'
  if (params >= 2) return 'form'
  if (params === 0) return 'nav'
  return 'content'
}

/** Room height by kind, matching what Tower.tsx draws: a kiosk for an action. */
export const KIND_H: Record<BlueprintKind, number> = {
  nav: 0.45,
  form: 0.75,
  content: 0.5,
  data: 0.65,
  action: KIOSK_H,
}

/** Parameters past the fourth stop growing the room; a floor has to stay walkable. */
const PARAM_CAP = 4

export function roomSize(kind: BlueprintKind, params: number): { w: number; d: number; h: number } {
  const n = Math.min(params, PARAM_CAP)
  return { w: 1.6 + 0.35 * n, d: 1.2 + 0.25 * n, h: KIND_H[kind] }
}

// ---------------------------------------------------------------- packing

/** Clearance between rooms, and between a room and anything reserved. */
export const GAP = 0.3
/** Free rectangle a floor packs into: the slab, less a walkable margin. */
export const X_MIN = -FLOOR_W / 2 + GAP
export const X_MAX = FLOOR_W / 2 - GAP
export const Z_BACK = -FLOOR_D / 2 + GAP
/** Rooms stop here; AISLE_Z and everything in front of it is the front aisle. */
export const Z_FRONT = AISLE_Z - GAP
/** Where the lateral lanes and the aisle end, matching the building page's lanes. */
export const LANE_X = FLOOR_W / 2 - 0.6

/**
 * The trunk leaves the core door heading for the aisle. The core fills the
 * back-left corner, so the trunk runs straight out of the door until it clears
 * the core, turns west along a row gap, and then follows the left edge. It
 * costs one column of the rows behind the core and a strip of the left edge in
 * the rows in front of it, rather than cutting every row in half.
 */
const TRUNK_X = CORE_DOOR.x
const TRUNK_LEFT_X = -LANE_X

interface Band {
  x0: number
  x1: number
}

/** The service core, as a keep-out with the room clearance already applied. */
const CORE_KEEPOUT = {
  x0: CORE.x - CORE.w / 2 - GAP,
  x1: CORE.x + CORE.w / 2 + GAP,
  z0: CORE.z - CORE.d / 2 - GAP,
  z1: CORE.z + CORE.d / 2 + GAP,
}

/** A row starting at or past this is clear of the core, so the trunk can turn. */
const CORE_FRONT = CORE_KEEPOUT.z1

const TRUNK_STUB: Band = { x0: TRUNK_X - GAP, x1: TRUNK_X + GAP }
const TRUNK_EDGE: Band = { x0: X_MIN - GAP, x1: TRUNK_LEFT_X + GAP }

export interface PackItem {
  name: string
  kind: BlueprintKind
  params: number
  w: number
  d: number
  h: number
}

export interface PackedFloor {
  rooms: BlueprintRoom[]
  /** Centre z of each gap between two rows, back to front. */
  gaps: number[]
  /** Index into `gaps` of the gap the trunk turns along, or -1 if it runs straight. */
  turn: number
  /** 1 unless the rooms had to be shrunk to fit the slab. */
  scale: number
  /** Rooms that did not fit even at the smallest scale. */
  dropped: string[]
}

const round = (n: number): number => Math.round(n * 1000) / 1000

/** Free x spans in one row band: the slab less the core and the trunk. */
export function freeSpans(z0: number, z1: number): [number, number][] {
  const blocked: Band[] = [z0 < CORE_FRONT ? TRUNK_STUB : TRUNK_EDGE]
  if (CORE_KEEPOUT.z0 < z1 && CORE_KEEPOUT.z1 > z0) blocked.push(CORE_KEEPOUT)

  let spans: [number, number][] = [[X_MIN, X_MAX]]
  for (const b of blocked) {
    const next: [number, number][] = []
    for (const [a, z] of spans) {
      if (b.x0 > a) next.push([a, Math.min(z, b.x0)])
      if (b.x1 < z) next.push([Math.max(a, b.x1), z])
    }
    spans = next.filter(([a, z]) => z - a > 0)
  }
  return spans
}

/** Lays one row out left to right across the free spans, or fails. */
function layoutRow(
  items: PackItem[],
  zTop: number,
  scale: number
): { rooms: BlueprintRoom[]; depth: number } | null {
  const depth = Math.max(...items.map((i) => i.d)) * scale
  if (zTop + depth > Z_FRONT + 1e-9) return null

  const spans = freeSpans(zTop, zTop + depth)
  if (spans.length === 0) return null

  const rooms: BlueprintRoom[] = []
  let span = 0
  let x = spans[0][0]
  for (const item of items) {
    const w = item.w * scale
    while (span < spans.length && x + w > spans[span][1] + 1e-9) {
      span++
      if (span < spans.length) x = spans[span][0]
    }
    if (span >= spans.length) return null
    const d = item.d * scale
    rooms.push({
      name: item.name,
      x: round(x + w / 2),
      z: round(zTop + d / 2),
      w: round(w),
      d: round(d),
      h: round(item.h * scale),
      kind: item.kind,
      params: item.params,
    })
    x += w + GAP
  }
  return { rooms, depth }
}

/**
 * Sizes are relative, not absolute: a floor whose rooms will not fit the slab
 * shrinks all of them by one factor rather than spilling over the walls or
 * losing a tool. A page with ten small tools packs at full size; one with ten
 * eight-parameter tools packs smaller, which is what a crowded page looks like.
 */
const SCALES = [1, 0.95, 0.9, 0.85, 0.8, 0.75, 0.7, 0.65, 0.6, 0.55, 0.5, 0.45, 0.4, 0.35, 0.3, 0.25, 0.2]

function packAt(items: PackItem[], scale: number, partial: boolean): PackedFloor | null {
  const rooms: BlueprintRoom[] = []
  const gaps: number[] = []
  const starts: number[] = [Z_BACK]
  const dropped: string[] = []
  let zTop = Z_BACK
  let row: PackItem[] = []
  let laid: { rooms: BlueprintRoom[]; depth: number } | null = null

  for (const item of items) {
    const grown = layoutRow([...row, item], zTop, scale)
    if (grown) {
      row = [...row, item]
      laid = grown
      continue
    }
    if (laid === null) {
      // Nothing placed yet in this row and the room still will not fit: the
      // slab is out of space, not the row.
      if (!partial) return null
      dropped.push(item.name)
      continue
    }
    rooms.push(...laid.rooms)
    zTop += laid.depth + GAP
    gaps.push(round(zTop - GAP / 2))
    starts.push(zTop)
    row = [item]
    laid = layoutRow(row, zTop, scale)
    if (laid === null) {
      if (!partial) return null
      dropped.push(item.name)
      row = []
    }
  }
  if (laid !== null) rooms.push(...laid.rooms)

  // The trunk turns along the first gap that has a row clear of the core in
  // front of it — the same test the packer used when it reserved the column.
  const clear = starts.findIndex((z, i) => i > 0 && z >= CORE_FRONT)
  return { rooms, gaps, turn: clear > 0 ? clear - 1 : -1, scale, dropped }
}

/**
 * Shelf-packs rooms into the floor rectangle, back wall first, left to right,
 * leaving the service core and the front aisle clear. Callers pass the rooms
 * already in the order they should appear.
 */
export function packRooms(items: PackItem[]): PackedFloor {
  if (items.length === 0) return { rooms: [], gaps: [], turn: -1, scale: 1, dropped: [] }
  for (const scale of SCALES) {
    const packed = packAt(items, scale, false)
    if (packed) return packed
  }
  // Unreachable for any page a scanner has produced; a floor is still a floor.
  return packAt(items, SCALES[SCALES.length - 1], true) as PackedFloor
}

/**
 * Circulation for one floor: the trunk out of the core door, a lateral lane
 * along every row gap, and the aisle across the front. All axis-aligned, and
 * all clear of the rooms by construction — the packer reserved the trunk's
 * column and edge before it placed anything.
 */
export function lanesFor(gaps: number[], turn: number): [number, number][][] {
  const door: [number, number] = [CORE_DOOR.x, CORE_DOOR.z]
  const trunk: [number, number][] =
    turn < 0
      ? [door, [TRUNK_X, AISLE_Z]]
      : [door, [TRUNK_X, gaps[turn]], [TRUNK_LEFT_X, gaps[turn]], [TRUNK_LEFT_X, AISLE_Z]]
  const laterals: [number, number][][] = gaps.map((z) => [
    [-LANE_X, z],
    [LANE_X, z],
  ])
  return [
    trunk,
    ...laterals,
    [
      [-LANE_X, AISLE_Z],
      [LANE_X, AISLE_Z],
    ],
  ]
}

// ---------------------------------------------------------------- floors

/** Pages become floors in scan order; a building stays low enough to read. */
export const MAX_FLOORS = 3
/** Past this, a page's rooms overflow into an annex floor on the same route. */
export const ROOMS_PER_FLOOR = 10

const byName = (a: { name: string }, b: { name: string }): number => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0)

/**
 * Which page each tool belongs to: the first page, in scan order, that
 * registered it. A tool available on every page is one room, on the floor
 * where an agent first meets it.
 */
export function homePages(pages: ScanPage[], tools: ScanTool[]): Map<string, string> {
  const home = new Map<string, string>()
  for (const page of pages) {
    for (const name of page.tools) if (!home.has(name)) home.set(name, page.path)
  }
  for (const tool of tools) if (!home.has(tool.name)) home.set(tool.name, tool.page)
  return home
}

function floorFor(name: string, route: string, items: PackItem[]): BlueprintFloor {
  const packed = packRooms(items)
  return { name, route, rooms: packed.rooms, lanes: lanesFor(packed.gaps, packed.turn) }
}

// ---------------------------------------------------------------- blueprint

export function deriveBlueprint(listing: DirectoryListing): Blueprint {
  const seed = fnv1a(listing.slug)
  const report = listing.report
  const kinds = new Map(listing.tools.map((t) => [t.name, t.kind]))
  const home = homePages(report.pages, report.tools)

  const itemFor = (tool: ScanTool): PackItem => {
    const params = paramCount(tool.inputSchema)
    const kind = roomKind(kinds.get(tool.name) ?? tool.risk, params)
    return { name: tool.name, kind, params, ...roomSize(kind, params) }
  }

  const floors: BlueprintFloor[] = []
  for (const page of report.pages.filter((p) => p.tools.length > 0).slice(0, MAX_FLOORS)) {
    const items = report.tools
      .filter((t) => home.get(t.name) === page.path)
      .map(itemFor)
      .sort(byName)
    const name = page.title || page.path
    floors.push(floorFor(name, page.path, items.slice(0, ROOMS_PER_FLOOR)))
    // One annex, on the same route: a page with more rooms than a floor holds
    // is still one page, and the building says so by repeating its name.
    if (items.length > ROOMS_PER_FLOOR) {
      floors.push(floorFor(`${name} (2)`, page.path, items.slice(ROOMS_PER_FLOOR)))
    }
  }
  // The building exists even when the scan found nothing to put in it.
  if (floors.length === 0) floors.push(floorFor(listing.host, report.pages[0]?.path ?? '/', []))

  const floorOf = new Map<string, number>()
  const kindCounts: Record<BlueprintKind, number> = { nav: 0, form: 0, content: 0, action: 0, data: 0 }
  floors.forEach((floor, i) => {
    for (const room of floor.rooms) {
      floorOf.set(room.name, i)
      kindCounts[room.kind]++
    }
  })

  const walks: BlueprintWalk[] = []
  const stopsFor = (names: string[]): [number, string][] =>
    names.filter((n) => floorOf.has(n)).map((n) => [floorOf.get(n) as number, n])
  for (const journey of listing.suggested_journeys) {
    if (journey.tools.length === 0 || !journey.tools.every((n) => floorOf.has(n))) continue
    walks.push({ name: journey.intent, who: 'agent', stops: stopsFor(journey.tools), delay: 2 * walks.length })
  }
  // The one walk somebody actually took. A failed run is not re-enacted.
  if (listing.journey && listing.journey.outcome === 'passed') {
    const stops = stopsFor(listing.journey.tools_called)
    if (stops.length > 0) {
      walks.push({ name: listing.journey.intent, who: 'test', stops, delay: 2 * walks.length })
    }
  }

  return {
    v: 1,
    slug: listing.slug,
    name: listing.name,
    host: listing.host,
    category: listing.category,
    seed,
    floors,
    walks,
    facade: facadeFor(listing, seed),
    stats: { pages: listing.counts.pages, tools: listing.counts.tools, kinds: kindCounts },
  }
}

/** One line per listing for the build log, so a shape change is visible in a diff. */
export function blueprintSummary(b: Blueprint): string {
  const rooms = b.floors.reduce((n, f) => n + f.rooms.length, 0)
  return (
    `${b.slug}: ${b.facade.archetype}/${b.facade.variant} ${b.facade.roof}, ` +
    `${b.floors.length} floor(s), ${rooms} room(s), ${b.walks.length} walk(s)`
  )
}
