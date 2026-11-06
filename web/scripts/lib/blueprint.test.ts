import path from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ARCHETYPE_CATEGORIES,
  GAP,
  MAX_FLOORS,
  ROOFS,
  ROOMS_PER_FLOOR,
  Z_FRONT,
  archetypeFor,
  deriveBlueprint,
  fnv1a,
  lanesFor,
  packRooms,
  paramCount,
  roomKind,
  roomSize,
  subSeed,
} from './blueprint'
import { loadDirectory } from './directory'
import { AISLE_Z, CORE, FLOOR_D, FLOOR_W } from '../../src/components/building/model'
import type { Blueprint, BlueprintRoom } from '../../src/types/blueprint'
import type { DirectoryListing } from '../../src/types/directory'

const FIXTURES = path.resolve(__dirname, '__fixtures__/blueprint')
const DIRECTORY_FIXTURES = path.resolve(__dirname, '__fixtures__/directory')

let warn: ReturnType<typeof vi.spyOn>
beforeEach(() => {
  warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
})
afterEach(() => {
  warn.mockRestore()
})

function listing(slug: string, dir = FIXTURES): DirectoryListing {
  const found = loadDirectory(dir).listings.find((l) => l.slug === slug)
  if (!found) throw new Error(`fixture listing ${slug} did not load`)
  return found
}

// ---------------------------------------------------------------- invariants

interface Box {
  x0: number
  x1: number
  z0: number
  z1: number
}

const boxOf = (r: BlueprintRoom): Box => ({
  x0: r.x - r.w / 2,
  x1: r.x + r.w / 2,
  z0: r.z - r.d / 2,
  z1: r.z + r.d / 2,
})
// Touching is allowed; sharing area is not.
const overlaps = (a: Box, b: Box): boolean =>
  a.x0 < b.x1 - 1e-6 && b.x0 < a.x1 - 1e-6 && a.z0 < b.z1 - 1e-6 && b.z0 < a.z1 - 1e-6

const CORE_BOX: Box = {
  x0: CORE.x - CORE.w / 2,
  x1: CORE.x + CORE.w / 2,
  z0: CORE.z - CORE.d / 2,
  z1: CORE.z + CORE.d / 2,
}

/** Everything a floor has to satisfy whatever the scan looked like. */
function problemsIn(b: Blueprint): string[] {
  const problems: string[] = []
  b.floors.forEach((floor, fi) => {
    const where = `${b.slug} floor ${fi}`
    const boxes = floor.rooms.map(boxOf)
    boxes.forEach((box, i) => {
      const name = floor.rooms[i].name
      if (overlaps(box, CORE_BOX)) problems.push(`${where}: ${name} is in the core`)
      if (box.z1 > AISLE_Z + 1e-6) problems.push(`${where}: ${name} crosses the aisle`)
      if (box.x0 < -FLOOR_W / 2 || box.x1 > FLOOR_W / 2 || box.z0 < -FLOOR_D / 2) {
        problems.push(`${where}: ${name} is off the slab`)
      }
      for (let j = i + 1; j < boxes.length; j++) {
        if (overlaps(box, boxes[j])) problems.push(`${where}: ${name} overlaps ${floor.rooms[j].name}`)
      }
      for (const lane of floor.lanes) {
        for (let k = 0; k + 1 < lane.length; k++) {
          const seg: Box = {
            x0: Math.min(lane[k][0], lane[k + 1][0]),
            x1: Math.max(lane[k][0], lane[k + 1][0]),
            z0: Math.min(lane[k][1], lane[k + 1][1]),
            z1: Math.max(lane[k][1], lane[k + 1][1]),
          }
          if (overlaps(box, seg)) problems.push(`${where}: ${name} sits on a lane`)
        }
      }
    })
    for (const lane of floor.lanes) {
      if (lane.length < 2) problems.push(`${where}: a lane with one vertex`)
      for (const [x, z] of lane) {
        if (Math.abs(x) > FLOOR_W / 2 || Math.abs(z) > FLOOR_D / 2) {
          problems.push(`${where}: lane vertex ${x},${z} is off the slab`)
        }
      }
      for (let k = 0; k + 1 < lane.length; k++) {
        if (lane[k][0] !== lane[k + 1][0] && lane[k][1] !== lane[k + 1][1]) {
          problems.push(`${where}: a lane segment is not axis-aligned`)
        }
      }
    }
    // The first vertex of the first lane is the door out of the core.
    expect(floor.lanes[0][0]).toEqual([-2.65, -2.5])
  })
  return problems
}

