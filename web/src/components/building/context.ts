// Which building the components in this directory are drawing.
//
// The /building page draws the demo corpus and provides nothing, so it gets
// `DEMO_MODEL` — the same constants the components used to import directly.
// A listing's building provides its own model, adapted from its blueprint.
import { createContext, useContext } from 'react'
import { DEMO_MODEL, type BuildingModel } from './model'

export const BuildingModelContext = createContext<BuildingModel>(DEMO_MODEL)

export function useBuildingModel(): BuildingModel {
  return useContext(BuildingModelContext)
}
