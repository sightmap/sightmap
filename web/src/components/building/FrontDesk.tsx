import { useFrame } from '@react-three/fiber'
import { Html, RoundedBox } from '@react-three/drei'
import { useRef } from 'react'
import * as THREE from 'three'
import { SLAB_T, TOOLS, TRAVELLER_COLORS } from './model'
import { smoothstep } from './chapters'
import { useShared } from './state'
import { Walker } from './Agents'

// Chapter 07. A front desk in the lobby with the tools the building offers,
// and an agent at the door about to use one.
const DESK: [number, number, number] = [4.05, SLAB_T, -1.2]

export default function FrontDesk() {
  const s = useShared()
  const g = useRef<THREE.Group>(null)
  const visitor = useRef<THREE.Group>(null)
  const cards = useRef<(HTMLDivElement | null)[]>([])
  const desk = useRef<THREE.Group>(null)
  useFrame(({ clock }) => {
    const o = smoothstep(s.cur.desk)
    const rise = smoothstep(Math.min(1, s.cur.rise * 1.6 - 0.3))
    if (desk.current) {
      desk.current.scale.set(1, Math.max(rise, 0.001), 1)
      desk.current.visible = rise > 0.01
    }
    if (g.current) {
      g.current.scale.setScalar(Math.max(o, 0.001))
      g.current.visible = o > 0.01
    }
    if (visitor.current) {
      const t = clock.getElapsedTime()
      visitor.current.position.set(6.1 + (s.reduced ? 0 : Math.sin(t * 0.8) * 0.05), 0, -1.2)
      visitor.current.scale.setScalar(Math.max(o, 0.001))
      visitor.current.visible = o > 0.01
    }
    cards.current.forEach((el, k) => {
      if (!el) return
      const po = Math.min(1, Math.max(0, o * 1.8 - k * 0.25))
      el.style.opacity = po.toFixed(2)
      el.style.visibility = po > 0.02 ? 'visible' : 'hidden'
      el.style.transform = `translateY(${((1 - po) * 14).toFixed(1)}px)`
    })
  })
  return (
    <>
      <group ref={desk} position={DESK}>
        <RoundedBox args={[0.9, 0.8, 2.1]} radius={0.05} position={[0, 0.4, 0]} castShadow receiveShadow>
          <meshStandardMaterial color="#6b4a2f" roughness={0.7} />
        </RoundedBox>
        <RoundedBox args={[1.1, 0.06, 2.3]} radius={0.03} position={[0, 0.83, 0]} castShadow receiveShadow>
          <meshStandardMaterial color="#fbf8f2" roughness={0.5} />
        </RoundedBox>
        <mesh position={[-0.55, 0.3, 0.2]} castShadow>
          <capsuleGeometry args={[0.13, 0.32, 4, 10]} />
          <meshStandardMaterial color="#4f5d75" roughness={0.8} />
        </mesh>
        <mesh position={[-0.55, 0.66, 0.2]} castShadow>
          <sphereGeometry args={[0.085, 12, 10]} />
          <meshStandardMaterial color="#c9976f" roughness={0.8} />
        </mesh>
        <mesh position={[0.25, 0.98, -0.7]} castShadow>
          <cylinderGeometry args={[0.13, 0.13, 0.26, 12]} />
          <meshStandardMaterial color="#c7b299" roughness={0.9} />
        </mesh>
        <mesh position={[0.25, 1.28, -0.7]} castShadow>
          <sphereGeometry args={[0.24, 12, 10]} />
          <meshStandardMaterial color="#67a05a" roughness={1} />
        </mesh>
      </group>
      <group ref={g} position={DESK}>
        <mesh position={[0, 1.45, 0]}>
          <cylinderGeometry args={[0.03, 0.03, 1.2, 8]} />
          <meshStandardMaterial color="#8a8272" roughness={0.6} />
        </mesh>
        {TOOLS.map((tool, k) => (
          <Html key={tool.name} position={[1.0, 0.9 + k * 0.7, -1.4]} center zIndexRange={[7, 0]} style={{ pointerEvents: 'none' }}>
            <div
              ref={(el) => {
                cards.current[k] = el
              }}
              className="bld-tool"
              style={{ opacity: 0, visibility: 'hidden' }}
            >
              <b>
                {tool.name}
                <span>({tool.args})</span>
              </b>
              <small>{tool.via}</small>
            </div>
          </Html>
        ))}
      </group>
      <Walker color={TRAVELLER_COLORS.agent} group={visitor} trail={false} />
    </>
  )
}
