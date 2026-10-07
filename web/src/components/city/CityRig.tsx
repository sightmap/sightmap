// The camera. A narrow-FOV perspective keeps the toy-town read while leaving
// enough parallax to sell depth, and the orbit is deliberately penned in: a
// fixed pitch band, free yaw, clamped zoom and a pan that cannot leave the
// plan. A limited camera is what makes a procedural scene look intentional,
// because every framing it can reach was chosen.
import { useEffect, useMemo, useRef } from 'react'
import { useFrame, useThree } from '@react-three/fiber'
import * as THREE from 'three'
import type { CityLot, CityPlan } from '@/types/city'

export const PITCH_MIN = THREE.MathUtils.degToRad(35)
export const PITCH_MAX = THREE.MathUtils.degToRad(55)
export const DISTANCE_MIN = 34
export const DISTANCE_MAX = 660

const OVERVIEW = { yaw: Math.PI * 0.18, pitch: THREE.MathUtils.degToRad(48), distance: 470 }
/** How close the camera stands when it has flown down to one building. */
const CLOSE = { pitch: THREE.MathUtils.degToRad(38), distance: 44 }

interface Pose {
  x: number
  z: number
  yaw: number
  pitch: number
  distance: number
}

export interface CityRigProps {
  plan: CityPlan
  /** The lot the camera has flown down to, or null for the whole city. */
  focus: CityLot | null
  reduced: boolean
}

export default function CityRig({ plan, focus, reduced }: CityRigProps) {
  const { camera, gl } = useThree()
  const goal = useRef<Pose>({ x: 0, z: 0, ...OVERVIEW })
  const now = useRef<Pose>({ ...goal.current })
  const home = useRef<Pose>({ ...goal.current })
  /** True while the camera is down at a building, so "back" means the city. */
  const down = useRef(false)
  const v = useMemo(() => ({ dir: new THREE.Vector3(), target: new THREE.Vector3(), right: new THREE.Vector3() }), [])
  const limit = useMemo(
    () => ({ x: plan.bounds.w * 0.52, z: plan.bounds.d * 0.52 }),
    [plan.bounds.w, plan.bounds.d]
  )

  // Drag orbits, shift- or right-drag pans, wheel and pinch zoom. Written
  // here rather than taken from a controls helper because the clamps are the
  // point: nothing may leave the plan, tip over, or fly to the ground.
  useEffect(() => {
    const el = gl.domElement
    const pointers = new Map<number, { x: number; y: number }>()
    let mode: 'orbit' | 'pan' | null = null
    let pinch = 0

    const clampTarget = () => {
      goal.current.x = THREE.MathUtils.clamp(goal.current.x, -limit.x, limit.x)
      goal.current.z = THREE.MathUtils.clamp(goal.current.z, -limit.z, limit.z)
    }

    const onDown = (e: PointerEvent) => {
      pointers.set(e.pointerId, { x: e.clientX, y: e.clientY })
      el.setPointerCapture(e.pointerId)
      if (pointers.size === 2) {
        const [a, b] = [...pointers.values()]
        pinch = Math.hypot(a.x - b.x, a.y - b.y)
        mode = 'pan'
        return
      }
      mode = e.button === 2 || e.shiftKey ? 'pan' : 'orbit'
    }

    const onMove = (e: PointerEvent) => {
      const last = pointers.get(e.pointerId)
      if (!last) return
      const dx = e.clientX - last.x
      const dy = e.clientY - last.y
      pointers.set(e.pointerId, { x: e.clientX, y: e.clientY })
      if (pointers.size === 2) {
        const [a, b] = [...pointers.values()]
        const span = Math.hypot(a.x - b.x, a.y - b.y)
        if (pinch > 0 && span > 0) {
          goal.current.distance = THREE.MathUtils.clamp(
            goal.current.distance * (pinch / span),
            DISTANCE_MIN,
            DISTANCE_MAX
          )
        }
        pinch = span
        return
      }
      if (mode === 'orbit') {
        goal.current.yaw -= dx * 0.005
        goal.current.pitch = THREE.MathUtils.clamp(goal.current.pitch + dy * 0.004, PITCH_MIN, PITCH_MAX)
      } else if (mode === 'pan') {
        const scale = goal.current.distance * 0.0016
        const sin = Math.sin(goal.current.yaw)
        const cos = Math.cos(goal.current.yaw)
        goal.current.x -= (dx * cos - dy * sin) * scale
        goal.current.z += (dx * sin + dy * cos) * scale
        clampTarget()
      }
    }

    const onUp = (e: PointerEvent) => {
      pointers.delete(e.pointerId)
      if (pointers.size < 2) pinch = 0
      if (pointers.size === 0) mode = null
      if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId)
    }

    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      goal.current.distance = THREE.MathUtils.clamp(
        goal.current.distance * Math.exp(e.deltaY * 0.0012),
        DISTANCE_MIN,
        DISTANCE_MAX
      )
    }

    const onContext = (e: Event) => e.preventDefault()

    el.addEventListener('pointerdown', onDown)
    el.addEventListener('pointermove', onMove)
    el.addEventListener('pointerup', onUp)
    el.addEventListener('pointercancel', onUp)
    el.addEventListener('wheel', onWheel, { passive: false })
    el.addEventListener('contextmenu', onContext)
    return () => {
      el.removeEventListener('pointerdown', onDown)
      el.removeEventListener('pointermove', onMove)
      el.removeEventListener('pointerup', onUp)
      el.removeEventListener('pointercancel', onUp)
      el.removeEventListener('wheel', onWheel)
      el.removeEventListener('contextmenu', onContext)
    }
  }, [gl, limit])

  // Level 2 is a camera move, not a page: the city stays where it is and the
  // view flies down to one address, then back to where it came from.
  useEffect(() => {
    if (focus) {
      // Only the first descent remembers where the city was being looked at;
      // walking from one building to the next must not overwrite it.
      if (!down.current) home.current = { ...goal.current }
      down.current = true
      goal.current = {
        x: focus.x,
        z: focus.z,
        // Stand in front of the building, which is the way its lot faces.
        yaw: focus.rotation,
        pitch: CLOSE.pitch,
        distance: CLOSE.distance,
      }
    } else {
      down.current = false
      goal.current = { ...home.current }
    }
  }, [focus])

  useFrame((_, dt) => {
    const d = Math.min(dt, 0.25)
    const g = goal.current
    const c = now.current
    if (reduced) {
      c.x = g.x
      c.z = g.z
      c.yaw = g.yaw
      c.pitch = g.pitch
      c.distance = g.distance
    } else {
      c.x = THREE.MathUtils.damp(c.x, g.x, 3.2, d)
      c.z = THREE.MathUtils.damp(c.z, g.z, 3.2, d)
      c.yaw = THREE.MathUtils.damp(c.yaw, g.yaw, 6, d)
      c.pitch = THREE.MathUtils.damp(c.pitch, g.pitch, 6, d)
      c.distance = THREE.MathUtils.damp(c.distance, g.distance, 3.2, d)
    }
    v.dir.set(Math.cos(c.pitch) * Math.sin(c.yaw), Math.sin(c.pitch), Math.cos(c.pitch) * Math.cos(c.yaw))
    v.target.set(c.x, 2, c.z)
    camera.position.copy(v.target).addScaledVector(v.dir, c.distance)
    camera.lookAt(v.target)
  }, -90)

  return null
}
