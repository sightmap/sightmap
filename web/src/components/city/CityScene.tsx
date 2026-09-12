// The WebGL half of the city. Loaded only through CityView's lazy boundary,
// so three.js never reaches the prerender or a browser without a canvas.
//
// Everything the scene draws is instanced per part kind and everything it
// animates is a transform, so the whole city — three hundred lots, their
// props, seventy agents — stays inside a couple of dozen draw calls.
import { useEffect, useMemo, useRef } from 'react'
import { Canvas, useFrame, useThree } from '@react-three/fiber'
import * as THREE from 'three'
import CityBuildings from './CityBuildings'
import CityFocus from './CityFocus'
import CityGround from './CityGround'
import CityLife from './CityLife'
import CityLights from './CityLights'
import CityProps from './CityProps'
import CityRig from './CityRig'
import { FOOTPRINT, builtLots, cityBuildings, type CityBuilding } from './buildings'
import { BOX } from './geometry'
import type { CityDocument, CityListing } from './document'

export interface CitySceneProps {
  city: CityDocument
  listings: CityListing[]
  night: boolean
  reduced: boolean
  /** Slug of the building the camera has flown down to. */
  focus: string | null
  onReady: () => void
  onHover: (slug: string | null) => void
  onSelect: (slug: string | null) => void
}

/** Calls `onReady` after the first rendered frame, so the legend can step aside. */
function Ready({ onReady }: { onReady: () => void }) {
  const fired = useRef(false)
  useFrame(() => {
    if (fired.current) return
    fired.current = true
    onReady()
  }, -99)
  return null
}

/** Draw calls and frame rate, for the same headless check the tour uses. */
function Stats() {
  const { gl } = useThree()
  const frames = useRef(0)
  const t0 = useRef(0)
  useFrame(() => {
    frames.current++
    const now = performance.now()
    if (t0.current === 0) t0.current = now
    if (now - t0.current < 1000) return
    const w = window as unknown as { __cityStats?: Record<string, number> }
    w.__cityStats = {
      fps: Math.round((frames.current * 1000) / (now - t0.current)),
      calls: gl.info.render.calls,
      triangles: gl.info.render.triangles,
      geometries: gl.info.memory.geometries,
      textures: gl.info.memory.textures,
    }
    frames.current = 0
    t0.current = now
  })
  return null
}

/**
 * One invisible box per listing's building, and the only thing the pointer
 * ever hits. Raycasting the instanced parts would mean testing every window in
 * the city; this is eight boxes, and it costs no draw call because it is never
 * drawn.
 */
