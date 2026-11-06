import { renderToString } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { BuildingModelContext, useBuildingModel } from './context'
import { modelFromBlueprint } from './adapt'
import { DEMO_MODEL, FLOORS, FLOOR_H, JOURNEYS, LANES, RISERS, SLAB_T, floorHeight, floorY, surfaceAt } from './model'
import {
  PALETTES,
  ROOF_STYLES,
  groundFloorParts,
  paletteColor,
  roofParts,
  rooftopItem,
  shellHeight,
  windowGrid,
  windowMullions,
} from './facade'

const ROOFTOP_KINDS_SEEN = ['tank', 'antenna', 'solar', 'garden']
import { FIXTURE_BLUEPRINT } from './fixture'
import type { Archetype } from '@/types/blueprint'

/** Reports whatever building it is given, so the default can be inspected. */
function Probe() {
  const m = useBuildingModel()
  return (
    <span>
      {[m.floors.length, m.lanes.length, m.journeys.length, m.risers.length, m.floors[0]?.name].join('|')}
    </span>
  )
}

describe('BuildingModelContext', () => {
  it('defaults to the demo constants', () => {
    expect(DEMO_MODEL.floors).toBe(FLOORS)
    expect(DEMO_MODEL.lanes).toBe(LANES)
    expect(DEMO_MODEL.journeys).toBe(JOURNEYS)
    expect(DEMO_MODEL.risers).toBe(RISERS)
    const html = renderToString(<Probe />)
    expect(html).toContain(`${FLOORS.length}|${LANES.length}|${JOURNEYS.length}|${RISERS.length}|${FLOORS[0].name}`)
  })

  it('hands a provided building to the components below it', () => {
    const model = modelFromBlueprint(FIXTURE_BLUEPRINT)
    const html = renderToString(
      <BuildingModelContext.Provider value={model}>
        <Probe />
      </BuildingModelContext.Provider>
    )
    expect(html).toContain('2|2|2|0|Home')
  })
})

describe('modelFromBlueprint', () => {
  const model = modelFromBlueprint(FIXTURE_BLUEPRINT)

  it('maps a page to a floor and a tool to a room', () => {
    expect(model.floors.map((f) => [f.name, f.route])).toEqual([
      ['Home', '/'],
      ['Checkout', '/checkout'],
    ])
    expect(model.floors[0].rooms.map((r) => r.name)).toEqual([
      'browse_catalog',
      'search_products',
      'open_help',
    ])
    const room = model.floors[1].rooms[0]
    expect(room).toEqual({ name: 'add_to_cart', x: -1.8, z: -2.0, w: 2.65, d: 1.95, h: 1.0, kind: 'action' })
  })

  it('carries each floor its own lanes, in floor order', () => {
    expect(model.lanes).toHaveLength(model.floors.length)
    expect(model.lanes[0][0][0]).toEqual([-2.65, -2.5])
    expect(model.lanes[1][0][1]).toEqual([-2.2, -2.5])
  })

  it('turns walks into journeys and leaves the risers empty', () => {
    expect(model.journeys).toEqual([
      {
        name: 'Find and buy a product',
        who: 'agent',
        stops: [
          [0, 'search_products'],
          [0, 'browse_catalog'],
          [1, 'add_to_cart'],
        ],
        delay: 0,
      },
      {
        name: 'Check an order',
        who: 'test',
        stops: [
          [1, 'read_order'],
          [0, 'open_help'],
        ],
        delay: 2,
      },
    ])
    expect(model.risers).toEqual([])
  })

  it('every journey stop resolves to a room on the floor it names', () => {
    for (const journey of model.journeys) {
      for (const [floor, name] of journey.stops) {
        expect(model.floors[floor].rooms.some((r) => r.name === name), `${name} on F${floor}`).toBe(true)
      }
    }
  })

  it('carries the facade and the seed through', () => {
    expect(model.facade).toEqual(FIXTURE_BLUEPRINT.facade)
    expect(model.seed).toBe(FIXTURE_BLUEPRINT.seed)
    // A copy, so mutating the model cannot write back into the blueprint.
    expect(model.facade).not.toBe(FIXTURE_BLUEPRINT.facade)
  })
})

