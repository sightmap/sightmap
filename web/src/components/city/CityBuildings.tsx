// Every massing in the city, as one instanced mesh per part kind. A Shell per
// lot with an HTML sign over it is what the listing page can afford for one
// building; three hundred of them would be three hundred DOM nodes and as many
// draw calls, so the same parts — walls, windows, roof, sign, the Sightkick
// mark — are collected here and drawn in a dozen.
import { useEffect, useMemo } from 'react'
import * as THREE from 'three'
import { roofParts } from '@/components/building/facade'
import { FOOTPRINT, cityWindows, type CityBuilding, type CityWindow } from './buildings'
import { BOX, DOME, PLANE, PRISM, PRISM_H, PRISM_W, CYLINDER, lotMatrix, place, pyramidGeometry } from './geometry'
import { LotHider, PartBuilder, type InstancedPart } from './instancing'
import { WINDOW } from './palette'
import { SIGN_HEIGHT, buildSignMesh, buildSignTexture, type SignInstance } from './signTexture'

const PARAPET_T = 0.18
/** How far a lit pane sits in front of its glass, so it never z-fights. */
const LIT_OFFSET = 0.04

interface Built {
  group: THREE.Group
  hider: LotHider
  lit: THREE.InstancedMesh | null
  dispose: () => void
}

const FACE_ROTATION = [0, Math.PI / 2, Math.PI, -Math.PI / 2]

function windowMatrix(out: THREE.Matrix4, base: THREE.Matrix4, w: CityWindow, push: number): THREE.Matrix4 {
  const nx = w.face === 1 ? 1 : w.face === 3 ? -1 : 0
  const nz = w.face === 0 ? 1 : w.face === 2 ? -1 : 0
  return place(
    out,
    base,
    w.x + nx * push,
    w.y,
    w.z + nz * push,
    1.25,
    1.15,
    1,
    0,
    FACE_ROTATION[w.face],
    0
  )
}

