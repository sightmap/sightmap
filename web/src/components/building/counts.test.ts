import { describe, expect, it } from 'vitest'
import { CHAPTERS } from './chapters'
import { COUNTS, FLOORS, JOURNEYS, RISERS } from './model'

describe('COUNTS', () => {
  it('derives every summary stat from the corpus with no hardcoded offsets', () => {
    expect(COUNTS).toEqual({
      views: FLOORS.length,
      components: FLOORS.reduce((n, f) => n + f.rooms.length, 0),
      requests: RISERS.length,
      memory: FLOORS.reduce((n, f) => n + f.rooms.filter((r) => r.memory).length, 0),
      journeys: JOURNEYS.length,
    })
  })
})

describe('wayfinding HUD', () => {
  it('advertises the count of memory notes the scene actually renders', () => {
    const wayfinding = CHAPTERS.find((c) => c.id === 'wayfinding')
    const renderedNotes = FLOORS.reduce(
      (n, f) => n + f.rooms.filter((r) => r.memory).length,
      0,
    )
    expect(wayfinding?.hud?.rows).toContainEqual(['Memory notes', String(COUNTS.memory)])
    expect(String(COUNTS.memory)).toBe(String(renderedNotes))
  })
})