describe('storey height', () => {
  it('leaves the demo building at the height it was drawn at', () => {
    expect(DEMO_MODEL.floorH).toBeUndefined()
    expect(floorHeight(DEMO_MODEL)).toBe(FLOOR_H)
    expect(floorY(0)).toBe(0)
    expect(floorY(2)).toBe(2 * FLOOR_H)
    expect(floorY(2, floorHeight(DEMO_MODEL))).toBe(2 * FLOOR_H)
  })

  it("stacks a blueprint's floors at whatever height it is given", () => {
    const tall = modelFromBlueprint(FIXTURE_BLUEPRINT, { floorH: 6.6 })
    expect(tall.floorH).toBe(6.6)
    expect(floorY(2, floorHeight(tall))).toBe(2 * 6.6)
    // Deck heights follow the storey, so walkers and labels climb with it.
    const room = tall.floors[1].rooms[0]
    expect(surfaceAt(tall, 1, room.x, room.z)).toBeCloseTo(6.6 + SLAB_T + 0.07, 5)
  })

  it('reaches the height the closed shell on a city lot drew', () => {
    // The city draws a listing's shell at its lot's storeys, not its floors.
    // Opening it must not move the roofline, so spending those storeys over
    // the floors the listing has puts the top slab back where it was.
    for (const [floors, storeys] of [
      [1, 6],
      [2, 4],
      [2, 9],
      [3, 4],
    ]) {
      const floorH = (storeys / floors) * FLOOR_H
      expect(floorY(floors, floorH) + SLAB_T, `${floors}/${storeys}`).toBeCloseTo(shellHeight(storeys), 10)
    }
  })

  it('leaves a blueprint at the default height when none is given', () => {
    const plain = modelFromBlueprint(FIXTURE_BLUEPRINT)
    expect(plain.floorH).toBeUndefined()
    expect(floorHeight(plain)).toBe(FLOOR_H)
  })
})

describe('closed-mode facade', () => {
  it('builds parts for every roof style', () => {
    expect(ROOF_STYLES).toHaveLength(7)
    const shapes = new Set<string>()
    for (const style of ROOF_STYLES) {
      const parts = roofParts(style)
      expect(parts.length, style).toBeGreaterThan(0)
      // Every roof closes the box with a deck before it does anything else.
      expect(parts[0].kind, style).toBe('slab')
      for (const part of parts) expect(Number.isFinite(part.y), `${style} ${part.kind}`).toBe(true)
      shapes.add(parts.map((p) => p.kind).join('+'))
    }
    // Seven styles, seven silhouettes — no two share a part list.
    expect(shapes.size).toBe(7)
  })

  it('gives every archetype four palette entries', () => {
    const archetypes = Object.keys(PALETTES) as Archetype[]
    expect(archetypes).toHaveLength(8)
    for (const a of archetypes) {
      expect(PALETTES[a], a).toHaveLength(4)
      expect(paletteColor(a, 0)).toBe(PALETTES[a][0])
      expect(paletteColor(a, 3)).toBe(PALETTES[a][3])
      // A palette index out of range wraps rather than drawing an undefined.
      expect(paletteColor(a, 9)).toBe(PALETTES[a][1])
    }
  })

  it('seeds the window grid so the same building is always the same', () => {
    const a = windowGrid(3, 1234)
    const b = windowGrid(3, 1234)
    const c = windowGrid(3, 5678)
    expect(a).toEqual(b)
    expect(a.length).toBeGreaterThan(0)
    expect(a.map((w) => w.lit).join('')).not.toBe(c.map((w) => w.lit).join(''))
    // Every window sits on a wall of the building it belongs to.
    for (const w of a) expect(w.y).toBeGreaterThan(0)
  })
})

describe('closed-mode street level', () => {
  it('gives every archetype a ground floor and a rooftop item', () => {
    const archetypes = Object.keys(PALETTES) as Archetype[]
    for (const a of archetypes) {
      for (const v of [0, 1, 2]) {
        const street = groundFloorParts(a, v)
        expect(street.length, `${a}/${v}`).toBeGreaterThan(1)
        for (const part of street) {
          // Everything stands on the pavement and under the second storey's
          // sill, bar a canopy or a marquee reaching over the street.
          expect(part.y, `${a}/${v} ${part.kind}`).toBeGreaterThan(-0.1)
          expect(part.y, `${a}/${v} ${part.kind}`).toBeLessThan(3.2)
        }
        const roof = rooftopItem(a, v)
        expect(ROOFTOP_KINDS_SEEN).toContain(roof.kind)
        expect(roof.parts.length, `${a}/${v} ${roof.kind}`).toBeGreaterThan(0)
      }
    }
  })

  it('moves the ground floor and the roof with the variant', () => {
    for (const a of Object.keys(PALETTES) as Archetype[]) {
      const looks = [0, 1, 2].map((v) => JSON.stringify(groundFloorParts(a, v)))
      expect(new Set(looks).size, a).toBeGreaterThan(1)
      const tops = [0, 1, 2].map((v) => rooftopItem(a, v).kind)
      expect(new Set(tops).size, a).toBe(3)
    }
  })

  it('frames the windows the mask left, and only those', () => {
    const windows = windowGrid(3, 4242)
    const parts = windowMullions(windows)
    expect(parts.length).toBeGreaterThan(windows.length)
    // A mullion belongs to a storey a window is on, never to an empty wall.
    const storeys = new Set(windows.map((w) => w.y.toFixed(3)))
    for (const p of parts) {
      const y = p.kind === 'box' ? p.y : p.y
      expect([...storeys].some((s) => Math.abs(Number(s) - y) < 0.6)).toBe(true)
    }
    // No windows, no frames.
    expect(windowMullions([])).toEqual([])
  })
})
