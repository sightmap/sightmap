// The city as the build hands it over: the plan, the listings placed on it,
// and what every other lot draws. `scripts/build-atlas.ts` writes exactly this
// shape into the generated atlas manifest and into public/atlas/city.json, so
// the page never derives a position and never fetches one.
import type { Blueprint } from '@/types/blueprint'
import type { CityAssignment, CityFill, CityLot, CityPlan } from '@/types/city'

export interface CityDocument extends CityPlan {
  assignments: CityAssignment[]
  fills: CityFill[]
}

/** What the page lends the city: one admitted listing and its building. */
export interface CityListing {
  slug: string
  name: string
  blueprint: Blueprint
}

/** Lots by id, for the places that hold an assignment or a fill instead. */
export function lotIndex(city: CityDocument): Map<number, CityLot> {
  return new Map(city.lots.map((lot) => [lot.id, lot]))
}

export interface CityStats {
  listings: number
  lots: number
  filler: number
  empty: number
  roads: number
  landmarks: number
}

export function cityStats(city: CityDocument): CityStats {
  let filler = 0
  let empty = 0
  for (const fill of city.fills) {
    if (fill.kind === 'filler') filler++
    else empty++
  }
  return {
    listings: city.assignments.length,
    lots: city.lots.length,
    filler,
    empty,
    roads: city.roads.length,
    // Only the landmarks that stand today; the milestones are still empty lots.
    landmarks: city.landmarks.filter((l) => l.minListings === undefined).length,
  }
}

const plural = (n: number, word: string): string => `${n} ${word}${n === 1 ? '' : 's'}`

/** The line under the heading: how much of the city is real. */
export function cityCaption(stats: CityStats): string {
  return (
    `${plural(stats.listings, 'listing')} on ${plural(stats.lots, 'lot')}, ` +
    `${plural(stats.filler, 'filler building')} and ${plural(stats.empty, 'empty lot')}, ` +
    `${plural(stats.roads, 'road')} and ${plural(stats.landmarks, 'landmark')}.`
  )
}

export interface CityLegendEntry {
  slug: string
  name: string
  /** The lot number the listing keeps for as long as the city stands. */
  lot: number
  /** Name of the district the lot sits in. */
  district: string
  peak: boolean
}

/**
 * The legend: every listing with an address, in lot order, which is also what
 * a visitor with no WebGL gets instead of the city.
 */
export function cityLegend(city: CityDocument, listings: CityListing[]): CityLegendEntry[] {
  const bySlug = new Map(listings.map((l) => [l.slug, l]))
  const lots = lotIndex(city)
  const out: CityLegendEntry[] = []
  for (const a of city.assignments) {
    const listing = bySlug.get(a.slug)
    const lot = lots.get(a.lot)
    if (!listing || !lot) continue
    const district = city.districts.find((d) => d.archetype === lot.district)
    out.push({
      slug: a.slug,
      name: listing.name,
      lot: a.lot,
      district: district?.name ?? lot.district,
      peak: a.peak,
    })
  }
  return out.sort((a, b) => a.name.localeCompare(b.name))
}
