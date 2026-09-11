import { describe, expect, it } from 'vitest'
import type { CityLot } from '../../src/types/city'
import { archetypeFor } from './blueprint'
import {
  activeMilestones,
  assignLots,
  centralityAt,
  CityListingInput,
  fillerFor,
  fnv1a,
  frontingRoad,
  frontPoint,
  heightScaleFor,
  planCity,
  pointPolylineDist,
  reservedLots,
  windowMask,
} from './city'

const plan = planCity()

/** Shortest distance from a rectangle to a polyline, by sampling the line. */
function polylineRectDist(points: [number, number][], r: { x: number; z: number; w: number; d: number }): number {
  let best = Infinity
  for (let i = 0; i + 1 < points.length; i++) {
    const [ax, az] = points[i]
    const [bx, bz] = points[i + 1]
    const steps = Math.max(2, Math.ceil(Math.hypot(bx - ax, bz - az) * 4))
    for (let s = 0; s <= steps; s++) {
      const x = ax + ((bx - ax) * s) / steps
      const z = az + ((bz - az) * s) / steps
      const dx = Math.max(Math.abs(x - r.x) - r.w / 2, 0)
      const dz = Math.max(Math.abs(z - r.z) - r.d / 2, 0)
      best = Math.min(best, Math.hypot(dx, dz))
    }
  }
  return best
}

const seedListings: CityListingInput[] = [
  { slug: 'attio-com', category: 'productivity', added: '2026-09-10', tools: toolsOf(2) },
  { slug: 'coinranking-com', category: 'data', added: '2026-09-10', tools: toolsOf(21) },
  { slug: 'emorahealth-com', category: 'health', added: '2026-09-10', tools: toolsOf(10) },
  { slug: 'flatwrite-md', category: 'other', added: '2026-09-10', tools: toolsOf(12) },
  { slug: 'forter-com', category: 'finance', added: '2026-09-10', tools: toolsOf(6) },
  { slug: 'qrcodecrafter-com', category: 'productivity', added: '2026-09-10', tools: toolsOf(13) },
  { slug: 'telnyx-com', category: 'devtools', added: '2026-09-10', tools: toolsOf(4) },
  { slug: 'webmcp-com', category: 'devtools', added: '2026-09-10', tools: toolsOf(7) },
]

function toolsOf(n: number): CityListingInput['tools'] {
  return Array.from({ length: n }, (_, i) => ({ name: `t${i}`, kind: 'read' as const, description: '', page: '/' }))
}

