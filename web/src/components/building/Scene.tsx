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
import { computeFraming, makeFraming } from './framing'
import { MobileTierContext, SharedStateContext, useMobileTier, useShared, type SharedState } from './state'
import { publishCaptureProgress } from './capture'
import { createTierSampler, type TierWindow } from './tier'
import Lights from './Lights'
import BakedEnvironment from './Environment'
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
  const frames = useRef(0)
  useEffect(() => {
    // Start on the chapter the page loaded at, not on a morph from chapter 0.
    paramsAt(s.progress, s.cur)
  }, [s])
  useFrame((_, dt) => {
    paramsAt(s.progress, s.target)
    const d = Math.min(dt, 0.25)
    const lambda = s.reduced ? 16 : 4.5
    let delta = 0
    for (const k of PARAM_KEYS) {
      s.cur[k] = THREE.MathUtils.damp(s.cur[k], s.target[k], lambda, d)
      if (s.capture) delta = Math.max(delta, Math.abs(s.target[k] - s.cur[k]))
    }
    // How far the scene still has to travel, for the capture harness to wait
    // on. Only under capture: a real session pays nothing for this.
    if (s.capture) publishCaptureProgress({ delta, frames: ++frames.current, progress: s.progress })
    // The overlay is the common DOM ancestor every portalled label resolves
    // into (see .bld-overlay in building.css), so publishing the damped
    // night value here — once — lets any label's CSS read --bld-night and
    // cross-fade colour with the same continuous curve Lights and Table
    // already lerp their materials against, instead of re-deriving it per
    // component or snapping on the page's discrete data-night boundary.
    s.overlay.current?.style.setProperty('--bld-night', s.cur.night.toFixed(3))
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
  const framing = useMemo(makeFraming, [])
  useFrame((_, dt) => {
    const c = s.cur
    const d = Math.min(dt, 0.25)
    // Capture mode freezes the drift: an unattended still has to be the same
    // image every run, and this term never repeats.
    const drift = s.reduced || s.capture ? 0 : Math.sin(performance.now() * 0.00018) * 1.4
    const tAz = c.az + drift + s.pointer.x * 3
    const tEl = c.el - s.pointer.y * 2
    az.current = THREE.MathUtils.damp(az.current, tAz, 3, d)
    el.current = THREE.MathUtils.damp(el.current, tEl, 3, d)

    computeFraming(
      {
        az: az.current,
        el: el.current,
        chapterZoom: c.zoom,
        lookY: c.lookY,
        mobile: s.mobile,
        frame: s.frame,
        width: size.width,
        height: size.height,
      },
      framing
    )

    camera.position.copy(framing.target).addScaledVector(framing.dir, CAMERA_DISTANCE)
    camera.up.copy(framing.up)
    camera.lookAt(framing.target)
    const ortho = camera as THREE.OrthographicCamera
    if (Math.abs(ortho.zoom - framing.zoom) > 1e-3) {
      ortho.zoom = framing.zoom
      ortho.updateProjectionMatrix()
    }
  }, -90)
  return null
}

/**
 * Watches how fast this device is actually drawing and reports the moment the
 * answer is "not fast enough to be worth it" (see tier.ts for why measured
 * rather than recognised, and what it costs).
 *
 * Only meaningful under frameloop="always": in demand mode frames arrive when
 * something asks for one, so their spacing measures the pump, not the GPU.
 * BuildingExperience withholds `onDemote` in every other case — including
 * capture, and including a visitor who has already overridden a demotion.
 */
export function TierProbe({ onDemote }: { onDemote: (w: TierWindow) => void }) {
  const sampler = useMemo(() => createTierSampler(), [])
  const fired = useRef(false)
  useFrame((_, dt) => {
    if (fired.current) return
    const w = sampler.frame(dt * 1000)
    if (w?.demote) {
      fired.current = true
      onDemote(w)
    }
  }, -98)
  return null
}

/** Everything on the table. Reads the shared state; needs a driver and a Rig. */
export function SceneContent() {
  return (
    <>
      <BakedEnvironment />
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

/**
 * Exposure for AgX.
 *
 * AgX rolls off rather than clipping, so it needs less headroom than the
 * NoToneMapping path it replaces; above ~1.1 the day scene's white slabs go
 * chalky. One value for both surfaces, checked against the tour's pale
 * background and the billboard's dark navy — there is one canvas
 * configuration, so an exposure that only suits one of them is not available.
 */
const TONE_MAPPING_EXPOSURE = 1.0

/**
 * One canvas configuration, shared by the tour and the homepage billboard.
 *
 * `flat` stays off: it means `NoToneMapping`, which is what clipped the
 * highlights and crushed the midtones. AgX is set explicitly rather than
 * relying on the ACESFilmic default, because ACES pushes saturation into the
 * warm daylight this model is lit by.
 *
 * `onCreated` lives here rather than on each `<Canvas>` so neither surface can
 * drift from the other — the homepage moving with the tour is the point, not a
 * side effect.
 */
export const CANVAS_PROPS = {
  orthographic: true,
  shadows: 'percentage',
  flat: false,
  gl: { antialias: true, alpha: true, powerPreference: 'high-performance' },
  camera: { position: [24, 24, 24], zoom: 50, near: 0.1, far: 140 },
  style: { position: 'absolute', inset: 0 },
  onCreated: ({ gl }: { gl: THREE.WebGLRenderer }) => {
    gl.setClearColor(0x000000, 0)
    gl.toneMapping = THREE.AgXToneMapping
    gl.toneMappingExposure = TONE_MAPPING_EXPOSURE
  },
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
  /** Called once if this device measures below the floor twice running.
   *  Omitted when the measurement would be meaningless or unwanted. */
  onDemote?: (w: TierWindow) => void
}

export default function Scene({ shared, mobile, frameloop, onReady, onDemote }: SceneProps) {
  return (
    <Canvas
      {...CANVAS_PROPS}
      dpr={mobile ? [1, 1.5] : [1, 1.75]}
      frameloop={frameloop}
    >
      {/* The canvas is its own reconciler, so both contexts are re-provided here. */}
      <SharedStateContext.Provider value={shared}>
        <MobileTierContext.Provider value={mobile}>
          <Updater />
          <Ready onReady={onReady} />
          <Stats />
          {onDemote && <TierProbe onDemote={onDemote} />}
          <Rig />
          <SceneContent />
        </MobileTierContext.Provider>
      </SharedStateContext.Provider>
    </Canvas>
  )
}
