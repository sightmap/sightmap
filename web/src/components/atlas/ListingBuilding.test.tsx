import { renderToString } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import ListingBuilding, { buildingCaption } from './ListingBuilding'
import { FIXTURE_BLUEPRINT } from '@/components/building/fixture'

const SCANNED = '2026-02-14T09:30:00.000Z'

describe('buildingCaption', () => {
  it('counts the floors, pages, rooms and tools the building was derived from', () => {
    expect(buildingCaption(FIXTURE_BLUEPRINT, SCANNED)).toBe(
      '2 floors for 2 pages, 5 rooms for 5 tools. Derived from the scan on February 14, 2026.'
    )
  })

  it('reads for a lobby: one floor, one page, no rooms', () => {
    const lobby = {
      ...FIXTURE_BLUEPRINT,
      floors: [{ name: 'fixture.example', route: '/', rooms: [], lanes: [] }],
      stats: { ...FIXTURE_BLUEPRINT.stats, pages: 1, tools: 0 },
    }
    expect(buildingCaption(lobby, SCANNED)).toBe(
      '1 floor for 1 page, 0 rooms for 0 tools. Derived from the scan on February 14, 2026.'
    )
  })
})

describe('the prerendered placeholder', () => {
  const html = renderToString(<ListingBuilding blueprint={FIXTURE_BLUEPRINT} scannedAt={SCANNED} />)

  it('carries the heading and the stats line', () => {
    expect(html).toContain('The building')
    expect(html).toContain('2 floors for 2 pages, 5 rooms for 5 tools.')
  })

  it('draws the building as static SVG', () => {
    expect(html).toContain('data-component="BuildingPoster"')
    expect(html).toContain('<svg')
  })

  it('mounts no canvas and offers no tour before the scene loads', () => {
    expect(html).not.toContain('<canvas')
    expect(html).not.toContain('atlas-building__tour')
  })
})
