import { describe, expect, it } from 'vitest'
import type { Archetype } from '@/types/blueprint'
import { personaFurnish } from './persona'
import { FIXTURE_BLUEPRINT } from './fixture'
import { modelFromBlueprint } from './adapt'
import { DEMO_MODEL, FLOOR_D, FLOOR_W, FLOOR_H, type Floor } from './model'
import type { ItemType } from './furnish'

const ARCHETYPES: Archetype[] = [
  'office',
  'storefront',
  'bank',
  'workshop',
  'theatre',
  'terminal',
  'clinic',
  'library',
]

const ground = (): Floor => ({
  name: 'Home',
  route: '/',
  rooms: [
    { name: 'read_catalog', x: -2.4, z: -2.2, w: 2.6, d: 1.9, h: 0.5, kind: 'content' },
    { name: 'search', x: 0.9, z: -2.2, w: 2.3, d: 1.7, h: 0.75, kind: 'form' },
    { name: 'get_order', x: 3.4, z: -0.4, w: 2.3, d: 1.7, h: 0.6, kind: 'data' },
    { name: 'buy', x: -2.4, z: 0.6, w: 1.6, d: 1.2, h: 1.0, kind: 'action' },
  ],
})

describe('personaFurnish', () => {
  it('gives every archetype but office something on the ground floor', () => {
    for (const a of ARCHETYPES) {
      const items = personaFurnish(a, 0, ground(), 0, 'seed')
      if (a === 'office') expect(items, a).toHaveLength(0)
      else expect(items.length, a).toBeGreaterThan(8)
    }
  })

  it('is deterministic for the same building', () => {
    for (const a of ARCHETYPES) {
      expect(personaFurnish(a, 1, ground(), 0, 's')).toEqual(personaFurnish(a, 1, ground(), 0, 's'))
    }
  })

  it('moves at least one piece per archetype when the variant moves', () => {
    for (const a of ARCHETYPES) {
      if (a === 'office') continue
      const looks = [0, 1, 2].map((v) => JSON.stringify(personaFurnish(a, v, ground(), 0, 's')))
      expect(new Set(looks).size, a).toBeGreaterThan(1)
    }
  })

  it('keeps every piece inside the floor plate and under the ceiling', () => {
    for (const a of ARCHETYPES) {
      for (const v of [0, 1, 2]) {
        for (const item of personaFurnish(a, v, ground(), 0, 's')) {
          // Canopies, and the lamps hung under them, may overhang the open
          // face; nothing else may leave the plate.
          const overhangs = item.type === 'awning' || item.type === 'screen'
          const slack = overhangs ? 2.2 : 0.4
          expect(Math.abs(item.x), `${a}/${v} ${item.type} x`).toBeLessThanOrEqual(FLOOR_W / 2 + slack)
          expect(Math.abs(item.z), `${a}/${v} ${item.type} z`).toBeLessThanOrEqual(FLOOR_D / 2 + slack)
          expect(item.y, `${a}/${v} ${item.type} y`).toBeGreaterThan(-0.2)
          expect(item.y + item.sy / 2, `${a}/${v} ${item.type} top`).toBeLessThanOrEqual(FLOOR_H + 0.6)
        }
      }
    }
  })

  it('dresses the upper floors without repeating the street', () => {
    // A first floor gets the room treatment and none of the entrance.
    const upper = personaFurnish('library', 2, ground(), 1, 's')
    const all = personaFurnish('library', 2, ground(), 0, 's')
    expect(upper.length).toBeGreaterThan(0)
    expect(upper.length).toBeLessThan(all.length)
  })

  it('falls back to the biggest rooms when the kind it wants is absent', () => {
    const noContent: Floor = { ...ground(), rooms: ground().rooms.filter((r) => r.kind !== 'content') }
    const stacks = (f: Floor) =>
      personaFurnish('library', 0, f, 1, 's').filter((i) => i.type === ('shelf' satisfies ItemType)).length
    expect(stacks(noContent)).toBeGreaterThan(0)
  })

  it('leaves the demo building alone', () => {
    // The /building page's model is not derived, so nothing here applies to
    // it; the guard lives in Tower, and this is the contract it relies on.
    expect(DEMO_MODEL.derived).toBeUndefined()
    expect(modelFromBlueprint(FIXTURE_BLUEPRINT).derived).toBe(true)
  })
})