describe('planCity', () => {
  it('is the same city every time it is built', () => {
    expect(JSON.stringify(planCity())).toBe(JSON.stringify(planCity()))
  })

  it('lays three tiers of road: the ring, two cross avenues, a diagonal, six stubs', () => {
    const byId = new Map(plan.roads.map((r) => [r.id, r]))
    const ring = byId.get('ring')!
    expect(ring.tier).toBe(2)
    expect(ring.width).toBe(8)
    expect(ring.closed).toBe(true)
    expect(ring.points[0]).toEqual(ring.points[ring.points.length - 1])
    expect(byId.get('avenue-ns')!.tier).toBe(2)
    expect(byId.get('avenue-ew')!.tier).toBe(2)
    expect(byId.get('diagonal')!.points.length).toBeGreaterThan(2)

    const stubs = plan.roads.filter((r) => r.kind === 'cul-de-sac')
    expect(stubs).toHaveLength(6)
    for (const stub of stubs) {
      expect(stub.tier).toBe(0)
      expect(stub.width).toBe(4)
      expect(stub.turningCircle).toBeGreaterThan(0)
      // Every stub leaves the ring and ends outside it.
      expect(pointPolylineDist(stub.points[0][0], stub.points[0][1], byId.get('ring')!.points)).toBeLessThan(0.001)
    }

    const streets = plan.roads.filter((r) => r.kind === 'street')
    expect(streets.length).toBeGreaterThan(20)
    for (const s of streets) expect([s.tier, s.width]).toEqual([1, 5])
    expect(new Set(plan.roads.map((r) => r.tier))).toEqual(new Set([0, 1, 2]))
  })

  it('splits the quadrants into blocks of 24 to 44 units a side', () => {
    const interior = plan.blocks.filter((b) => !b.rim)
    expect(interior.length).toBeGreaterThan(20)
    for (const b of interior) {
      expect(b.w).toBeGreaterThanOrEqual(24)
      expect(b.w).toBeLessThanOrEqual(44)
      expect(b.d).toBeGreaterThanOrEqual(24)
      expect(b.d).toBeLessThanOrEqual(44)
    }
  })

  it('holds enough lots for the directory to grow into', () => {
    // The observation tower is reserved for the 250th listing, so the ground
    // has to have more addresses than that before it is worth reserving.
    expect(plan.lots.length).toBeGreaterThanOrEqual(300)
  })

  it('leaves no two lots overlapping', () => {
    const lots = plan.lots
    expect(lots.length).toBeGreaterThan(100)
    for (let i = 0; i < lots.length; i++) {
      for (let j = i + 1; j < lots.length; j++) {
        const a = lots[i]
        const b = lots[j]
        const apart = Math.abs(a.x - b.x) >= (a.w + b.w) / 2 || Math.abs(a.z - b.z) >= (a.d + b.d) / 2
        expect(apart, `lots ${a.id} and ${b.id} overlap`).toBe(true)
      }
    }
  })

  it('puts every lot on a road, facing it, at the tier it records', () => {
    for (const lot of plan.lots) {
      const front = frontingRoad(plan.roads, lot)
      expect(front.gap, `lot ${lot.id} is ${front.gap} from its kerb`).toBeLessThanOrEqual(1)
      expect(lot.tier).toBe(front.tier)
      // The front point is on the far side of the lot from its centre.
      const [fx, fz] = frontPoint(lot)
      expect(Math.hypot(fx - lot.x, fz - lot.z)).toBeGreaterThan(0)
    }
  })

  it('gives the diagonal wedge lots that hug its kerb', () => {
    const diagonal = plan.roads.find((r) => r.id === 'diagonal')!
    const wedges = plan.lots.filter((l) => l.wedge)
    expect(wedges.length).toBeGreaterThanOrEqual(2)
    for (const lot of wedges) {
      const d = polylineRectDist(diagonal.points, lot)
      // Clear of the carriageway, but right up against it.
      expect(d).toBeGreaterThanOrEqual(diagonal.width / 2 - 0.01)
      expect(d).toBeLessThanOrEqual(diagonal.width / 2 + 1)
      // Still inside the block it was cut from.
      const block = plan.blocks.find((b) => b.id === lot.block)!
      expect(Math.abs(lot.x - block.x) + lot.w / 2).toBeLessThanOrEqual(block.w / 2 + 0.01)
      expect(Math.abs(lot.z - block.z) + lot.d / 2).toBeLessThanOrEqual(block.d / 2 + 0.01)
    }
  })

  it('peaks the centrality at the plaza and lets it fall below 0.2 at the corners', () => {
    expect(centralityAt(plan, 0, 0)).toBe(1)
    const { w, d } = plan.bounds
    for (const [x, z] of [
      [w / 2, d / 2],
      [-w / 2, d / 2],
      [w / 2, -d / 2],
      [-w / 2, -d / 2],
    ]) {
      expect(centralityAt(plan, x, z), `corner ${x},${z}`).toBeLessThan(0.2)
    }
    expect(Math.max(...plan.lots.map((l) => l.centrality))).toBeGreaterThan(0.9)
  })

  it('scales height in three steps, monotone in centrality', () => {
    expect(heightScaleFor(0)).toBe(1)
    expect(heightScaleFor(0.5)).toBe(2)
    expect(heightScaleFor(0.85)).toBe(3)
    const sorted = [...plan.lots].sort((a, b) => a.centrality - b.centrality)
    let last = 0
    for (const lot of sorted) {
      expect([1, 2, 3]).toContain(lot.heightScale)
      expect(lot.heightScale).toBe(heightScaleFor(lot.centrality))
      expect(lot.heightScale).toBeGreaterThanOrEqual(last)
      last = lot.heightScale
    }
    expect(new Set(plan.lots.map((l) => l.heightScale))).toEqual(new Set([1, 2, 3]))
  })

  it('gives every archetype a district and every lot its nearest seed', () => {
    expect(plan.districts).toHaveLength(8)
    expect(new Set(plan.districts.map((d) => d.archetype)).size).toBe(8)
    expect(new Set(plan.districts.map((d) => d.accent)).size).toBe(8)
    for (const lot of plan.lots.slice(0, 40)) {
      const nearest = [...plan.districts].sort(
        (a, b) => Math.hypot(lot.x - a.x, lot.z - a.z) - Math.hypot(lot.x - b.x, lot.z - b.z)
      )[0]
      expect(lot.district).toBe(nearest.archetype)
    }
  })

  it('stands up the three launch landmarks and reserves the three milestones', () => {
    const byId = new Map(plan.landmarks.map((l) => [l.id, l]))
    expect(byId.get('plaza')!.minListings).toBeUndefined()
    expect(byId.get('park')!.minListings).toBeUndefined()
    expect(byId.get('mast')!.minListings).toBeUndefined()
    expect(plan.lots[byId.get('park')!.lot!].wedge).toBe(true)
    expect(byId.get('city-hall')!.minListings).toBe(25)
    expect(byId.get('clock-tower')!.minListings).toBe(100)
    expect(byId.get('observation-tower')!.minListings).toBe(250)
    // The clock tower is on the diagonal; city hall has the best address.
    expect(plan.lots[byId.get('clock-tower')!.lot!].wedge).toBe(true)
    const hall = plan.lots[byId.get('city-hall')!.lot!]
    expect(hall.centrality).toBe(Math.max(...plan.lots.map((l) => l.centrality)))
    expect(new Set(plan.landmarks.map((l) => l.lot)).size).toBe(plan.landmarks.length)
  })

  it('places civic props by rule', () => {
    const kinds = plan.props.map((p) => p.kind)
    expect(kinds).toContain('fountain')
    expect(kinds).toContain('beacon')
    expect(kinds.filter((k) => k === 'water-tower')).toHaveLength(1)
    expect(kinds.filter((k) => k === 'bus-shelter').length).toBeGreaterThan(4)
    expect(kinds.filter((k) => k === 'billboard')).toHaveLength(plan.lots.filter((l) => l.wedge).length)
    expect(kinds.filter((k) => k === 'bench').length).toBeGreaterThan(8)
    expect(kinds.filter((k) => k === 'tree').length).toBeGreaterThan(12)
    for (const p of plan.props) expect(plan.districts.some((d) => d.archetype === p.district)).toBe(true)
  })

  it('runs closed loops that stay on the road they name', () => {
    expect(plan.loops.map((l) => l.id)).toEqual([
      'cars-ring',
      'cars-avenue-ns',
      'cars-avenue-ew',
      'cars-diagonal',
      'walk-plaza',
      'walk-park',
    ])
    const cars = plan.loops.filter((l) => l.kind === 'car')
    expect(cars.map((l) => l.count)).toEqual([12, 6, 6, 6])
    expect(plan.loops.filter((l) => l.kind === 'pedestrian').reduce((n, l) => n + l.count, 0)).toBeLessThanOrEqual(40)
    for (const loop of plan.loops) {
      const road = plan.roads.find((r) => r.id === loop.road)!
      expect(loop.tier).toBe(road.tier)
      expect(loop.phases).toHaveLength(loop.count)
      for (const p of loop.phases) expect(p).toBeGreaterThanOrEqual(0)
      for (const p of loop.phases) expect(p).toBeLessThan(1)
      const first = loop.points[0]
      const last = loop.points[loop.points.length - 1]
      expect(first, `${loop.id} is not closed`).toEqual(last)
      for (const [x, z] of loop.points) {
        expect(pointPolylineDist(x, z, road.points), `${loop.id} leaves ${road.id}`).toBeLessThanOrEqual(road.width / 2)
      }
    }
  })
})

