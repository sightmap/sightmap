// A listing's building on its own page. Nothing on this module's import path
// touches three.js: the scene is behind React.lazy, so the prerender, a
// crawler and a browser without WebGL all get the static drawing and the
// stats line instead, and the WebGL chunk is only fetched once a visitor with
// a working canvas has the section in front of them.
import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from 'react'
import type { Blueprint } from '@/types/blueprint'
import { modelFromBlueprint } from '@/components/building/adapt'
import { CHAPTERS } from '@/components/building/chapters'
import { BuildingModelContext } from '@/components/building/context'
import { FLOOR_H } from '@/components/building/model'
import Poster from '@/components/building/Poster'
import { createSharedState } from '@/components/building/state'
import { webglAvailable } from '@/components/building/webgl'
import { formatScanned } from '@/lib/directory'

const ListingScene = lazy(() => import('./ListingScene'))

const plural = (n: number, word: string): string => `${n} ${word}${n === 1 ? '' : 's'}`

/** The one line under the heading: what the building is made of, and when. */
export function buildingCaption(blueprint: Blueprint, scannedAt: string): string {
  const rooms = blueprint.floors.reduce((n, f) => n + f.rooms.length, 0)
  return (
    `${plural(blueprint.floors.length, 'floor')} for ${plural(blueprint.stats.pages, 'page')}, ` +
    `${plural(rooms, 'room')} for ${plural(blueprint.stats.tools, 'tool')}. ` +
    `Derived from the scan on ${formatScanned(scannedAt)}.`
  )
}

export interface ListingBuildingProps {
  blueprint: Blueprint
  /** The scan the building was derived from. */
  scannedAt: string
}

export default function ListingBuilding({ blueprint, scannedAt }: ListingBuildingProps) {
  const model = useMemo(() => modelFromBlueprint(blueprint), [blueprint])
  const shared = useMemo(() => {
    const s = createSharedState()
    // One fixed pose: the building stands, its walls are up, and nothing
    // scrolls. The chapter is the one the tour calls "the building".
    const built = { ...CHAPTERS[2].scene, labels: 0, tags: 0, agents: 0 }
    Object.assign(s.cur, built)
    Object.assign(s.target, built)
    return s
  }, [])

  const [mounted, setMounted] = useState(false)
  const [webgl, setWebgl] = useState(false)
  const [ready, setReady] = useState(false)
  const [touring, setTouring] = useState(false)
  const [reduced, setReduced] = useState(false)
  const [walk, setWalk] = useState<string | null>(null)

  useEffect(() => {
    setMounted(true)
    setWebgl(webglAvailable())
    const motion = window.matchMedia('(prefers-reduced-motion: reduce)')
    shared.reduced = motion.matches
    shared.mobile = window.innerWidth < 900
    setReduced(motion.matches)
  }, [shared])

  const onReady = useCallback(() => setReady(true), [])
  const onTourEnd = useCallback(() => setTouring(false), [])
  const onWalkChange = useCallback((name: string | null) => setWalk(name), [])

  const walks = model.journeys.length
  // The drawing is isometric, so a taller building needs a taller stage. The
  // stage is also what sets the zoom, so it is generous: a building whose
  // rooms carry tool names has to be big enough to read them.
  const stage = Math.round(258 + model.floors.length * FLOOR_H * 46)

  return (
    <BuildingModelContext.Provider value={model}>
      <section
        className="atlas-building"
        data-component="ListingBuilding"
        aria-labelledby="atlas-building-h"
      >
        <h2 id="atlas-building-h" className="atlas-listing__h2">
          The building
        </h2>
        <p className="atlas-listing__note atlas-building__stats">{buildingCaption(blueprint, scannedAt)}</p>

        <div className="atlas-building__stage" style={{ height: `${stage}px` }} data-ready={ready ? 'true' : 'false'}>
          <Poster hidden={ready} bare className="atlas-building__poster" />
          {mounted && webgl && (
            <Suspense fallback={null}>
              <ListingScene
                model={model}
                shared={shared}
                touring={touring}
                onReady={onReady}
                onTourEnd={onTourEnd}
                onWalkChange={onWalkChange}
              />
            </Suspense>
          )}
          {mounted && webgl && walks > 0 && (
            <div className="atlas-building__controls">
              <button
                type="button"
                className="atlas-building__tour"
                onClick={() => setTouring((t) => !t)}
                aria-label={touring ? 'Stop the tour' : 'Start the tour'}
              >
                {/* Reduced motion gets the routes drawn and the walkers
                    parked, so the button says what it will actually do.
                    While the tour runs it names the walk under way, since
                    that is the one piece of the tour the drawing cannot
                    say by itself. */}
                {reduced
                  ? touring
                    ? 'Hide the routes'
                    : 'Show the routes'
                  : touring
                    ? (walk ?? 'Stop the tour')
                    : 'Tour'}
              </button>
            </div>
          )}
        </div>

        <p className="atlas-building__note">
          Floors are the pages the scan reached, rooms are the tools it found on them, and a walk is
          one of the journeys those tools would support. Nothing here has been run.
        </p>
      </section>
    </BuildingModelContext.Provider>
  )
}
