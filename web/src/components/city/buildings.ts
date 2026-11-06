// What stands on each lot, as plain data. The renderer turns this into a
// handful of instanced meshes; nothing here knows about three.js, so the
// massing of the whole city is testable and can be counted before it is drawn.
import type { Archetype, RoofStyle } from '@/types/blueprint'
import type { CityLandmark, CityLot } from '@/types/city'
import { FLOOR_D, FLOOR_H, FLOOR_W } from '@/components/building/model'
import { paletteColor } from '@/components/building/facade'
import { FILLER_WALLS, accent } from './palette'
import type { CityDocument, CityListing } from './document'

/** Footprint of every building in the city: the dollhouse's own floor plate. */
export const FOOTPRINT = { w: FLOOR_W, d: FLOOR_D }
export const STOREY = FLOOR_H
/** Even a rescan that finds four hundred pages may not build a spike. */
const MAX_STOREYS = 12

export type CityBuildingKind = 'listing' | 'filler' | 'empty' | 'landmark'

export interface CityBuilding {
  lot: CityLot
  kind: CityBuildingKind
  slug?: string
  name?: string
  archetype?: Archetype
  storeys: number
  /** Height of the box, in world units. */
  height: number
  wall: string
  roof: RoofStyle | null
  sign?: string
  sightkick: boolean
  /** 32-bit window-light mask; 0 draws no lit windows. */
  mask: number
  peak: boolean
  /** District accent, for the awning band and the props that belong to it. */
  accent: string
  /** The landmark this lot is holding, built or reserved. */
  landmark?: CityLandmark
}

function hash(n: number): number {
  let h = (n ^ 0x9e3779b9) >>> 0
  h = Math.imul(h ^ (h >>> 16), 0x85ebca6b)
  h = Math.imul(h ^ (h >>> 13), 0xc2b2ae35)
  return (h ^ (h >>> 16)) >>> 0
}

const FILLER_ROOFS: RoofStyle[] = ['flat', 'parapet', 'gable', 'hip', 'flat', 'parapet']

/**
 * Every lot, in plan order, with what it draws. A listing's building takes its
 * colours and roof from the blueprint's facade; a filler takes a neutral wall
 * and a hashed roof, and an empty lot takes nothing but its board.
 */
export function cityBuildings(city: CityDocument, listings: CityListing[]): CityBuilding[] {
  const byLot = new Map<number, CityListing & { peak: boolean }>()
  const bySlug = new Map(listings.map((l) => [l.slug, l]))
  for (const a of city.assignments) {
    const listing = bySlug.get(a.slug)
    if (listing) byLot.set(a.lot, { ...listing, peak: a.peak })
  }
  const fills = new Map(city.fills.map((f) => [f.lot, f]))
  const landmarks = new Map<number, CityLandmark>()
  for (const l of city.landmarks) if (l.lot !== undefined) landmarks.set(l.lot, l)
  const built = city.assignments.length

  const out: CityBuilding[] = []
  for (const lot of city.lots) {
    const districtAccent = accent(city.districts.find((d) => d.archetype === lot.district)?.accent ?? 0)
    const landmark = landmarks.get(lot.id)
    const listing = byLot.get(lot.id)

    if (listing) {
      const facade = listing.blueprint.facade
      const floors = Math.max(1, listing.blueprint.floors.length)
      const scaled = floors * lot.heightScale * (listing.peak ? 1.5 : 1)
      const storeys = Math.max(1, Math.min(MAX_STOREYS, Math.round(scaled)))
      out.push({
        lot,
        kind: 'listing',
        slug: listing.slug,
        name: listing.name,
        archetype: facade.archetype,
        storeys,
        height: storeys * STOREY,
        wall: paletteColor(facade.archetype, facade.palette),
        roof: facade.roof,
        sign: facade.sign || listing.name,
        sightkick: facade.sightkick,
        mask: hash(lot.id * 31 + 7),
        peak: listing.peak,
        accent: districtAccent,
        landmark,
      })
      continue
    }

    if (landmark) {
      // The park and the mast are ground and prop, never a massing. A
      // milestone lot is a fenced gap until the directory earns it.
      const earned = landmark.minListings === undefined || built >= landmark.minListings
      const monument = landmark.minListings !== undefined && earned
      out.push({
        lot,
        kind: monument ? 'landmark' : landmark.minListings === undefined ? 'landmark' : 'empty',
        name: landmark.name,
        storeys: monument ? 4 : 0,
        height: monument ? 4 * STOREY : 0,
        wall: FILLER_WALLS[0],
        roof: monument ? 'parapet' : null,
        sightkick: false,
        mask: 0,
        peak: false,
        accent: districtAccent,
        landmark,
      })
      continue
    }

    const fill = fills.get(lot.id)
    if (fill && fill.kind === 'filler') {
      const h = hash(lot.id)
      const storeys = Math.max(1, Math.min(MAX_STOREYS, fill.storeys))
      out.push({
        lot,
        kind: 'filler',
        storeys,
        height: storeys * STOREY,
        wall: FILLER_WALLS[h % FILLER_WALLS.length],
        roof: FILLER_ROOFS[(h >>> 8) % FILLER_ROOFS.length],
        sightkick: false,
        mask: fill.mask,
        peak: false,
        accent: districtAccent,
      })
      continue
    }

    out.push({
      lot,
      kind: 'empty',
      storeys: 0,
      height: 0,
      wall: FILLER_WALLS[1],
      roof: null,
      sightkick: false,
      mask: 0,
      peak: false,
      accent: districtAccent,
    })
  }
  return out
}

/** Lots with something standing on them, for the ground's contact shadows. */
export function builtLots(buildings: CityBuilding[]): Set<number> {
  const out = new Set<number>()
  for (const b of buildings) if (b.height > 0) out.add(b.lot.id)
  return out
}

export interface CityWindow {
  x: number
  y: number
  z: number
  /** 0 and 2 face ±Z, 1 and 3 face ±X. */
  face: 0 | 1 | 2 | 3
  lit: boolean
}

const BAYS_X = 4
const BAYS_Z = 3

/**
 * The window grid of one building, from its lot's mask. Cheap and regular:
 * at city distance a window is four pixels, and what carries the night is
 * which ones are lit, not where they are.
 */
export function cityWindows(b: CityBuilding, out: CityWindow[] = []): CityWindow[] {
  out.length = 0
  if (b.storeys <= 0) return out
  const halfW = FOOTPRINT.w / 2
  const halfD = FOOTPRINT.d / 2
  let bit = 0
  for (let s = 0; s < b.storeys; s++) {
    const y = s * STOREY + STOREY * 0.55
    for (let i = 0; i < BAYS_X; i++) {
      const x = -halfW + (FOOTPRINT.w / BAYS_X) * (i + 0.5)
      for (const face of [0, 2] as const) {
        const lit = ((b.mask >>> (bit++ & 31)) & 1) === 1
        out.push({ x, y, z: face === 0 ? halfD + 0.03 : -halfD - 0.03, face, lit })
      }
    }
    for (let i = 0; i < BAYS_Z; i++) {
      const z = -halfD + (FOOTPRINT.d / BAYS_Z) * (i + 0.5)
      for (const face of [1, 3] as const) {
        const lit = ((b.mask >>> (bit++ & 31)) & 1) === 1
        out.push({ x: face === 1 ? halfW + 0.03 : -halfW - 0.03, y, z, face, lit })
      }
    }
  }
  return out
}
