// Blueprint → building model. The blueprint is pure data derived from a
// listing's scan (src/types/blueprint.ts); this is the one place that turns
// it into the shape the scene components draw. No three.js on this path, so
// the prerender can call it.
import type { Blueprint } from '@/types/blueprint'
import type { BuildingModel } from './model'

export function modelFromBlueprint(blueprint: Blueprint): BuildingModel {
  return {
    floors: blueprint.floors.map((floor) => ({
      name: floor.name,
      route: floor.route,
      rooms: floor.rooms.map((room) => ({
        name: room.name,
        x: room.x,
        z: room.z,
        w: room.w,
        d: room.d,
        h: room.h,
        kind: room.kind,
      })),
    })),
    lanes: blueprint.floors.map((floor) => floor.lanes),
    // A walk is a journey; the blueprint only ever records agents and tests,
    // since nothing here has watched a real user.
    journeys: blueprint.walks.map((walk) => ({
      name: walk.name,
      who: walk.who,
      stops: walk.stops,
      delay: walk.delay,
    })),
    // A scan sees pages and tools, never the requests behind them, so a
    // derived building has no risers and the core draws the shaft alone.
    risers: [],
    facade: { ...blueprint.facade },
    seed: blueprint.seed,
  }
}