// ---------------------------------------------------------------- seeding

describe('fnv1a', () => {
  it('is stable, slug-sensitive and unsigned 32-bit', () => {
    expect(fnv1a('alpha-tools')).toBe(fnv1a('alpha-tools'))
    expect(fnv1a('alpha-tools')).not.toBe(fnv1a('beta-tools'))
    for (const s of ['', 'a', 'alpha-tools', 'a-very-long-slug-indeed']) {
      const h = fnv1a(s)
      expect(Number.isInteger(h)).toBe(true)
      expect(h).toBeGreaterThanOrEqual(0)
      expect(h).toBeLessThan(2 ** 32)
    }
  })

  it('gives each labelled choice its own stream', () => {
    const seed = fnv1a('alpha-tools')
    expect(subSeed(seed, 'variant')).not.toBe(subSeed(seed, 'palette'))
    expect(subSeed(seed, 'variant')).toBe(subSeed(seed, 'variant'))
  })
})

// ---------------------------------------------------------------- facade

describe('archetypeFor', () => {
  it('maps every category on the table and calls everything else an office', () => {
    expect(archetypeFor('commerce')).toBe('storefront')
    expect(archetypeFor('crypto')).toBe('bank')
    expect(archetypeFor('devtools')).toBe('workshop')
    expect(archetypeFor('news')).toBe('theatre')
    expect(archetypeFor('logistics')).toBe('terminal')
    expect(archetypeFor('wellness')).toBe('clinic')
    expect(archetypeFor('productivity')).toBe('library')
    expect(archetypeFor('other')).toBe('office')
    expect(archetypeFor('')).toBe('office')
  })

  it('gives every archetype three roofs', () => {
    for (const [archetype] of ARCHETYPE_CATEGORIES) expect(ROOFS[archetype]).toHaveLength(3)
    expect(ROOFS.office).toHaveLength(3)
  })
})

// ---------------------------------------------------------------- rooms

describe('room derivation', () => {
  it('counts only an object schema with object properties', () => {
    expect(paramCount({ type: 'object', properties: { a: {}, b: {} } })).toBe(2)
    expect(paramCount({ type: 'object' })).toBe(0)
    expect(paramCount({ type: 'object', properties: 'nope' })).toBe(0)
    expect(paramCount(undefined)).toBe(0)
    expect(paramCount('{"properties":{"a":{}}}')).toBe(0)
  })

  it('splits read tools by how much they ask for', () => {
    expect(roomKind('action', 0)).toBe('action')
    expect(roomKind('sensitive', 9)).toBe('data')
    expect(roomKind('read', 0)).toBe('nav')
    expect(roomKind('read', 1)).toBe('content')
    expect(roomKind('read', 2)).toBe('form')
  })

  it('grows a room with its parameters, up to a cap', () => {
    expect(roomSize('nav', 0)).toEqual({ w: 1.6, d: 1.2, h: 0.45 })
    expect(roomSize('form', 4).w).toBeCloseTo(3)
    expect(roomSize('form', 40)).toEqual(roomSize('form', 4))
    expect(roomSize('action', 0).h).toBe(1)
  })
})

// ---------------------------------------------------------------- packing

