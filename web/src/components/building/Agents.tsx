import { useFrame } from '@react-three/fiber'
import { Trail } from '@react-three/drei'
import { useMemo, useRef } from 'react'
import * as THREE from 'three'
import { JOURNEYS, TRAVELLER_COLORS, type Journey } from './model'
import { buildPath, pointAt, type Path } from './geometry'
import { smoothstep } from './chapters'
import { useShared } from './state'

// The people: one walker per journey, looping through its stops, riding the
// core between floors. Users are pink, coding agents green, tests gold.
const WALK = 1.75
const DWELL = 0.9
const RESET = 1.4

interface Runner {
  d: number
  dwell: number
  next: number
  wait: number
  fade: number
}

export function Walker({
  color,
  group,
  trailRef,
  scale = 1,
  trail = true,
  trailLength = 3,
}: {
  color: string
  group: React.RefObject<THREE.Group | null>
  /** drei portals the trail mesh to the scene root, so it has to be hidden
   *  through its own ref rather than through the walker's group. */
  trailRef?: React.RefObject<THREE.Mesh | null>
  scale?: number
  trail?: boolean
  trailLength?: number
}) {
  return (
    <>
      <group ref={group} scale={scale}>
        <group scale={1.35}>
        <mesh position={[0, 0.2, 0]} castShadow>
          <capsuleGeometry args={[0.1, 0.2, 4, 10]} />
          <meshStandardMaterial color={color} emissive={color} emissiveIntensity={0.35} roughness={0.5} />
        </mesh>
        <mesh position={[0, 0.5, 0]} castShadow>
          <sphereGeometry args={[0.12, 14, 14]} />
          <meshStandardMaterial color="#fbf8f2" roughness={0.6} />
        </mesh>
        <mesh position={[0, 0.012, 0]} rotation-x={-Math.PI / 2}>
          <ringGeometry args={[0.18, 0.28, 24]} />
          <meshBasicMaterial color={color} transparent opacity={0.55} depthWrite={false} />
        </mesh>
        </group>
      </group>
      {trail && (
        <Trail
          // drei types the ref as the geometry; at runtime it is the trail mesh.
          ref={trailRef as unknown as React.ComponentProps<typeof Trail>['ref']}
          width={0.7}
          length={trailLength}
          color={color}
          attenuation={(w) => w * w}
          decay={1.2}
          target={group as React.RefObject<THREE.Object3D>}
        />
      )}
    </>
  )
}

function Agent({ journey, path }: { journey: Journey; path: Path }) {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
  const tg = useRef<THREE.Mesh>(null)
  const run = useRef<Runner>({ d: 0, dwell: DWELL, next: 1, wait: journey.delay, fade: 0 })
  const tmp = useMemo(() => new THREE.Vector3(), [])
  const ahead = useMemo(() => new THREE.Vector3(), [])
  const color = TRAVELLER_COLORS[journey.who]

  useFrame((_, dt) => {
    const r = run.current
    const d = Math.min(dt, 0.2)
    const c = s.cur
    if (!s.reduced) {
      if (r.wait > 0) {
        r.wait -= d
        r.fade = Math.max(0, r.fade - d * 2)
      } else if (r.dwell > 0) {
        r.dwell -= d
        r.fade = Math.min(1, r.fade + d * 2.5)
      } else if (r.next < path.stops.length) {
        const target = path.cum[path.stops[r.next]]
        r.d = Math.min(target, r.d + WALK * d)
        if (r.d >= target - 1e-4) {
          r.next += 1
          r.dwell = DWELL
        }
      } else {
        // Journey complete: fade out, go back to the start.
        r.fade = Math.max(0, r.fade - d * 2.5)
        if (r.fade <= 0) {
          r.d = 0
          r.next = 1
          r.dwell = DWELL
          r.wait = RESET
        }
      }
    } else {
      r.fade = 1
    }
    if (!g.current) return
    pointAt(path, r.d, tmp)
    g.current.position.copy(tmp)
    // Heading follows the path tangent: sample a point slightly further
    // along the (now Catmull-Rom-rounded) path and face it.
    pointAt(path, r.d + 0.05, ahead)
    if (ahead.distanceToSquared(tmp) > 1e-8) {
      g.current.rotation.y = Math.atan2(ahead.x - tmp.x, ahead.z - tmp.z)
    }
    const focus = s.focus && s.focus !== journey.name ? 0.3 : 1
    const rise = smoothstep(Math.min(1, c.rise * 1.3 - 0.3))
    const sc = c.agents * focus * smoothstep(r.fade) * rise
    g.current.scale.setScalar(Math.max(sc, 0.001))
    g.current.visible = sc > 0.02
    if (tg.current) tg.current.visible = sc > 0.4 && r.fade > 0.5 && r.dwell <= 0
  })

  return <Walker color={color} group={g} trailRef={tg} trail={!s.reduced} trailLength={s.mobile ? 2 : 3.5} />
}

export default function Agents() {
  const s = useShared()
  const journeys = useMemo(() => (s.mobile ? JOURNEYS.slice(0, 5) : JOURNEYS), [s.mobile])
  const paths = useMemo(() => journeys.map((j) => buildPath(j)), [journeys])
  return (
    <>
      {journeys.map((j, k) => (
        <Agent key={j.name} journey={j} path={paths[k]} />
      ))}
    </>
  )
}
