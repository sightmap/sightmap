import { useFrame } from '@react-three/fiber'
import { Html } from '@react-three/drei'
import { useMemo, useRef } from 'react'
import * as THREE from 'three'
import { KIOSK_H, PLATE, SLAB_T, TRAVELLER_COLORS, findRoom, floorY, roomStand } from './model'
import { smoothstep } from './chapters'
import { useShared } from './state'
import { Walker } from './Agents'

// Chapter 05. A looping vignette on the Checkout floor: the ContinueButton
// room slides to a new position (a redesign), a test walks to where the room
// used to be and finds nothing, the map updates, and the test walks on to the
// room's new position. Timings below are seconds into an 8.4s loop.
const FLOOR = 3
const PERIOD = 8.4
const T = {
  slideStart: 1.0,
  slideEnd: 1.8,
  walkEnd: 2.6,
  failEnd: 3.9,
  updateEnd: 4.7,
  walk2End: 6.0,
  passEnd: 7.8,
}

type Phase = 'idle' | 'walk' | 'fail' | 'update' | 'walk2' | 'pass' | 'reset'

function phaseAt(t: number): Phase {
  if (t < T.slideStart) return 'idle'
  if (t < T.walkEnd) return 'walk'
  if (t < T.failEnd) return 'fail'
  if (t < T.updateEnd) return 'update'
  if (t < T.walk2End) return 'walk2'
  if (t < T.passEnd) return 'pass'
  return 'reset'
}

const STATUS: Record<Phase, { cls: string; text: string }> = {
  idle: { cls: 'run', text: 'test Regression · step 4 of 5' },
  walk: { cls: 'run', text: 'click ContinueButton' },
  fail: { cls: 'fail', text: '✗ [data-testid="continue-btn"] not found' },
  update: { cls: 'map', text: 'sightmap: ContinueButton moved · selector updated' },
  walk2: { cls: 'run', text: 'retry via ContinueButton' },
  pass: { cls: 'pass', text: '✓ ContinueButton clicked · run passed' },
  reset: { cls: 'run', text: '' },
}

export default function HealDemo() {
  const s = useShared()
  const walker = useRef<THREE.Group>(null)
  const ghost = useRef<THREE.Mesh>(null)
  const status = useRef<HTMLDivElement>(null)
  const room = useMemo(() => findRoom(FLOOR, 'ContinueButton'), [])
  const from = useMemo(() => new THREE.Vector3(...roomStand(FLOOR, findRoom(FLOOR, 'PaymentForm'))), [])
  const oldPos = useMemo(() => new THREE.Vector3(...roomStand(FLOOR, room, 0)), [room])
  const newPos = useMemo(() => new THREE.Vector3(...roomStand(FLOOR, room, 1)), [room])
  const tmp = useMemo(() => new THREE.Vector3(), [])
  const start = useRef<number | null>(null)
  const lastPhase = useRef<Phase>('reset')

  useFrame(({ clock }) => {
    const c = s.cur
    const active = c.heal > 0.5
    const now = clock.getElapsedTime()
    if (!active) {
      start.current = null
      s.healShift = THREE.MathUtils.damp(s.healShift, 0, 6, 0.016)
    } else if (start.current === null) {
      start.current = now
    }
    const t = start.current === null ? 0 : (now - start.current) % PERIOD
    const phase: Phase = active ? phaseAt(t) : 'reset'
    if (active) {
      const slide = smoothstep((t - T.slideStart) / (T.slideEnd - T.slideStart))
      s.healShift = phase === 'reset' ? 1 - smoothstep((t - T.passEnd) / (PERIOD - T.passEnd)) : slide
    }
    // Walker position along the two legs.
    if (walker.current) {
      let p = from
      if (phase === 'walk') {
        p = tmp.copy(from).lerp(oldPos, smoothstep((t - T.slideStart) / (T.walkEnd - T.slideStart)))
      } else if (phase === 'fail' || phase === 'update') {
        p = oldPos
      } else if (phase === 'walk2') {
        p = tmp.copy(oldPos).lerp(newPos, smoothstep((t - T.updateEnd) / (T.walk2End - T.updateEnd)))
      } else if (phase === 'pass') {
        p = newPos
      }
      walker.current.position.copy(p)
      const bob = phase === 'walk' || phase === 'walk2' ? Math.abs(Math.sin(t * 14)) * 0.05 : 0
      walker.current.position.y += bob
      const sc = c.heal * (phase === 'reset' ? 1 - smoothstep((t - T.passEnd) / (PERIOD - T.passEnd)) : 1)
      walker.current.scale.setScalar(Math.max(sc, 0.001))
      walker.current.visible = sc > 0.02
    }
    // Ghost of the old room position once it has moved.
    if (ghost.current) {
      const m = ghost.current.material as THREE.MeshStandardMaterial
      const show = active && (phase === 'walk' || phase === 'fail' || phase === 'update') ? s.healShift : 0
      m.opacity = show * 0.5
      ghost.current.visible = show > 0.02
      m.color.set(phase === 'fail' ? '#e04d6a' : '#ffffff')
      m.emissive.set(phase === 'fail' ? '#e04d6a' : '#ffffff')
    }
    if (status.current) {
      const st = STATUS[phase]
      if (phase !== lastPhase.current) {
        status.current.textContent = st.text
        status.current.className = `bld-status bld-status--${st.cls}`
        lastPhase.current = phase
      }
      const o = active && phase !== 'reset' ? c.heal : 0
      status.current.style.opacity = o.toFixed(2)
      status.current.style.visibility = o > 0.02 ? 'visible' : 'hidden'
    }
  })

  return (
    <>
      <Walker color={TRAVELLER_COLORS.test} group={walker} trail={false} />
      <mesh ref={ghost} position={[room.x, floorY(FLOOR) + SLAB_T + PLATE + KIOSK_H / 2, room.z]} visible={false}>
        <boxGeometry args={[room.w, KIOSK_H, room.d]} />
        <meshStandardMaterial color="#ffffff" transparent opacity={0} wireframe emissive="#ffffff" emissiveIntensity={0.6} />
      </mesh>
      <Html position={[room.x - 1.6, floorY(FLOOR) + SLAB_T + KIOSK_H + 1.15, room.z - 0.4]} zIndexRange={[7, 0]} style={{ pointerEvents: 'none' }}>
        <div ref={status} className="bld-status bld-status--run" style={{ opacity: 0, visibility: 'hidden' }} />
      </Html>
    </>
  )
}