describe('assignLots', () => {
  it('gives the eight seed listings eight distinct lots in their own districts', () => {
    const assignments = assignLots(plan, seedListings)
    expect(assignments).toHaveLength(8)
    expect(new Set(assignments.map((a) => a.lot)).size).toBe(8)
    const reserved = reservedLots(plan)
    for (const a of assignments) {
      const lot = plan.lots[a.lot]
      expect(reserved.has(a.lot), `${a.slug} took a reserved lot`).toBe(false)
      const listing = seedListings.find((l) => l.slug === a.slug)!
      expect(lot.district).toBe(archetypeFor(listing.category))
    }
    // (added, slug) order, which is the order the plan is grown in.
    expect(assignments.map((a) => a.slug)).toEqual([...seedListings].map((l) => l.slug).sort())
  })

  it('keeps every lot when a ninth listing arrives', () => {
    const before = assignLots(plan, seedListings)
    const ninth: CityListingInput = { slug: 'zed-io', category: 'devtools', added: '2026-10-01', tools: toolsOf(3) }
    const after = assignLots(plan, [...seedListings, ninth])
    expect(after).toHaveLength(9)
    for (const a of before) {
      expect(after.find((x) => x.slug === a.slug)!.lot, `${a.slug} moved`).toBe(a.lot)
    }
  })

  it('honours a persisted lot over the probe', () => {
    const free = plan.lots.find((l) => !reservedLots(plan).has(l.id) && l.id !== assignLots(plan, seedListings)[0].lot)!
    const pinned = seedListings.map((l) => (l.slug === 'telnyx-com' ? { ...l, lot: free.id } : l))
    const assignments = assignLots(plan, pinned)
    expect(assignments.find((a) => a.slug === 'telnyx-com')!.lot).toBe(free.id)
    expect(new Set(assignments.map((a) => a.lot)).size).toBe(8)
    // The probe would not have chosen it on its own.
    const natural = assignLots(plan, seedListings).find((a) => a.slug === 'telnyx-com')!.lot
    expect(natural).not.toBe(free.id)
  })

  it('probes from the slug hash, so the lot depends on the slug and nothing else', () => {
    const one: CityListingInput = { slug: 'alpha-io', category: 'finance', added: '2026-09-01', tools: toolsOf(1) }
    const solo = assignLots(plan, [one])[0]
    const start = fnv1a(one.slug) % plan.lots.length
    const reserved = reservedLots(plan)
    let expected = -1
    for (let i = 0; i < plan.lots.length && expected < 0; i++) {
      const lot = plan.lots[(start + i) % plan.lots.length]
      if (!reserved.has(lot.id) && lot.district === 'bank') expected = lot.id
    }
    expect(solo.lot).toBe(expected)
  })

  it('crowns the listing with the most tools as the peak', () => {
    const assignments = assignLots(plan, seedListings)
    expect(assignments.filter((a) => a.peak)).toHaveLength(1)
    expect(assignments.find((a) => a.peak)!.slug).toBe('coinranking-com')
  })

  it('falls back out of the district when it is full', () => {
    const crowd: CityListingInput[] = plan.lots
      .filter((l) => l.district === 'bank')
      .map((_, i) => ({ slug: `bank-${String(i).padStart(3, '0')}`, category: 'finance', added: '2026-09-10', tools: toolsOf(1) }))
    const extra: CityListingInput = { slug: 'zzz-overflow', category: 'finance', added: '2026-09-11', tools: toolsOf(1) }
    const assignments = assignLots(plan, [...crowd, extra])
    expect(new Set(assignments.map((a) => a.lot)).size).toBe(assignments.length)
    const spill = assignments.find((a) => a.slug === 'zzz-overflow')!
    expect(plan.lots[spill.lot].district).not.toBe('bank')
  })
})

