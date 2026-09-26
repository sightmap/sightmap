// The civic kit: the fountain on the plaza, the mast and its beacon, the water
// tower, trees, benches, bus shelters and billboards, plus the fence and the
// board on every lot still waiting for a listing. The research is blunt that
// ten props placed by rule do more for "this is a place" than ten more
// building variants, and they cost three draw calls between them because each
// kind is one instanced primitive.
import { useEffect, useMemo, useRef } from 'react'
import { useFrame } from '@react-three/fiber'
import * as THREE from 'three'
import type { CityPlan, CityProp } from '@/types/city'
import type { CityBuilding } from './buildings'
import { BOX, CONE, CYLINDER, SPHERE, lotMatrix, place } from './geometry'
import { PartBuilder } from './instancing'
import { PROPS, accent } from './palette'

const MAST_H = 26
const BEACON_Y = MAST_H + 1.2

interface Kit {
  group: THREE.Group
  beacon: THREE.Mesh | null
  dispose: () => void
}

function build(plan: CityPlan, buildings: CityBuilding[]): Kit {
  const group = new THREE.Group()
  group.name = 'city-props'
  const boxes = new PartBuilder()
  const cylinders = new PartBuilder()
  const cones = new PartBuilder()

  const base = new THREE.Matrix4()
  const m = new THREE.Matrix4()
  const color = new THREE.Color()
  const accentOf = (prop: CityProp) =>
    color.set(accent(plan.districts.find((d) => d.archetype === prop.district)?.accent ?? 0))

  for (const prop of plan.props) {
    lotMatrix(base, prop.x, prop.z, prop.rotation)
    switch (prop.kind) {
      case 'fountain':
        cylinders.add(place(m, base, 0, 0.3, 0, 7, 0.6, 7), color.set(PROPS.board), -1)
        cylinders.add(place(m, base, 0, 0.85, 0, 5.4, 0.5, 5.4), color.set(PROPS.glass), -1)
        cylinders.add(place(m, base, 0, 1.6, 0, 1.1, 2.2, 1.1), color.set(PROPS.board), -1)
        break
      case 'bus-shelter':
        boxes.add(place(m, base, 0, 2.35, 0, 3.4, 0.16, 1.5), accentOf(prop), -1)
        boxes.add(place(m, base, 0, 1.2, -0.65, 3.4, 2.3, 0.12), color.set(PROPS.glass), -1)
        boxes.add(place(m, base, -1.6, 1.2, 0.6, 0.14, 2.3, 0.14), color.set(PROPS.metal), -1)
        boxes.add(place(m, base, 1.6, 1.2, 0.6, 0.14, 2.3, 0.14), color.set(PROPS.metal), -1)
        break
      case 'billboard':
        boxes.add(place(m, base, -1.7, 1.8, 0, 0.22, 3.6, 0.22), color.set(PROPS.metal), -1)
        boxes.add(place(m, base, 1.7, 1.8, 0, 0.22, 3.6, 0.22), color.set(PROPS.metal), -1)
        boxes.add(place(m, base, 0, 4.5, 0, 5.4, 2.8, 0.24), color.set('#c9456d'), -1)
        break
      case 'bench':
        boxes.add(place(m, base, 0, 0.42, 0, 1.7, 0.14, 0.55), color.set(PROPS.trunk), -1)
        boxes.add(place(m, base, 0, 0.75, -0.24, 1.7, 0.5, 0.12), color.set(PROPS.trunk), -1)
        break
      case 'tree':
        cylinders.add(place(m, base, 0, 1.1, 0, 0.4, 2.2, 0.4), color.set(PROPS.trunk), -1)
        cones.add(
          place(m, base, 0, 3.4, 0, 4.4, 4.2, 4.4),
          color.set(prop.id % 2 === 0 ? PROPS.leaf : PROPS.leafAlt),
          -1
        )
        break
      case 'water-tower': {
        for (const [x, z] of [[-2.2, -2.2], [2.2, -2.2], [2.2, 2.2], [-2.2, 2.2]] as const) {
          cylinders.add(place(m, base, x, 5, z, 0.45, 10, 0.45), color.set(PROPS.metal), -1)
        }
        cylinders.add(place(m, base, 0, 12, 0, 7, 4.4, 7), color.set(PROPS.board), -1)
        cones.add(place(m, base, 0, 15.4, 0, 7.6, 2.4, 7.6), color.set(PROPS.metal), -1)
        break
      }
      case 'beacon':
        cylinders.add(place(m, base, 0, MAST_H / 2, 0, 0.9, MAST_H, 0.9), color.set(PROPS.metal), -1)
        cylinders.add(place(m, base, 0, MAST_H * 0.35, 0, 3.2, 0.3, 3.2), color.set(PROPS.metal), -1)
        cylinders.add(place(m, base, 0, MAST_H * 0.7, 0, 2.2, 0.3, 2.2), color.set(PROPS.metal), -1)
        break
    }
  }

  // What an unclaimed lot shows: a fence and a board, so the gap in the street
  // reads as room to grow rather than as an unfinished render.
  for (const b of buildings) {
    if (b.kind !== 'empty') continue
    const lot = b.lot
    lotMatrix(base, lot.x, lot.z, lot.rotation)
    // The lot rectangle is axis-aligned but the fence is drawn in the lot's
    // own frame, so a lot facing east or west swaps its two extents.
    const sideways = Math.abs(Math.abs(lot.rotation) - Math.PI / 2) < 0.2
    const fw = (sideways ? lot.d : lot.w) - 0.6
    const fd = (sideways ? lot.w : lot.d) - 0.6
    color.set(PROPS.fence)
    boxes.add(place(m, base, 0, 0.35, fd / 2, fw, 0.7, 0.1), color, lot.id)
    boxes.add(place(m, base, 0, 0.35, -fd / 2, fw, 0.7, 0.1), color, lot.id)
    boxes.add(place(m, base, fw / 2, 0.35, 0, 0.1, 0.7, fd), color, lot.id)
    boxes.add(place(m, base, -fw / 2, 0.35, 0, 0.1, 0.7, fd), color, lot.id)
    boxes.add(place(m, base, 0, 0.9, fd / 2 - 0.5, 1.6, 0.9, 0.08), color.set(PROPS.board), lot.id)
  }

  const materials: THREE.Material[] = []
  const material = <T extends THREE.Material>(mat: T): T => {
    materials.push(mat)
    return mat
  }
  const solid = material(new THREE.MeshStandardMaterial({ roughness: 0.9 }))
  const beaconMat = material(new THREE.MeshBasicMaterial({ color: PROPS.beacon }))

  for (const part of [
    boxes.build(BOX, solid, { castShadow: true, receiveShadow: true, name: 'prop-boxes' }),
    cylinders.build(CYLINDER, solid, { castShadow: true, name: 'prop-cylinders' }),
    cones.build(CONE, solid, { castShadow: true, name: 'prop-cones' }),
  ]) {
    if (part) group.add(part.mesh)
  }

  let beacon: THREE.Mesh | null = null
  const mast = plan.props.find((p) => p.kind === 'beacon')
  if (mast) {
    beacon = new THREE.Mesh(SPHERE, beaconMat)
    beacon.position.set(mast.x, BEACON_Y, mast.z)
    beacon.scale.setScalar(1.6)
    beacon.name = 'beacon'
    group.add(beacon)
  }

  return {
    group,
    beacon,
    dispose: () => {
      for (const mat of materials) mat.dispose()
    },
  }
}

export interface CityPropsProps {
  plan: CityPlan
  buildings: CityBuilding[]
  reduced: boolean
}

export default function CityProps({ plan, buildings, reduced }: CityPropsProps) {
  const kit = useMemo(() => build(plan, buildings), [plan, buildings])
  const t = useRef(0)
  useEffect(() => () => kit.dispose(), [kit])
  useEffect(() => {
    // A beacon that does not blink still has to be lit, or the mast reads as
    // broken rather than as still.
    if (kit.beacon) kit.beacon.visible = true
  }, [kit, reduced])

  useFrame((_, dt) => {
    if (!kit.beacon || reduced) return
    t.current += dt
    kit.beacon.visible = t.current % 2 < 1.1
  })

  return <primitive object={kit.group} />
}
