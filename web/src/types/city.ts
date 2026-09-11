// The city the buildings stand in. Pure data, no three.js import, so the
// build can write it as JSON and the prerender can read counts from it.
//
// The plan is a fixed piece of ground: `planCity()` builds it from one
// constant seed and it never changes unless that seed does, so a listing that
// takes a lot keeps that lot for as long as the city stands. Listings are laid
// onto the plan separately (`assignLots`), which is why nothing here mentions
// a slug except `CityAssignment`.
//
// Units are the building page's world units (see components/building/model.ts)
// with +Y up, +X east and +Z south, so the plan reads like a map with north at
// the top. Every rectangle — block, lot — is axis-aligned; the one diagonal in
// the city is a road, and the lots it clips are shrunk to fit rather than cut
// into triangles.
import type { Archetype } from './blueprint'

/** What a road is for; `tier` sets its width and who drives on it. */
export type CityRoadKind = 'avenue' | 'street' | 'cul-de-sac' | 'path'

/** 2 = avenue (8 wide), 1 = local street (5), 0 = cul-de-sac stub and footpath (4). */
export type CityTier = 0 | 1 | 2

/** Which side of a block or rim band a row of lots faces. */
export type CityEdge = 'n' | 's' | 'e' | 'w'

export interface CityRoad {
  /** Stable id; loops and lots refer to roads by it. */
  id: string
  kind: CityRoadKind
  tier: CityTier
  /** Full carriageway width; the polyline is its centreline. */
  width: number
  /** Centreline as [x, z] pairs; a closed road repeats its first point last. */
  points: [number, number][]
  /** True when the polyline returns to its start (the ring, the plaza path). */
  closed: boolean
  /** Radius of the turning circle at the last point, for a cul-de-sac stub. */
  turningCircle?: number
}

export interface CityBlock {
  /** Index into `CityPlan.blocks`. */
  id: number
  /** Centre of the block rectangle. */
  x: number
  z: number
  /** Extent along x. */
  w: number
  /** Extent along z. */
  d: number
  /** Which edges carry a row of lots (interior blocks front a street on all four). */
  edges: CityEdge[]
  /** True for the bands outside the ring, which front the ring on one side only. */
  rim: boolean
  /** Centrality at the block centre, which sets its lot frontage and depth. */
  centrality: number
}

export interface CityLot {
  /** Index into `CityPlan.lots`; this is the number a listing's YAML persists. */
  id: number
  /** Centre of the lot rectangle. */
  x: number
  z: number
  /** Extent along x. */
  w: number
  /** Extent along z. */
  d: number
  /** Y rotation in radians that turns a building's +Z front toward its street. */
  rotation: number
  /** Id of the block this lot belongs to. */
  block: number
  /** Tier of the road the lot fronts. */
  tier: CityTier
  /** Nearest district seed, which is also the archetype the lot prefers. */
  district: Archetype
  /** 0 at the rim, 1 at the plaza; drives height and lot size. */
  centrality: number
  /** 1, 2 or 3: storey multiplier the closed facade draws the real floors at. */
  heightScale: 1 | 2 | 3
  /** True when the diagonal avenue clips this lot; these are landmark sites. */
  wedge: boolean
}

export interface CityDistrict {
  /** The archetype whose listings prefer this district. */
  archetype: Archetype
  /** Display name, for the city legend. */
  name: string
  /** Hand-placed seed point; a lot's district is its nearest seed. */
  x: number
  z: number
  /** Index into the city palette, for kerb tint, awnings and props — never walls. */
  accent: number
}

export type CityLandmarkKind =
  | 'plaza'
  | 'park'
  | 'mast'
  | 'city-hall'
  | 'clock-tower'
  | 'observation-tower'

export interface CityLandmark {
  id: string
  kind: CityLandmarkKind
  name: string
  x: number
  z: number
  /** Radius of the ground it occupies, for the ones that are not on a lot. */
  r?: number
  /** Lot this landmark stands on, for the ones that are. */
  lot?: number
  /** Directory size at which a reserved lot stops being empty and is built. */
  minListings?: number
}

export type CityPropKind =
  | 'fountain'
  | 'bus-shelter'
  | 'billboard'
  | 'bench'
  | 'tree'
  | 'water-tower'
  | 'beacon'

export interface CityProp {
  id: number
  kind: CityPropKind
  x: number
  z: number
  /** Y rotation in radians; 0 faces +Z. */
  rotation: number
  /** District the prop takes its accent from. */
  district: Archetype
}

export interface CityLoop {
  id: string
  /** Cars run on avenues, pedestrians on the plaza and park paths. */
  kind: 'car' | 'pedestrian'
  /** Id of the road this loop follows. */
  road: string
  /** Tier of that road, so the renderer can size and speed the traffic. */
  tier: CityTier
  /** How many agents run the loop; the renderer decides their speed. */
  count: number
  /** One seeded start offset in [0, 1) per agent, so they are not in lockstep. */
  phases: number[]
  /** Closed polyline: the first point is repeated last. */
  points: [number, number][]
}

export interface CityPlan {
  v: 1
  /** FNV-1a of the plan's constant seed string; the only source of variation. */
  seed: number
  /** The ground the city sits on, centred on the origin. */
  bounds: { x: number; z: number; w: number; d: number }
  roads: CityRoad[]
  blocks: CityBlock[]
  lots: CityLot[]
  districts: CityDistrict[]
  landmarks: CityLandmark[]
  props: CityProp[]
  loops: CityLoop[]
}

export interface CityAssignment {
  slug: string
  /** Id of the lot the listing stands on. */
  lot: number
  /** The one clear high point: the assigned listing with the most tools. */
  peak: boolean
}

/** What an unassigned lot draws, so growth is legible instead of blank. */
export interface CityFill {
  lot: number
  kind: 'filler' | 'empty'
  /** Storeys the filler massing draws; 0 for an empty lot. */
  storeys: number
  /** 32-bit window-light mask from the lot hash; 0 for an empty lot. */
  mask: number
}
