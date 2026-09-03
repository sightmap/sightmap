// The WebGL scene. Loaded lazily by BuildingExperience, client-only.
//
// One orthographic camera at an isometric-ish angle looks at an architect's
// model on a drafting table. Scroll position (shared.progress) picks a blend
// of two neighbouring chapters' parameters; the updater damps toward it every
// frame and every other component reads the damped values, so nothing here
// re-renders on scroll.
import { Canvas, useFrame, useThree } from '@react-three/fiber'
import { useEffect, useMemo, useRef } from 'react'
import * as THREE from 'three'
import { PARAM_KEYS, paramsAt } from './chapters'
import { SharedStateContext, useShared, type SharedState } from './state'
import Lights from './Lights'
import Table from './Table'
import Tower from './Tower'
import Core from './Core'
import Wayfinding from './Wayfinding'
import People, { crowd } from './People'
import HealDemo from './HealDemo'
import Trajectory from './Trajectory'
import FrontDesk from './FrontDesk'

const CAMERA_DISTANCE = 42

/** Calls `onReady` after the first rendered frame, so the poster can fade. */
export function Ready({ onReady }: { onReady: () => void }) {
  const fired = useRef(false)
  useFrame(() => {
    if (!fired.current) {
      fired.current = true
      onReady()
    }
  }, -99)
  return null
}

/** The tour's driver: scroll position → chapter blend → damped parameters. */
function Updater() {
  const s = useShared()
  useEffect(() => {
    // Start on the chapter the page loaded at, not on a morph from chapter 0.
    paramsAt(s.progress, s.cur)
  }, [s])
  useFrame((_, dt) => {
    paramsAt(s.progress, s.target)
    const d = Math.min(dt, 0.25)
    const lambda = s.reduced ? 16 : 4.5
    for (const k of PARAM_KEYS) s.cur[k] = THREE.MathUtils.damp(s.cur[k], s.target[k], lambda, d)
  }, -100)
  return null
}

export function Stats() {
  // Exposes draw calls / triangles / fps for the headless verification loop.
  const { gl } = useThree()
  const frames = useRef(0)
  const t0 = useRef(performance.now())
  useFrame(() => {
    frames.current++
    const now = performance.now()
    if (now - t0.current > 1000) {
      const w = window as unknown as { __bldStats?: Record<string, number> }
      w.__bldStats = {
        fps: Math.round((frames.current * 1000) / (now - t0.current)),
        calls: gl.info.render.calls,
        triangles: gl.info.render.triangles,
        geometries: gl.info.memory.geometries,
        textures: gl.info.memory.textures,
        shadows: gl.shadowMap.enabled ? 1 : 0,
        people: crowd.drawn,
      }
      frames.current = 0
      t0.current = now
    }
  })
  return null
}

export function Rig() {
  const s = useShared()
  const { camera, size } = useThree()
  const az = useRef(s.cur.az)
  const el = useRef(s.cur.el)
  const v = useMemo(
    () => ({
      dir: new THREE.Vector3(),
      right: new THREE.Vector3(),
      up: new THREE.Vector3(),
      target: new THREE.Vector3(),
    }),
    []
  )
  useFrame((_, dt) => {
    const c = s.cur
    const d = Math.min(dt, 0.25)
    const drift = s.reduced ? 0 : Math.sin(performance.now() * 0.00018) * 1.4
    const tAz = c.az + drift + s.pointer.x * 3
    const tEl = c.el - s.pointer.y * 2
    az.current = THREE.MathUtils.damp(az.current, tAz, 3, d)
    el.current = THREE.MathUtils.damp(el.current, tEl, 3, d)
    const a = THREE.MathUtils.degToRad(az.current)
    const e = THREE.MathUtils.degToRad(el.current)

    // Frame the model: the table's isometric footprint is ~27 world units
    // wide and, with the tower on it, ~18 tall, whatever the viewport.
    // The billboard crops in tight on the tower instead.
    const billboard = s.frame === 'billboard'
    const fit = billboard ? Math.min(size.width / 15, size.height / 12.5) : Math.min(size.width / 27, size.height / 22)
    const zoom = fit * c.zoom * (s.mobile && !billboard ? 1.3 : 1)

    v.dir.set(Math.cos(e) * Math.sin(a), Math.sin(e), Math.cos(e) * Math.cos(a))
    v.right.set(Math.cos(a), 0, -Math.sin(a))
    v.up.crossVectors(v.dir, v.right).normalize()

    // Desktop: the story card sits on the left, so push the model right.
    // Mobile: the card sits at the bottom, so push the model up.
    // Billboard: nudge the tower left so the floor directory fits in the frame.
    const shiftRight = billboard ? (-size.width * 0.07) / zoom : s.mobile ? (-size.width * 0.12) / zoom : (size.width * 0.13) / zoom
    const shiftUp = billboard || !s.mobile ? 0 : (size.height * 0.11) / zoom
    v.target.set(0, c.lookY, 0).addScaledVector(v.right, -shiftRight).addScaledVector(v.up, -shiftUp)

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

/** Everything on the table. Reads the shared state; needs a driver and a Rig. */
export function SceneContent() {
  return (
    <>
      <Lights />
      <Table />
      <Tower />
      <Core />
      <Wayfinding />
      <People />
      <HealDemo />
      <Trajectory />
      <FrontDesk />
    </>
  )
}

export const CANVAS_PROPS = {
  orthographic: true,
  shadows: 'percentage',
  flat: true,
  gl: { antialias: true, alpha: true, powerPreference: 'high-performance' },
  camera: { position: [24, 24, 24], zoom: 50, near: 0.1, far: 140 },
  style: { position: 'absolute', inset: 0 },
} satisfies Partial<React.ComponentProps<typeof Canvas>>

export interface SceneProps {
  shared: SharedState
  onReady: () => void
}

export default function Scene({ shared, onReady }: SceneProps) {
  return (
    <Canvas
      {...CANVAS_PROPS}
      dpr={shared.mobile ? [1, 1.5] : [1, 1.75]}
      onCreated={({ gl }) => gl.setClearColor(0x000000, 0)}
    >
      <SharedStateContext.Provider value={shared}>
        <Updater />
        <Ready onReady={onReady} />
        <Stats />
        <Rig />
        <SceneContent />
      </SharedStateContext.Provider>
    </Canvas>
  )
}
