import { renderToString } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { BuildingModelContext, useBuildingModel } from './context'
import { modelFromBlueprint } from './adapt'
import { DEMO_MODEL, FLOORS, JOURNEYS, LANES, RISERS } from './model'
import { FIXTURE_BLUEPRINT } from './fixture'

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