function build(buildings: CityBuilding[]): Built {
  const group = new THREE.Group()
  group.name = 'city-buildings'

  const walls = new PartBuilder()
  const roofBoxes = new PartBuilder()
  const prisms = new PartBuilder()
  const domes = new PartBuilder()
  const pyramids = new Map<number, PartBuilder>()
  const glass = new PartBuilder()
  const lit = new PartBuilder()
  const doors = new PartBuilder()
  const awnings = new PartBuilder()
  const marks = new PartBuilder()
  const signs: SignInstance[] = []

  const base = new THREE.Matrix4()
  const m = new THREE.Matrix4()
  const wall = new THREE.Color()
  const roofColor = new THREE.Color()
  const accentColor = new THREE.Color()
  const panes: CityWindow[] = []
  const signTexts = buildings.filter((b) => b.kind === 'listing' && b.sign).map((b) => b.sign as string)
  const sign = buildSignTexture(signTexts)

  for (const b of buildings) {
    const lot = b.lot
    lotMatrix(base, lot.x, lot.z, lot.rotation)
    if (b.height <= 0) continue
    wall.set(b.wall)
    roofColor.copy(wall).multiplyScalar(0.62)

    walls.add(place(m, base, 0, b.height / 2, 0, FOOTPRINT.w, b.height, FOOTPRINT.d), wall, lot.id)

    cityWindows(b, panes)
    for (const pane of panes) {
      glass.add(windowMatrix(m, base, pane, 0), null, lot.id)
      if (pane.lit) lit.add(windowMatrix(m, base, pane, LIT_OFFSET), null, lot.id)
    }

    doors.add(place(m, base, 0, 0.95, FOOTPRINT.d / 2 + 0.02, 1.8, 1.9, 0.16), null, lot.id)

    if (b.kind === 'listing') {
      accentColor.set(b.accent)
      awnings.add(
        place(m, base, 0, 2.15, FOOTPRINT.d / 2 + 0.3, FOOTPRINT.w * 0.72, 0.2, 0.7),
        accentColor,
        lot.id
      )
      if (b.sightkick) {
        marks.add(
          place(m, base, -2.4, 1.5, FOOTPRINT.d / 2 + 0.08, 0.52, 0.14, 0.52, Math.PI / 2),
          null,
          lot.id
        )
      }
      const region = b.sign ? sign?.atlas.index.get(b.sign) : undefined
      if (sign && region !== undefined) {
        const r = sign.atlas.regions[region]
        const width = (SIGN_HEIGHT * r.width) / r.height
        signs.push({
          region,
          lot: lot.id,
          matrix: place(
            new THREE.Matrix4(),
            base,
            0,
            Math.max(b.height - 1.1, 2.9),
            FOOTPRINT.d / 2 + 0.12,
            Math.min(width, FOOTPRINT.w - 0.6),
            SIGN_HEIGHT,
            1
          ),
        })
      }
    }

    if (!b.roof) continue
    for (const part of roofParts(b.roof, FOOTPRINT.w, FOOTPRINT.d)) {
      const y = b.height
      switch (part.kind) {
        case 'slab':
          roofBoxes.add(place(m, base, 0, y + part.y + part.h / 2, 0, part.w, part.h, part.d), roofColor, lot.id)
          break
        case 'parapet': {
          const half = part.h / 2
          roofBoxes.add(
            place(m, base, 0, y + part.y + half, part.d / 2 - PARAPET_T / 2, part.w, part.h, PARAPET_T),
            roofColor,
            lot.id
          )
          roofBoxes.add(
            place(m, base, 0, y + part.y + half, -part.d / 2 + PARAPET_T / 2, part.w, part.h, PARAPET_T),
            roofColor,
            lot.id
          )
          roofBoxes.add(
            place(m, base, part.w / 2 - PARAPET_T / 2, y + part.y + half, 0, PARAPET_T, part.h, part.d),
            roofColor,
            lot.id
          )
          roofBoxes.add(
            place(m, base, -part.w / 2 + PARAPET_T / 2, y + part.y + half, 0, PARAPET_T, part.h, part.d),
            roofColor,
            lot.id
          )
          break
        }
        case 'prism':
          prisms.add(
            part.along === 'x'
              ? place(m, base, part.x, y + part.y + part.h / 3, part.z, part.w, part.h / PRISM_H, part.d / PRISM_W, -Math.PI / 2, 0, -Math.PI / 2)
              : place(m, base, part.x, y + part.y + part.h / 3, part.z, part.w / PRISM_W, part.h / PRISM_H, part.d, -Math.PI / 2, 0, 0),
            roofColor,
            lot.id
          )
          break
        case 'pyramid': {
          let builder = pyramids.get(part.topScale)
          if (!builder) {
            builder = new PartBuilder()
            pyramids.set(part.topScale, builder)
          }
          pyramidGeometry(part.topScale)
          builder.add(
            place(m, base, 0, y + part.y + part.h / 2, 0, part.w * Math.SQRT2, part.h, part.d * Math.SQRT2, 0, Math.PI / 4, 0),
            roofColor,
            lot.id
          )
          break
        }
        case 'dome':
          domes.add(place(m, base, 0, y + part.y, 0, part.r * 2, part.h * 2, part.r * 2), roofColor, lot.id)
          break
      }
    }
  }

  const materials: THREE.Material[] = []
  const material = <T extends THREE.Material>(mat: T): T => {
    materials.push(mat)
    return mat
  }
  const wallMat = material(new THREE.MeshStandardMaterial({ roughness: 0.95 }))
  const roofMat = material(new THREE.MeshStandardMaterial({ roughness: 0.9 }))
  const glassMat = material(new THREE.MeshStandardMaterial({ color: WINDOW.glass, roughness: 0.3, metalness: 0.05 }))
  const litMat = material(new THREE.MeshBasicMaterial({ color: WINDOW.lit }))
  const doorMat = material(new THREE.MeshStandardMaterial({ color: '#2b2d33', roughness: 0.7 }))
  const accentMat = material(new THREE.MeshStandardMaterial({ roughness: 0.8 }))
  const markMat = material(
    new THREE.MeshStandardMaterial({ color: '#c9456d', emissive: new THREE.Color('#c9456d'), emissiveIntensity: 0.5, roughness: 0.4 })
  )

  const parts: InstancedPart[] = []
  const add = (part: InstancedPart | null) => {
    if (!part) return null
    parts.push(part)
    group.add(part.mesh)
    return part
  }

  add(walls.build(BOX, wallMat, { castShadow: true, receiveShadow: true, name: 'walls' }))
  add(roofBoxes.build(BOX, roofMat, { castShadow: true, receiveShadow: true, name: 'roofs' }))
  add(prisms.build(PRISM, roofMat, { castShadow: true, name: 'roof-prisms' }))
  add(domes.build(DOME, roofMat, { castShadow: true, name: 'roof-domes' }))
  for (const [topScale, builder] of pyramids) {
    add(builder.build(pyramidGeometry(topScale), roofMat, { castShadow: true, name: `roof-pyramids-${topScale}` }))
  }
  add(glass.build(PLANE, glassMat, { name: 'windows' }))
  const litPart = add(lit.build(PLANE, litMat, { visible: false, name: 'windows-lit' }))
  add(doors.build(BOX, doorMat, { name: 'doors' }))
  add(awnings.build(BOX, accentMat, { castShadow: true, name: 'awnings' }))
  add(marks.build(CYLINDER, markMat, { name: 'sightkick' }))

  const signPart = sign ? buildSignMesh(sign, signs) : null
  if (signPart) {
    signPart.mesh.name = 'signs'
    parts.push(signPart)
    group.add(signPart.mesh)
  }

  return {
    group,
    hider: new LotHider(parts),
    lit: litPart?.mesh ?? null,
    dispose: () => {
      for (const mat of materials) mat.dispose()
      sign?.texture.dispose()
      if (signPart) {
        signPart.mesh.geometry.dispose()
        ;(signPart.mesh.material as THREE.Material).dispose()
      }
    },
  }
}

export interface CityBuildingsProps {
  buildings: CityBuilding[]
  night: boolean
  /** The lot whose closed building steps aside for its dollhouse. */
  hiddenLot: number | null
}

export default function CityBuildings({ buildings, night, hiddenLot }: CityBuildingsProps) {
  const built = useMemo(() => build(buildings), [buildings])
  useEffect(() => () => built.dispose(), [built])
  useEffect(() => built.hider.set(hiddenLot), [built, hiddenLot])
  useEffect(() => {
    if (built.lit) built.lit.visible = night
  }, [built, night])
  return <primitive object={built.group} />
}