function Picker({
  buildings,
  onHover,
  onSelect,
}: {
  buildings: CityBuilding[]
  onHover: (slug: string | null) => void
  onSelect: (slug: string | null) => void
}) {
  const { gl, camera } = useThree()
  const picks = useMemo(() => buildings.filter((b) => b.slug), [buildings])
  const mesh = useMemo(() => {
    const m = new THREE.InstancedMesh(BOX, new THREE.MeshBasicMaterial(), Math.max(picks.length, 1))
    const matrix = new THREE.Matrix4()
    const position = new THREE.Vector3()
    const quaternion = new THREE.Quaternion()
    const scale = new THREE.Vector3()
    picks.forEach((b, i) => {
      position.set(b.lot.x, b.height / 2 + 0.5, b.lot.z)
      quaternion.setFromEuler(new THREE.Euler(0, b.lot.rotation, 0))
      scale.set(FOOTPRINT.w + 1, b.height + 1, FOOTPRINT.d + 1)
      matrix.compose(position, quaternion, scale)
      m.setMatrixAt(i, matrix)
    })
    m.count = picks.length
    m.visible = false
    m.name = 'city-pick'
    m.computeBoundingSphere()
    return m
  }, [picks])

  useEffect(() => () => (mesh.material as THREE.Material).dispose(), [mesh])

  useEffect(() => {
    const el = gl.domElement
    const raycaster = new THREE.Raycaster()
    const ndc = new THREE.Vector2()
    const hits: THREE.Intersection[] = []
    let down: { x: number; y: number } | null = null
    let hovered: string | null = null

    const slugAt = (e: PointerEvent | MouseEvent): string | null => {
      const rect = el.getBoundingClientRect()
      ndc.set(((e.clientX - rect.left) / rect.width) * 2 - 1, -((e.clientY - rect.top) / rect.height) * 2 + 1)
      raycaster.setFromCamera(ndc, camera)
      hits.length = 0
      mesh.raycast(raycaster, hits)
      if (hits.length === 0) return null
      hits.sort((a, b) => a.distance - b.distance)
      const id = hits[0].instanceId
      return id === undefined ? null : (picks[id]?.slug ?? null)
    }

    const onMove = (e: PointerEvent) => {
      if (picks.length === 0) return
      const slug = slugAt(e)
      if (slug === hovered) return
      hovered = slug
      el.style.cursor = slug ? 'pointer' : ''
      onHover(slug)
    }
    const onDown = (e: PointerEvent) => {
      down = { x: e.clientX, y: e.clientY }
    }
    const onUp = (e: PointerEvent) => {
      if (!down) return
      const moved = Math.hypot(e.clientX - down.x, e.clientY - down.y)
      down = null
      // A drag is a camera move, never a click on what happened to be under it.
      if (moved > 5 || e.button !== 0) return
      onSelect(slugAt(e))
    }
    const onLeave = () => {
      if (hovered === null) return
      hovered = null
      el.style.cursor = ''
      onHover(null)
    }

    el.addEventListener('pointermove', onMove)
    el.addEventListener('pointerdown', onDown)
    el.addEventListener('pointerup', onUp)
    el.addEventListener('pointerleave', onLeave)
    return () => {
      el.removeEventListener('pointermove', onMove)
      el.removeEventListener('pointerdown', onDown)
      el.removeEventListener('pointerup', onUp)
      el.removeEventListener('pointerleave', onLeave)
      el.style.cursor = ''
    }
  }, [gl, camera, mesh, picks, onHover, onSelect])

  return <primitive object={mesh} />
}

function City({ city, listings, night, reduced, focus, onReady, onHover, onSelect }: CitySceneProps) {
  const buildings = useMemo(() => cityBuildings(city, listings), [city, listings])
  const built = useMemo(() => builtLots(buildings), [buildings])
  const empty = useMemo(
    () => new Set(buildings.filter((b) => b.kind === 'empty').map((b) => b.lot.id)),
    [buildings]
  )
  const focused = useMemo(() => buildings.find((b) => b.slug === focus) ?? null, [buildings, focus])
  const blueprint = useMemo(() => listings.find((l) => l.slug === focus)?.blueprint ?? null, [listings, focus])

  return (
    <>
      <Ready onReady={onReady} />
      <Stats />
      <CityRig plan={city} focus={focused?.lot ?? null} reduced={reduced} />
      <CityLights plan={city} night={night} />
      <CityGround plan={city} built={built} empty={empty} />
      <CityBuildings buildings={buildings} night={night} hiddenLot={focused?.lot.id ?? null} />
      <CityProps plan={city} buildings={buildings} reduced={reduced} />
      <CityLife plan={city} reduced={reduced} />
      <Picker buildings={buildings} onHover={onHover} onSelect={onSelect} />
      {focused && blueprint && (
        <CityFocus lot={focused.lot} blueprint={blueprint} night={night} reduced={reduced} />
      )}
    </>
  )
}

export default function CityScene(props: CitySceneProps) {
  return (
    <Canvas
      shadows="percentage"
      flat
      gl={{ antialias: true, powerPreference: 'high-performance' }}
      camera={{ fov: 30, position: [120, 220, 260], near: 1, far: 2200 }}
      dpr={[1, 1.75]}
      style={{ position: 'absolute', inset: 0, touchAction: 'none' }}
    >
      <City {...props} />
    </Canvas>
  )
}