describe('packRooms', () => {
  const items = (n: number, params = 0) =>
    Array.from({ length: n }, (_, i) => {
      const name = `tool_${String(i).padStart(2, '0')}`
      return { name, kind: 'form' as const, params, ...roomSize('form', params) }
    })

  it('fills from the back wall toward the aisle, left to right', () => {
    const { rooms } = packRooms(items(3))
    expect(rooms.map((r) => r.name)).toEqual(['tool_00', 'tool_01', 'tool_02'])
    expect(new Set(rooms.map((r) => r.z)).size).toBe(1)
    expect(rooms[0].x).toBeLessThan(rooms[1].x)
    expect(rooms[0].z - rooms[0].d / 2).toBeCloseTo(-FLOOR_D / 2 + GAP)
    expect(rooms[1].x - rooms[1].w / 2 - (rooms[0].x + rooms[0].w / 2)).toBeCloseTo(GAP)
  })

  it('opens a new row rather than spilling past the aisle', () => {
    const { rooms, gaps } = packRooms(items(8, 4))
    expect(gaps.length).toBeGreaterThan(0)
    for (const r of rooms) expect(r.z + r.d / 2).toBeLessThanOrEqual(Z_FRONT + 1e-6)
  })

  it('shrinks a crowded floor rather than dropping a tool or leaving the slab', () => {
    const crowded = packRooms(items(12, 4))
    expect(crowded.rooms).toHaveLength(12)
    expect(crowded.dropped).toEqual([])
    expect(crowded.scale).toBeLessThan(1)
    // Sizes stay in proportion: one factor for the whole floor.
    expect(new Set(crowded.rooms.map((r) => r.w)).size).toBe(1)
  })

  it('draws a trunk, one lane per row gap, and the aisle', () => {
    const { gaps, turn } = packRooms(items(8, 4))
    const lanes = lanesFor(gaps, turn)
    expect(lanes).toHaveLength(gaps.length + 2)
    expect(lanes[0][0]).toEqual([-2.65, -2.5])
    expect(lanes[lanes.length - 1]).toEqual([
      [-4.4, AISLE_Z],
      [4.4, AISLE_Z],
    ])
  })

  it('has nothing to place, and no row gaps, for an empty floor', () => {
    expect(packRooms([])).toEqual({ rooms: [], gaps: [], turn: -1, scale: 1, dropped: [] })
    expect(lanesFor([], -1)).toHaveLength(2)
  })
})

// ---------------------------------------------------------------- blueprint

