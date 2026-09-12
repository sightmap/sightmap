// Level two: the building the camera flew down to, opened up. The closed
// instance on that lot steps aside (CityBuildings hides it) and the same
// parametric tower the listing page draws stands in its place, in the lot's
// own frame — so the dollhouse is not a different building, it is this one
// with its front wall off.
//
// The shell drew the lot's storeys, not the listing's floors: a three-floor
// listing on a central lot is a nine-storey shell. The tower is given a
// storey height that spends those storeys over the floors it has, so the
// building keeps the height its neighbours knew it by instead of collapsing
// to a third of it the moment it is opened.
import { useEffect, useMemo } from 'react'
import * as THREE from 'three'
import type { Blueprint } from '@/types/blueprint'
import type { CityLot } from '@/types/city'
import Core from '@/components/building/Core'
import Tower from '@/components/building/Tower'
import { modelFromBlueprint } from '@/components/building/adapt'
import { CHAPTERS } from '@/components/building/chapters'
import { BuildingModelContext } from '@/components/building/context'
import { FLOOR_H } from '@/components/building/model'
import { SharedStateContext, createSharedState } from '@/components/building/state'

export interface CityFocusProps {
  lot: CityLot
  blueprint: Blueprint
  /** Storeys the closed shell on this lot drew. */
  storeys: number
  night: boolean
  reduced: boolean
}

export default function CityFocus({ lot, blueprint, storeys, night, reduced }: CityFocusProps) {
  const model = useMemo(() => {
    const floors = Math.max(1, blueprint.floors.length)
    return modelFromBlueprint(blueprint, {
      floorH: (Math.max(floors, storeys) / floors) * FLOOR_H,
    })
  }, [blueprint, storeys])
  const shared = useMemo(() => {
    const s = createSharedState()
    // One fixed pose, the chapter the tour calls "the building": walls up,
    // no labels, no crowd, nothing scrolling.
    const built = { ...CHAPTERS[2].scene, labels: 0, tags: 0, agents: 0 }
    Object.assign(s.cur, built)
    Object.assign(s.target, built)
    return s
  }, [])

  useEffect(() => {
    shared.reduced = reduced
    shared.cur.night = night ? 1 : 0
    shared.target.night = night ? 1 : 0
  }, [shared, night, reduced])

  return (
    <group position={[lot.x, 0, lot.z]} rotation={new THREE.Euler(0, lot.rotation, 0)}>
      <BuildingModelContext.Provider value={model}>
        <SharedStateContext.Provider value={shared}>
          <Tower mode="dollhouse" />
          <Core />
        </SharedStateContext.Provider>
      </BuildingModelContext.Provider>
    </group>
  )
}
