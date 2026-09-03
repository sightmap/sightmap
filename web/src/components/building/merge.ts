import * as THREE from 'three'
import { mergeGeometries } from 'three/examples/jsm/utils/BufferGeometryUtils.js'
import { toCreasedNormals } from 'three-stdlib'

// Folding many small meshes into one buffer.
//
// The tower is a few hundred untextured primitives that already share
// materials: a floor's component carpets are five colours of the same plastic,
// its two curtain walls are the same spandrel, glass and steel. Each primitive
// costs its own draw call for a handful of triangles, and casters pay again in
// the shadow pass.
//
// Merging happens per floor, never across floors. The scroll drives each
// floor's `visible` and `scale.y` ramps from the group *above* these meshes, so
// one buffer per floor collapses the call count while leaving every ramp
// untouched.
//
// Colour that used to live in a per-kind material moves into a `color` vertex
// attribute, which is why five component kinds can share one mesh. The material
// side of that (making the attribute drive emissive as well as diffuse) lives
// in Tower's `useMaterials`.

const EPS = 0.00001

/**
 * drei's `<RoundedBox>` geometry, built imperatively so it can be merged.
 *
 * Deliberately a transcription of drei's own construction — same shape, same
 * extrude parameters, same crease pass, and the same discarding of
 * `toCreasedNormals`' return value — because these replace `<RoundedBox>`
 * meshes that have to keep shading identically. Defaults match drei's.
 */
export function roundedBoxGeometry(
  width: number,
  height: number,
  depth: number,
  radius: number,
  smoothness = 4,
  bevelSegments = 4,
  creaseAngle = 0.4
): THREE.ExtrudeGeometry {
  const r = radius - EPS
  const shape = new THREE.Shape()
  shape.absarc(EPS, EPS, EPS, -Math.PI / 2, -Math.PI, true)
  shape.absarc(EPS, height - r * 2, EPS, Math.PI, Math.PI / 2, true)
  shape.absarc(width - r * 2, height - r * 2, EPS, Math.PI / 2, 0, true)
  shape.absarc(width - r * 2, EPS, EPS, 0, -Math.PI / 2, true)
  const geometry = new THREE.ExtrudeGeometry(shape, {
    depth: depth - radius * 2,
    bevelEnabled: true,
    bevelSegments: bevelSegments * 2,
    steps: 1,
    bevelSize: radius - EPS,
    bevelThickness: radius,
    curveSegments: smoothness,
  })
  geometry.center()
  toCreasedNormals(geometry, creaseAngle)
  return geometry
}

/** One primitive on its way into a merged buffer, posed in the merged mesh's space. */
export interface Part {
  geometry: THREE.BufferGeometry
  /** Offset baked into the vertices. */
  position?: readonly [number, number, number]
  /** Rotation (radians, XYZ) baked into the vertices, applied before `position`. */
  rotation?: readonly [number, number, number]
  /**
   * Written to every one of this part's vertices as a `color` attribute. Give
   * it to all parts or none: a merge needs one attribute set across the batch.
   */
  color?: THREE.Color
}

const scratch = new THREE.Matrix4()
const euler = new THREE.Euler()

/**
 * Merge `parts` into a single buffer, baking each part's pose into its vertices.
 *
 * Takes ownership of the input geometries and disposes them — callers build
 * them for the merge and have no other use for them.
 */
export function mergeParts(parts: Part[]): THREE.BufferGeometry {
  if (parts.length === 0) {
    throw new Error('mergeParts: nothing to merge')
  }

  const posed = parts.map(({ geometry, position, rotation, color }) => {
    // Non-indexed throughout: `mergeGeometries` refuses a mix, and the sources
    // here are a mix (ExtrudeGeometry is not indexed, BoxGeometry is).
    const g = geometry.index ? geometry.toNonIndexed() : geometry.clone()
    if (rotation) g.applyMatrix4(scratch.makeRotationFromEuler(euler.set(rotation[0], rotation[1], rotation[2])))
    if (position) g.translate(position[0], position[1], position[2])
    if (color) {
      const count = g.getAttribute('position').count
      const rgb = new Float32Array(count * 3)
      for (let i = 0; i < count; i++) {
        rgb[i * 3] = color.r
        rgb[i * 3 + 1] = color.g
        rgb[i * 3 + 2] = color.b
      }
      g.setAttribute('color', new THREE.BufferAttribute(rgb, 3))
    }
    return g
  })

  const merged = mergeGeometries(posed)
  for (const g of posed) g.dispose()
  for (const p of parts) p.geometry.dispose()
  if (!merged) {
    throw new Error('mergeParts: parts do not share one attribute set')
  }
  return merged
}
