import path from 'node:path'
import { renderToString } from 'react-dom/server'
import { StaticRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import { assignLots, fillerFor, planCity } from '../../../scripts/lib/city'
import { deriveBlueprint } from '../../../scripts/lib/blueprint'
import { loadDirectory } from '../../../scripts/lib/directory'
import CityPlaceholder from './CityPlaceholder'
import { cityCaption, cityLegend, cityStats, type CityDocument, type CityListing } from './document'

// The real directory and the real plan: what a visitor with no WebGL sees has
// to be the city's own legend, not a fixture that resembles one.
const directory = loadDirectory(path.resolve('src/data/directory'))
const plan = planCity()
const assignments = assignLots(plan, directory.listings)
const city: CityDocument = { ...plan, assignments, fills: fillerFor(plan, assignments) }
const listings: CityListing[] = directory.listings.map((l) => ({
  slug: l.slug,
  name: l.name,
  blueprint: deriveBlueprint(l),
}))

const html = renderToString(
  <StaticRouter location="/atlas/city">
    <CityPlaceholder entries={cityLegend(city, listings)} />
  </StaticRouter>
)

describe('the city legend', () => {
  it('has one entry per seed listing, each linking to its page', () => {
    expect(directory.listings).toHaveLength(8)
    for (const listing of directory.listings) {
      expect(html).toContain(`href="/atlas/${listing.slug}"`)
      expect(html).toContain(listing.name)
    }
  })

  it('gives every listing the lot it was assigned', () => {
    const legend = cityLegend(city, listings)
    expect(legend).toHaveLength(8)
    expect(new Set(legend.map((e) => e.lot)).size).toBe(8)
    for (const entry of legend) expect(html).toContain(`lot ${entry.lot}`)
    expect(legend.filter((e) => e.peak)).toHaveLength(1)
  })

  it('mounts no canvas: this is what the prerender ships', () => {
    expect(html).not.toContain('<canvas')
    expect(html).toContain('data-component="CityPlaceholder"')
  })
})

describe('the stats line', () => {
  it('counts what the city is made of', () => {
    const stats = cityStats(city)
    expect(stats.listings).toBe(8)
    expect(stats.lots).toBe(plan.lots.length)
    expect(stats.filler + stats.empty).toBe(city.fills.length)
    // The milestones are still empty lots and do not count yet.
    expect(stats.landmarks).toBe(city.landmarks.filter((l) => l.minListings === undefined).length)
    expect(cityCaption(stats)).toBe(
      `8 listings on ${stats.lots} lots, ${stats.filler} filler buildings and ` +
        `${stats.empty} empty lots, ${stats.roads} roads and ${stats.landmarks} landmarks.`
    )
  })
})
