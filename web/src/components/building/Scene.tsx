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
import { MobileTierContext, SharedStateContext, useMobileTier, useShared, type SharedState } from './state'
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

/**
 * Exposes renderer counters for the headless verification loop.
 *
 * Published on a timer rather than from `useFrame`, because under
 * `frameloop="demand"` an idle scene runs no frames at all — and a probe that
 * only ever hears from `useFrame` would read the last *drawn* frame's counters
 * and conclude the idle scene was still submitting that much work. A sample
 * window with no frames in it reports zeroes, which is what actually happened.
 */
export function Stats() {
  const { gl, scene } = useThree()
  const s = useShared()
  const mobile = useMobileTier()
  const frames = useRef(0)
  const last = useRef({ calls: 0, triangles: 0 })
  useFrame(() => {
    frames.current++
    last.current.calls = gl.info.render.calls
    last.current.triangles = gl.info.render.triangles
  })
  useEffect(() => {
    let t0 = performance.now()
    const publish = () => {
      const now = performance.now()
      const drawn = frames.current
      // The shadow map size is read off the light itself rather than
      // recomputed here, so the probe sees what the renderer actually has.
      let shadowMapSize = 0
      scene.traverse((o) => {
        const l = o as THREE.DirectionalLight
        if (l.isDirectionalLight && l.castShadow) shadowMapSize = l.shadow.mapSize.x
      })
      const w = window as unknown as { __bldStats?: Record<string, number> }
      w.__bldStats = {
        fps: Math.round((drawn * 1000) / Math.max(now - t0, 1)),
        framesDrawn: drawn,
        calls: drawn ? last.current.calls : 0,
        triangles: drawn ? last.current.triangles : 0,
        geometries: gl.info.memory.geometries,
        textures: gl.info.memory.textures,
        shadows: gl.shadowMap.enabled ? 1 : 0,
        people: crowd.drawn,
        shadowMapSize,
        dpr: gl.getPixelRatio(),
        walkers: s.walkers,
        mobile: mobile ? 1 : 0,
      }
      frames.current = 0
      t0 = now
    }
    const id = window.setInterval(publish, 1000)
    return () => window.clearInterval(id)
  }, [gl, scene, s, mobile])
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
  /** The quality tier, as state, so a resize past the boundary re-renders it. */
  mobile: boolean
  /**
   * How hard the tour is allowed to run. 'always' while the stage is on screen
   * and the tab is visible; 'never' when it is not; 'demand' under reduced
   * motion, where the scene settles to a still and only redraws when the scroll
   * or a resize asks it to.
   */
  frameloop: 'always' | 'never' | 'demand'
  onReady: () => void
}

export default function Scene({ shared, mobile, frameloop, onReady }: SceneProps) {
  return (
    <Canvas
      {...CANVAS_PROPS}
      dpr={mobile ? [1, 1.5] : [1, 1.75]}
      frameloop={frameloop}
      onCreated={({ gl }) => gl.setClearColor(0x000000, 0)}
    >
      {/* The canvas is its own reconciler, so both contexts are re-provided here. */}
      <SharedStateContext.Provider value={shared}>
        <MobileTierContext.Provider value={mobile}>
          <Updater />
          <Ready onReady={onReady} />
          <Stats />
          <Rig />
          <SceneContent />
        </MobileTierContext.Provider>
      </SharedStateContext.Provider>
    </Canvas>
  )
}
