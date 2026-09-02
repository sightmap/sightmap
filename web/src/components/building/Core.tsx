import { useFrame } from '@react-three/fiber'
import { Edges, Instance, Instances } from '@react-three/drei'
import { useMemo, useRef } from 'react'
import * as THREE from 'three'
import { CORE, FLOORS, FLOOR_H, RISERS, SLAB_T, floorY } from './model'
import { smoothstep } from './chapters'
import { useShared } from './state'

// The service core: a glass elevator shaft in the back corner with the API
// risers running up inside it. Pulses travel the risers — requests in flight.
const TOP = FLOORS.length * FLOOR_H + 0.55
const stepGeom = new THREE.BoxGeometry(1, 1, 1)

// A switchback stair climbing the shaft next to the elevator.
function Stairs() {
  const steps = useMemo(() => {
    const out: { p: [number, number, number]; s: [number, number, number] }[] = []
    const perFlight = 7
    const x0 = CORE.x - CORE.w / 2 + 0.22
    for (let f = 0; f < FLOORS.length; f++) {
      const base = floorY(f) + SLAB_T
      for (let k = 0; k < perFlight; k++) {
        const t = k / perFlight
        const y = base + 0.15 + t * (FLOOR_H - 0.3)
        const z = f % 2 === 0 ? CORE.z - CORE.d / 2 + 0.2 + t * (CORE.d - 0.4) : CORE.z + CORE.d / 2 - 0.2 - t * (CORE.d - 0.4)
        out.push({ p: [x0, y, z], s: [0.36, 0.05, 0.2] })
      }
    }
    return out
  }, [])
  return (
    <Instances limit={steps.length} range={steps.length} geometry={stepGeom} castShadow>
      <meshStandardMaterial color="#d8bf9a" roughness={0.7} />
      {steps.map((st, k) => (
        <Instance key={k} position={st.p} scale={st.s} />
      ))}
    </Instances>
  )
}

function Pulses() {
  const s = useShared()
  const refs = useRef<(THREE.Mesh | null)[]>([])
  const heights = useMemo(() => RISERS.map((r) => floorY(Math.max(...r.floors)) + SLAB_T + 0.6), [])
  useFrame(({ clock }) => {
    const t = clock.getElapsedTime()
    RISERS.forEach((_, k) => {
      const m = refs.current[k]
      if (!m) return
      const speed = s.reduced ? 0 : 0.34 + k * 0.03
      const phase = ((t * speed + k * 0.37) % 1 + 1) % 1
      m.position.y = 0.1 + phase * (heights[k] - 0.2)
      const fade = Math.sin(phase * Math.PI)
      m.scale.setScalar(0.6 + 0.6 * fade)
      m.visible = s.cur.rise > 0.85
    })
  })
  return (
    <>
      {RISERS.map((r, k) => (
        <mesh
          key={r.name}
          ref={(el) => {
            refs.current[k] = el
          }}
          position={[riserX(k), 0, riserZ()]}
        >
          <sphereGeometry args={[0.085, 12, 12]} />
          <meshStandardMaterial color={r.color} emissive={r.color} emissiveIntensity={1.4} roughness={0.4} />
        </mesh>
      ))}
    </>
  )
}

const riserX = (k: number): number => CORE.x - CORE.w / 2 + 0.24 + k * 0.28
const riserZ = (): number => CORE.z - CORE.d / 2 + 0.22

export default function Core() {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
  const car = useRef<THREE.Mesh>(null)
  useFrame(({ clock }) => {
    const rise = smoothstep(THREE.MathUtils.clamp(s.cur.rise * 1.6 - 0.3, 0, 1))
    if (g.current) {
      g.current.visible = rise > 0.01
      g.current.scale.set(1, Math.max(rise, 0.001), 1)
    }
    if (car.current) {
      const t = s.reduced ? 0.4 : (Math.sin(clock.getElapsedTime() * 0.35) + 1) / 2
      car.current.position.y = 0.6 + t * (TOP - 1.9)
    }
  })
  return (
    <group ref={g}>
      <mesh position={[CORE.x, TOP / 2, CORE.z]}>
        <boxGeometry args={[CORE.w, TOP, CORE.d]} />
        <meshStandardMaterial color="#b7c9ee" transparent opacity={0.16} roughness={0.15} depthWrite={false} />
        <Edges color="#d5e2fb" threshold={20} lineWidth={1} transparent opacity={0.6} />
      </mesh>
      {/* Elevator car. */}
      <mesh ref={car} position={[CORE.x + 0.3, 0.6, CORE.z + 0.1]} castShadow>
        <boxGeometry args={[0.8, 1.05, 0.9]} />
        <meshStandardMaterial color="#fbf8f2" roughness={0.6} />
      </mesh>
      {RISERS.map((r, k) => {
        const h = floorY(Math.max(...r.floors)) + SLAB_T + 0.6
        return (
          <group key={r.name}>
            <mesh position={[riserX(k), h / 2, riserZ()]}>
              <cylinderGeometry args={[0.045, 0.045, h, 10]} />
              <meshStandardMaterial color={r.color} emissive={r.color} emissiveIntensity={0.35} roughness={0.5} />
            </mesh>
            {r.floors.map((f) => (
              <mesh key={f} position={[riserX(k), floorY(f) + SLAB_T + 0.35, riserZ() + 0.12]}>
                <boxGeometry args={[0.16, 0.16, 0.24]} />
                <meshStandardMaterial color={r.color} emissive={r.color} emissiveIntensity={0.5} roughness={0.5} />
              </mesh>
            ))}
          </group>
        )
      })}
      <Stairs />
      <Pulses />
    </group>
  )
}
