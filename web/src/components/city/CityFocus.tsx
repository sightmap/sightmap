// Level two: the building the camera flew down to, opened up. The closed
// instance on that lot steps aside (CityBuildings hides it) and the same
// parametric tower the listing page draws stands in its place, in the lot's
// own frame — so the dollhouse is not a different building, it is this one
// with its front wall off.
import { useEffect, useMemo } from 'react'
import * as THREE from 'three'
import type { Blueprint } from '@/types/blueprint'
import type { CityLot } from '@/types/city'
import Core from '@/components/building/Core'
import Tower from '@/components/building/Tower'
import { modelFromBlueprint } from '@/components/building/adapt'
import { CHAPTERS } from '@/components/building/chapters'
import { BuildingModelContext } from '@/components/building/context'
import { SharedStateContext, createSharedState } from '@/components/building/state'

export interface CityFocusProps {
  lot: CityLot
  blueprint: Blueprint
  night: boolean
  reduced: boolean
}

export default function CityFocus({ lot, blueprint, night, reduced }: CityFocusProps) {
  const model = useMemo(() => modelFromBlueprint(blueprint), [blueprint])
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
