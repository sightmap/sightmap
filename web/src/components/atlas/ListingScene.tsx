// The WebGL half of a listing's building. Loaded only by ListingBuilding's
// lazy boundary, so three.js never reaches the prerender or a visitor whose
// browser has no WebGL.
//
// The same tower the /building page draws, in its dollhouse cutaway, with the
// blueprint's own floors and rooms. No scroll, no chapters: the scene holds
// one pose, the viewer may swing it around Y, and the Tour button walks the
// blueprint's journeys once and stops.
import { Canvas, useFrame, useThree } from '@react-three/fiber'
import { Line } from '@react-three/drei'
import { useCallback, useEffect, useMemo, useRef, type PointerEvent as ReactPointerEvent } from 'react'
import * as THREE from 'three'
import { Walker } from '@/components/building/Agents'
import Core from '@/components/building/Core'
import Lights from '@/components/building/Lights'
import Tower from '@/components/building/Tower'
import { BuildingModelContext, useBuildingModel } from '@/components/building/context'
import { buildPath, pointAt, type Path } from '@/components/building/geometry'
import { FLOOR_H, TRAVELLER_COLORS, type BuildingModel, type Journey } from '@/components/building/model'
import { SharedStateContext, useShared, type SharedState } from '@/components/building/state'

/** How far the viewer may swing the building around Y, each way. */
export const ORBIT_LIMIT = 60
const BASE_AZ = 42
const CAMERA_DISTANCE = 42
const WALK = 2.0
const DWELL = 0.8
/** More walkers than this on a listing is noise, not information. */
const MAX_WALKS = 4

export interface ListingSceneProps {
  model: BuildingModel
  shared: SharedState
  /** The tour is running; the walkers move and the routes are drawn. */
  touring: boolean
  onReady: () => void
  /** Every walker has reached its last stop. */
  onTourEnd: () => void
}

function lookHeight(model: BuildingModel): number {
  return (model.floors.length * FLOOR_H) / 2 + 0.6
}

/** Fixed pose, plus whatever the viewer has dragged. No zoom: the building
 *  is framed to the stage and stays there. */
function Rig({ model }: { model: BuildingModel }) {
  const s = useShared()
  const { camera, size } = useThree()
  const az = useRef(BASE_AZ)
  const v = useMemo(
    () => ({ dir: new THREE.Vector3(), right: new THREE.Vector3(), up: new THREE.Vector3(), target: new THREE.Vector3() }),
    []
  )
  const lookY = lookHeight(model)
  const height = model.floors.length * FLOOR_H + 9
  useFrame((_, dt) => {
    const d = Math.min(dt, 0.25)
    const want = BASE_AZ + THREE.MathUtils.clamp(s.orbit, -ORBIT_LIMIT, ORBIT_LIMIT)
    az.current = s.reduced ? want : THREE.MathUtils.damp(az.current, want, 8, d)
    const a = THREE.MathUtils.degToRad(az.current)
    const e = THREE.MathUtils.degToRad(25)
    const zoom = Math.min(size.width / 17, size.height / height)
    v.dir.set(Math.cos(e) * Math.sin(a), Math.sin(e), Math.cos(e) * Math.cos(a))
    v.right.set(Math.cos(a), 0, -Math.sin(a))
    v.up.crossVectors(v.dir, v.right).normalize()
    v.target.set(0, lookY, 0)
    camera.position.copy(v.target).addScaledVector(v.dir, CAMERA_DISTANCE)
    camera.up.copy(v.up)
    camera.lookAt(v.target)
    const ortho = camera as THREE.OrthographicCamera
    if (Math.abs(ortho.zoom - zoom) > 1e-3) {
      ortho.zoom = zoom
      ortho.updateProjectionMatrix()
    }
  }, -90)
  return null
}

function Ready({ onReady }: { onReady: () => void }) {
  const fired = useRef(false)
  useFrame(() => {
    if (fired.current) return
    fired.current = true
    onReady()
  }, -99)
  return null
}

interface Run {
  d: number
  dwell: number
  next: number
  wait: number
  done: boolean
}

/** One walker per journey. Unlike the tour's crowd this run does not loop:
 *  each walker stops at its last room and the tour ends. */