describe('fillerFor', () => {
  const assignments = assignLots(plan, seedListings)
  const fills = fillerFor(plan, assignments)
  const byLot = new Map(fills.map((f) => [f.lot, f]))

  it('fills the good addresses and fences the poor ones', () => {
    const assigned = new Set(assignments.map((a) => a.lot))
    // A lot already built on draws nothing: the three launch landmarks stand
    // on theirs, and the listings stand on theirs.
    const standing = new Set(plan.landmarks.filter((l) => l.minListings === undefined).map((l) => l.lot))
    const waiting = new Set(plan.landmarks.filter((l) => l.minListings !== undefined).map((l) => l.lot))
    for (const lot of plan.lots) {
      const fill = byLot.get(lot.id)
      if (assigned.has(lot.id) || standing.has(lot.id)) {
        expect(fill, `lot ${lot.id} is taken and should not be filled`).toBeUndefined()
        continue
      }
      expect(fill).toBeDefined()
      expect(fill!.kind).toBe(!waiting.has(lot.id) && lot.centrality > 0.45 ? 'filler' : 'empty')
      if (fill!.kind === 'empty') {
        expect(fill!.storeys).toBe(0)
        expect(fill!.mask).toBe(0)
      } else {
        expect(fill!.storeys).toBeGreaterThanOrEqual(lot.heightScale)
        expect(fill!.storeys).toBeLessThanOrEqual(4 * lot.heightScale)
        expect(fill!.mask).toBe(windowMask(lot.id))
      }
    }
    expect(fills.filter((f) => f.kind === 'filler').length).toBeGreaterThan(10)
    expect(fills.filter((f) => f.kind === 'empty').length).toBeGreaterThan(10)
  })

  it('holds a milestone lot empty until the directory earns it, then builds on it', () => {
    const hall = plan.landmarks.find((l) => l.id === 'city-hall')!
    expect(activeMilestones(plan, 8)).toHaveLength(0)
    expect(byLot.get(hall.lot!)).toEqual({ lot: hall.lot, kind: 'empty', storeys: 0, mask: 0 })

    const grown: CityListingInput[] = Array.from({ length: 25 }, (_, i) => ({
      slug: `site-${String(i).padStart(3, '0')}`,
      category: 'other',
      added: '2026-09-10',
      tools: toolsOf(1),
    }))
    const many = assignLots(plan, grown)
    expect(many).toHaveLength(25)
    expect(activeMilestones(plan, 25).map((l) => l.id)).toEqual(['city-hall'])
    expect(fillerFor(plan, many).find((f) => f.lot === hall.lot)).toBeUndefined()
    expect(activeMilestones(plan, 100).map((l) => l.id)).toEqual(['city-hall', 'clock-tower'])
    expect(activeMilestones(plan, 250)).toHaveLength(3)
  })
})

