import * as THREE from 'three'
import type { RoofPart } from './facade'

// The roof, drawn from `facade.ts`'s part list. Shared, because the same
// shape has to sit on the closed building the city instances and on the open
// dollhouse a listing page shows: a bank keeps its dome either way.

const unitBox = new THREE.BoxGeometry(1, 1, 1)
/** Triangular prism, ridge along +Y before rotation. */
const prismGeom = new THREE.CylinderGeometry(0.5, 0.5, 1, 3)
const domeGeom = new THREE.SphereGeometry(0.5, 14, 8, 0, Math.PI * 2, 0, Math.PI / 2)

// The unit prism's triangle sits between y = -0.25 and y = 0.5 and spans
// 0.866 across, so a part of height h and width w scales by these.
const PRISM_H = 0.75
const PRISM_W = 0.866

const WALL_T = 0.18

// A truncated pyramid per top ratio, built once and shared.
const pyramids = new Map<number, THREE.CylinderGeometry>()
function pyramidGeom(topScale: number): THREE.CylinderGeometry {
  let g = pyramids.get(topScale)
  if (!g) {
    g = new THREE.CylinderGeometry(0.5 * topScale, 0.5, 1, 4)
    pyramids.set(topScale, g)
  }
  return g
}

export interface RoofShapeProps {
  parts: RoofPart[]
  /** Height of the wall head the roof sits on. */
  y: number
  mat: THREE.Material
  trim: THREE.Material
  /** Drop the deck the walls already carry (the dollhouse draws its own). */
  skipDeck?: boolean
}

export default function RoofShape({ parts, y, mat, trim, skipDeck = false }: RoofShapeProps) {
  return (
    <group position={[0, y, 0]}>
      {parts.map((part, k) => {
        switch (part.kind) {
          case 'slab':
            if (skipDeck && k === 0) return null
            return (
              <mesh key={k} geometry={unitBox} material={mat} position={[0, part.y + part.h / 2, 0]} scale={[part.w, part.h, part.d]} castShadow receiveShadow />
            )
          case 'parapet':
            return (
              <group key={k}>
                {([
                  [0, part.d / 2 - WALL_T / 2, part.w, WALL_T],
                  [0, -part.d / 2 + WALL_T / 2, part.w, WALL_T],
                ] as const).map(([x, z, w, d], j) => (
                  <mesh key={`z${j}`} geometry={unitBox} material={trim} position={[x, part.y + part.h / 2, z]} scale={[w, part.h, d]} castShadow />
                ))}
                {([part.w / 2 - WALL_T / 2, -part.w / 2 + WALL_T / 2] as const).map((x, j) => (
                  <mesh key={`x${j}`} geometry={unitBox} material={trim} position={[x, part.y + part.h / 2, 0]} scale={[WALL_T, part.h, part.d]} castShadow />
                ))}
              </group>
            )
          case 'prism':
            return (
              <mesh
                key={k}
                geometry={prismGeom}
                material={mat}
                position={[part.x, part.y + part.h / 3, part.z]}
                rotation={part.along === 'x' ? [-Math.PI / 2, 0, -Math.PI / 2] : [-Math.PI / 2, 0, 0]}
                scale={
                  part.along === 'x'
                    ? [part.w, part.h / PRISM_H, part.d / PRISM_W]
                    : [part.w / PRISM_W, part.h / PRISM_H, part.d]
                }
                castShadow
              />
            )
          case 'pyramid':
            return (
              <mesh
                key={k}
                geometry={pyramidGeom(part.topScale)}
                material={mat}
                position={[0, part.y + part.h / 2, 0]}
                rotation={[0, Math.PI / 4, 0]}
                scale={[part.w * Math.SQRT2, part.h, part.d * Math.SQRT2]}
                castShadow
              />
            )
          case 'dome':
            return (
              <mesh
                key={k}
                geometry={domeGeom}
                material={mat}
                position={[0, part.y, 0]}
                scale={[part.r * 2, part.h * 2, part.r * 2]}
                castShadow
              />
            )
        }
      })}
    </group>
  )
}