function Tourist({
  journey,
  path,
  touring,
  onDone,
}: {
  journey: Journey
  path: Path
  touring: boolean
  onDone: () => void
}) {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
  const run = useRef<Run>({ d: 0, dwell: DWELL, next: 1, wait: journey.delay, done: false })
  const tmp = useMemo(() => new THREE.Vector3(), [])
  const reported = useRef(false)
  const color = TRAVELLER_COLORS[journey.who]

  useFrame((_, dt) => {
    const r = run.current
    if (!touring) {
      // Reset, so a second tour starts from the front door again.
      r.d = 0
      r.dwell = DWELL
      r.next = 1
      r.wait = journey.delay
      r.done = false
      reported.current = false
      if (g.current) g.current.visible = false
      return
    }
    const d = Math.min(dt, 0.2)
    if (s.reduced) {
      // Nothing moves: the route is drawn and its walker waits at the first
      // stop, so the page still answers "who walks here" without animating.
      r.d = 0
    } else if (r.wait > 0) {
      r.wait -= d
    } else if (r.dwell > 0) {
      r.dwell -= d
    } else if (r.next < path.stops.length) {
      const target = path.cum[path.stops[r.next]]
      r.d = Math.min(target, r.d + WALK * d)
      if (r.d >= target - 1e-4) {
        r.next += 1
        r.dwell = DWELL
      }
    } else if (!r.done) {
      r.done = true
    }
    if (r.done && !reported.current) {
      reported.current = true
      onDone()
    }
    if (!g.current) return
    pointAt(path, r.d, tmp)
    g.current.position.copy(tmp)
    g.current.visible = true
    g.current.scale.setScalar(1)
  })

  return <Walker color={color} group={g} trail={false} />
}

/**
 * The walks worth drawing: the first few, and only those that route. A
 * blueprint naming a room that is not on the floor it claims would otherwise
 * take the whole page down with it, and a building is not worth that.
 */
function walksOf(model: BuildingModel): { journey: Journey; path: Path }[] {
  const out: { journey: Journey; path: Path }[] = []
  for (const journey of model.journeys) {
    if (out.length >= MAX_WALKS) break
    try {
      const path = buildPath(model, journey)
      if (path.points.length >= 2) out.push({ journey, path })
    } catch {
      // Skipped, not reported: the page still has its building.
    }
  }
  return out
}

function Tour({ touring, onTourEnd }: { touring: boolean; onTourEnd: () => void }) {
  const model = useBuildingModel()
  const walks = useMemo(() => walksOf(model), [model])
  const journeys = walks.map((w) => w.journey)
  const left = useRef(journeys.length)
  const done = useCallback(() => {
    left.current -= 1
    if (left.current <= 0) onTourEnd()
  }, [onTourEnd])
  useEffect(() => {
    left.current = journeys.length
    // Nothing routes, so the tour is over before it starts and the button
    // goes back to offering it rather than sticking on "stop".
    if (touring && journeys.length === 0) onTourEnd()
  }, [touring, journeys.length, onTourEnd])
  if (journeys.length === 0) return null
  return (
    <>
      {walks.map(({ journey: j, path }, k) => (
        <group key={j.name + k}>
          <Line
            points={path.points}
            color={TRAVELLER_COLORS[j.who]}
            lineWidth={2}
            transparent
            opacity={touring ? 0.8 : 0}
            visible={touring}
            depthWrite={false}
          />
          <Tourist journey={j} path={path} touring={touring} onDone={done} />
        </group>
      ))}
    </>
  )
}

export default function ListingScene({ model, shared, touring, onReady, onTourEnd }: ListingSceneProps) {
  const drag = useRef<number | null>(null)
  const start = useRef(0)

  const onDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    drag.current = e.clientX
    start.current = shared.orbit
    e.currentTarget.setPointerCapture(e.pointerId)
  }
  const onMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (drag.current === null) return
    const dx = e.clientX - drag.current
    shared.orbit = Math.max(-ORBIT_LIMIT, Math.min(ORBIT_LIMIT, start.current + dx * 0.35))
  }
  const onUp = (e: ReactPointerEvent<HTMLDivElement>) => {
    drag.current = null
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId)
  }

  return (
    <Canvas
      orthographic
      shadows="percentage"
      flat
      gl={{ antialias: true, alpha: true, powerPreference: 'high-performance' }}
      camera={{ position: [24, 24, 24], zoom: 40, near: 0.1, far: 140 }}
      dpr={[1, 1.75]}
      style={{ position: 'absolute', inset: 0, touchAction: 'pan-y' }}
      onCreated={({ gl }) => gl.setClearColor(0x000000, 0)}
      onPointerDown={onDown}
      onPointerMove={onMove}
      onPointerUp={onUp}
      onPointerCancel={onUp}
    >
      <BuildingModelContext.Provider value={model}>
        <SharedStateContext.Provider value={shared}>
          <Ready onReady={onReady} />
          <Rig model={model} />
          <Lights />
          <Tower mode="dollhouse" />
          <Core />
          <Tour touring={touring} onTourEnd={onTourEnd} />
        </SharedStateContext.Provider>
      </BuildingModelContext.Provider>
    </Canvas>
  )
}