describe('windowMask', () => {
  it('is a stable 32-bit mask that differs between lots', () => {
    expect(windowMask(7)).toBe(windowMask(7))
    expect(windowMask(7)).not.toBe(windowMask(8))
    for (const id of [0, 1, 2, 99]) {
      const m = windowMask(id)
      expect(Number.isInteger(m)).toBe(true)
      expect(m).toBeGreaterThanOrEqual(0)
      expect(m).toBeLessThanOrEqual(0xffffffff)
    }
  })
})

describe('archetypeFor', () => {
  it('maps the categories the directory uses and falls back to office', () => {
    const cases: [string, string][] = [
      ['commerce', 'storefront'],
      ['finance', 'bank'],
      ['devtools', 'workshop'],
      ['media', 'theatre'],
      ['travel', 'terminal'],
      ['health', 'clinic'],
      ['productivity', 'library'],
      ['data', 'library'],
      ['other', 'office'],
      ['made-up', 'office'],
    ]
    for (const [category, archetype] of cases) expect(archetypeFor(category)).toBe(archetype)
  })

  it('can reach every district the plan lays out', () => {
    // A district no category maps to is ground no listing can ever be given,
    // so the table and the eight seeds have to stay in step.
    const reachable = new Set(
      ['commerce', 'finance', 'devtools', 'media', 'travel', 'health', 'data', 'other'].map(archetypeFor)
    )
    expect(reachable).toEqual(new Set(plan.districts.map((d) => d.archetype)))
  })
})

describe('frontPoint', () => {
  it('walks to the middle of the edge the lot faces', () => {
    const lot = { x: 10, z: 20, w: 8, d: 6 } as CityLot
    expect(frontPoint({ ...lot, rotation: 0 })).toEqual([10, 23])
    expect(frontPoint({ ...lot, rotation: Math.PI })).toEqual([10, 17])
    expect(frontPoint({ ...lot, rotation: Math.PI / 2 })).toEqual([14, 20])
    expect(frontPoint({ ...lot, rotation: -Math.PI / 2 })).toEqual([6, 20])
  })
})