describe('deriveBlueprint', () => {
  it('turns pages into floors and tools into rooms', () => {
    const b = deriveBlueprint(listing('three-pages'))
    expect(b.v).toBe(1)
    expect(b.slug).toBe('three-pages')
    expect(b.floors.map((f) => f.name)).toEqual(['Home', 'Catalog', '/checkout'])
    expect(b.floors.map((f) => f.route)).toEqual(['/', '/catalog', '/checkout'])
    // A page past the floor cap, and a page with no tools, are not floors.
    expect(b.floors).toHaveLength(MAX_FLOORS)
    expect(b.floors.flatMap((f) => f.rooms.map((r) => r.name))).not.toContain('contact_support')
  })

  it('takes the kind a maintainer corrected, and the size the schema declares', () => {
    const home = deriveBlueprint(listing('three-pages')).floors[0]
    const kinds = Object.fromEntries(home.rooms.map((r) => [r.name, r.kind]))
    // The scan called `browse` an action; the listing calls it a read with no
    // parameters, which is a doorway.
    expect(kinds).toEqual({
      about: 'content',
      account_balance: 'data',
      browse: 'nav',
      buy_item: 'action',
      search_items: 'form',
    })
    const search = home.rooms.find((r) => r.name === 'search_items')
    expect(search?.params).toBe(3)
    expect(search?.w).toBeGreaterThan(home.rooms.find((r) => r.name === 'browse')!.w)
  })

  it('gives a tool seen on two pages one room, on the page it was first seen on', () => {
    const b = deriveBlueprint(listing('three-pages'))
    const rooms = b.floors.flatMap((f, i) => f.rooms.map((r) => `${i}:${r.name}`))
    expect(rooms.filter((r) => r.endsWith(':browse'))).toEqual(['0:browse'])
    expect(b.floors[1].rooms.map((r) => r.name)).toEqual(['filter_items'])

    // The same shape in the directory fixture, where `search` is on / and /docs.
    const alpha = deriveBlueprint(listing('alpha-tools', DIRECTORY_FIXTURES))
    expect(alpha.floors.flatMap((f) => f.rooms.map((r) => r.name)).filter((n) => n === 'search')).toHaveLength(1)
  })

  it('walks every suggested journey that resolves, then the run that passed', () => {
    const b = deriveBlueprint(listing('three-pages'))
    expect(b.walks).toEqual([
      {
        name: 'Find an item and buy it',
        who: 'agent',
        stops: [
          [0, 'search_items'],
          [0, 'buy_item'],
        ],
        delay: 0,
      },
      {
        name: 'Check out with a saved card',
        who: 'test',
        stops: [
          [0, 'search_items'],
          [2, 'pay'],
          [2, 'card_details'],
        ],
        delay: 2,
      },
    ])
    // Every stop names a room that exists on the floor it names.
    for (const walk of b.walks) {
      for (const [floor, name] of walk.stops) {
        expect(b.floors[floor].rooms.some((r) => r.name === name)).toBe(true)
      }
    }
  })

  it('leaves out a journey whose tools did not all become rooms', () => {
    const b = deriveBlueprint(listing('three-pages'))
    expect(b.walks.map((w) => w.name)).not.toContain('Ask a human for help')
  })

  it('does not re-enact a journey that failed', () => {
    const l = listing('three-pages')
    const failed: DirectoryListing = { ...l, journey: { ...l.journey!, outcome: 'failed' } }
    expect(deriveBlueprint(failed).walks.map((w) => w.who)).toEqual(['agent'])
  })

  it('counts the scan, and the rooms by kind', () => {
    const b = deriveBlueprint(listing('three-pages'))
    expect(b.stats.pages).toBe(5)
    expect(b.stats.tools).toBe(9)
    expect(b.stats.kinds).toEqual({ nav: 1, form: 2, content: 1, action: 2, data: 2 })
    const rooms = b.floors.reduce((n, f) => n + f.rooms.length, 0)
    expect(Object.values(b.stats.kinds).reduce((a, n) => a + n, 0)).toBe(rooms)
  })

  it('wears the archetype its category maps to, and signs itself', () => {
    const b = deriveBlueprint(listing('three-pages'))
    expect(b.facade.archetype).toBe('storefront')
    expect(b.facade.roof).toBe(ROOFS.storefront[b.facade.variant])
    expect(b.facade.sign).toBe('Three Pages')
    expect(b.facade.sightkick).toBe(true)
    expect(b.facade.variant).toBeGreaterThanOrEqual(0)
    expect(b.facade.variant).toBeLessThan(3)
    expect(b.facade.palette).toBeGreaterThanOrEqual(0)
    expect(b.facade.palette).toBeLessThan(4)
  })

  it('is the same building twice, and a different one for a different slug', () => {
    const l = listing('three-pages')
    expect(deriveBlueprint(l)).toEqual(deriveBlueprint(l))

    const other = deriveBlueprint(listing('crowded'))
    const b = deriveBlueprint(l)
    expect(other.seed).not.toBe(b.seed)
    expect(other.facade.variant).not.toBe(b.facade.variant)
    expect(other.facade.palette).not.toBe(b.facade.palette)
    // Renaming the site moves nothing: the slug is the only source of variation.
    const renamed = deriveBlueprint({ ...l, name: 'Something Else' })
    expect(renamed.facade.variant).toBe(b.facade.variant)
    expect(renamed.facade.palette).toBe(b.facade.palette)
  })

  it('overflows a page with more rooms than a floor holds into an annex', () => {
    const b = deriveBlueprint(listing('crowded'))
    expect(b.floors.map((f) => f.name)).toEqual(['Console', 'Console (2)'])
    expect(b.floors.map((f) => f.route)).toEqual(['/', '/'])
    expect(b.floors[0].rooms).toHaveLength(ROOMS_PER_FLOOR)
    expect(b.floors[1].rooms).toHaveLength(4)
    // Sorted by name, and each tool on exactly one of the two floors.
    const names = b.floors.flatMap((f) => f.rooms.map((r) => r.name))
    expect(names).toEqual([...names].sort())
    expect(new Set(names).size).toBe(14)
  })

  it('is a lobby when the scan found no tools at all', () => {
    const b = deriveBlueprint(listing('no-tools'))
    expect(b.floors).toHaveLength(1)
    expect(b.floors[0]).toMatchObject({ name: 'quiet.example.org', route: '/', rooms: [] })
    expect(b.floors[0].lanes).toHaveLength(2)
    expect(b.walks).toEqual([])
    expect(b.stats.kinds).toEqual({ nav: 0, form: 0, content: 0, action: 0, data: 0 })
  })

  it('keeps every room on the slab, off the core, off the lanes and behind the aisle', () => {
    const listings = [...loadDirectory(FIXTURES).listings, ...loadDirectory(DIRECTORY_FIXTURES).listings]
    expect(listings.length).toBeGreaterThan(3)
    for (const l of listings) expect(problemsIn(deriveBlueprint(l))).toEqual([])
  })

  it('holds those invariants for a page of nothing but wide tools', () => {
    const l = listing('crowded')
    const tools = l.report.tools.map((t) => ({
      ...t,
      inputSchema: { type: 'object', properties: Object.fromEntries(Array.from({ length: 9 }, (_, i) => [`p${i}`, {}])) },
    }))
    const wide: DirectoryListing = { ...l, report: { ...l.report, tools } }
    const b = deriveBlueprint(wide)
    expect(b.floors.reduce((n, f) => n + f.rooms.length, 0)).toBe(14)
    expect(problemsIn(b)).toEqual([])
  })
})
