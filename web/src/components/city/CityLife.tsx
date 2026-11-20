// Traffic and people. Both are decoration on a loop — the plan says how many
// run each road and where they start, and the renderer only decides how fast.
// SimCity's own automata are a visual approximation loosely correlated with
// the simulation; these are not even that, and they are what make the city
// read as inhabited rather than modelled.
import { useEffect, useMemo, useRef } from 'react'
import { useFrame } from '@react-three/fiber'
import * as THREE from 'three'
import type { CityPlan } from '@/types/city'
import { loopPath, loopSpeed, phaseShift, poseAt, type LoopPath, type LoopPose } from './loops'
import { BOX } from './geometry'
import { CAR_COLORS, PEDESTRIAN_COLOR } from './palette'

const CAR = { w: 1.9, h: 1.0, d: 3.9 }
const CAPSULE = new THREE.CapsuleGeometry(0.24, 0.5, 3, 6)

interface Agent {
  path: LoopPath
  /** Where this agent sits on the loop at t = 0, in [0, 1). */
  offset: number
  /** Loops a second. */
  rate: number
}

interface Fleet {
  mesh: THREE.InstancedMesh
  agents: Agent[]
}

function fleet(
  plan: CityPlan,
  kind: 'car' | 'pedestrian',
  geometry: THREE.BufferGeometry,
  material: THREE.Material,
  lift: number
): Fleet | null {
  const agents: Agent[] = []
  const colors: number[] = []
  const color = new THREE.Color()
  for (const loop of plan.loops) {
    if (loop.kind !== kind) continue
    const path = loopPath(loop.points)
    if (path.length === 0) continue
    const rate = loopSpeed(loop) / path.length
    for (let i = 0; i < loop.count; i++) {
      agents.push({ path, offset: (i + phaseShift(loop.phases[i])) / loop.count, rate })
      color.set(kind === 'car' ? CAR_COLORS[(i + loop.id.length) % CAR_COLORS.length] : PEDESTRIAN_COLOR)
      colors.push(color.r, color.g, color.b)
    }
  }
  if (agents.length === 0) return null
  const mesh = new THREE.InstancedMesh(geometry, material, agents.length)
  mesh.instanceColor = new THREE.InstancedBufferAttribute(new Float32Array(colors), 3)
  mesh.position.y = lift
  mesh.frustumCulled = false
  mesh.name = `city-${kind}s`
  return { mesh, agents }
}

export interface CityLifeProps {
  plan: CityPlan
  reduced: boolean
}

export default function CityLife({ plan, reduced }: CityLifeProps) {
  const built = useMemo(() => {
    const carMat = new THREE.MeshStandardMaterial({ roughness: 0.5, metalness: 0.15 })
    const walkerMat = new THREE.MeshStandardMaterial({ roughness: 0.9 })
    const group = new THREE.Group()
    group.name = 'city-life'
    const cars = fleet(plan, 'car', BOX, carMat, CAR.h / 2)
    const walkers = fleet(plan, 'pedestrian', CAPSULE, walkerMat, 0.62)
    for (const f of [cars, walkers]) if (f) group.add(f.mesh)
    return { group, fleets: [cars, walkers].filter((f): f is Fleet => f !== null), materials: [carMat, walkerMat] }
  }, [plan])

  useEffect(
    () => () => {
      for (const mat of built.materials) mat.dispose()
    },
    [built]
  )

  const t = useRef(0)
  // One scratch set for the whole animation loop: nothing here allocates per
  // frame, which is the only reason seventy agents cost nothing.
  const scratch = useMemo(
    () => ({
      pose: { x: 0, z: 0, angle: 0 } as LoopPose,
      matrix: new THREE.Matrix4(),
      position: new THREE.Vector3(),
      quaternion: new THREE.Quaternion(),
      euler: new THREE.Euler(),
      scale: new THREE.Vector3(),
    }),
    []
  )

  const draw = (time: number) => {
    for (const f of built.fleets) {
      const isCar = f.mesh.geometry === BOX
      scratch.scale.set(isCar ? CAR.w : 1, isCar ? CAR.h : 1, isCar ? CAR.d : 1)
      for (let i = 0; i < f.agents.length; i++) {
        const agent = f.agents[i]
        poseAt(agent.path, agent.offset + time * agent.rate, scratch.pose)
        scratch.position.set(scratch.pose.x, 0, scratch.pose.z)
        scratch.euler.set(0, scratch.pose.angle, 0)
        scratch.quaternion.setFromEuler(scratch.euler)
        scratch.matrix.compose(scratch.position, scratch.quaternion, scratch.scale)
        f.mesh.setMatrixAt(i, scratch.matrix)
      }
      f.mesh.instanceMatrix.needsUpdate = true
    }
  }

  useEffect(() => {
    // Reduced motion parks every agent where it started rather than hiding
    // them: the streets still have traffic on them, it simply is not moving.
    draw(0)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [built])

  useFrame((_, dt) => {
    if (reduced) return
    t.current += Math.min(dt, 0.25)
    draw(t.current)
  })

  return <primitive object={built.group} />
}
