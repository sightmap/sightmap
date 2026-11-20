// The kit the whole city is built from: one geometry per part kind, shared by
// every instance of it. Six primitives and a truncated pyramid per roof ratio
// is the entire model budget — everything else is a transform and a colour.
import * as THREE from 'three'

export const BOX = new THREE.BoxGeometry(1, 1, 1)
export const PLANE = new THREE.PlaneGeometry(1, 1)
/** Triangular prism, ridge along +Y before rotation; the gable and sawtooth. */
export const PRISM = new THREE.CylinderGeometry(0.5, 0.5, 1, 3)
export const DOME = new THREE.SphereGeometry(0.5, 12, 6, 0, Math.PI * 2, 0, Math.PI / 2)
export const CYLINDER = new THREE.CylinderGeometry(0.5, 0.5, 1, 10)
export const CONE = new THREE.ConeGeometry(0.5, 1, 8)
export const SPHERE = new THREE.SphereGeometry(0.5, 10, 8)

// The unit prism's triangle sits between y = -0.25 and y = 0.5 and spans
// 0.866 across, so a part of height h and width w scales by these.
export const PRISM_H = 0.75
export const PRISM_W = 0.866

const pyramids = new Map<number, THREE.CylinderGeometry>()

/** A four-sided pyramid, truncated at `topScale`; one geometry per ratio. */
export function pyramidGeometry(topScale: number): THREE.CylinderGeometry {
  let g = pyramids.get(topScale)
  if (!g) {
    g = new THREE.CylinderGeometry(0.5 * topScale, 0.5, 1, 4)
    pyramids.set(topScale, g)
  }
  return g
}

/** Every truncation ratio a roof asked for, so each can get its own mesh. */
export function pyramidRatios(): number[] {
  return [...pyramids.keys()]
}

const scratchPos = new THREE.Vector3()
const scratchQuat = new THREE.Quaternion()
const scratchScale = new THREE.Vector3()
const scratchEuler = new THREE.Euler()
const scratchLocal = new THREE.Matrix4()

/** base ∘ local, written into `out`. The base is the lot; the local is the part. */
export function place(
  out: THREE.Matrix4,
  base: THREE.Matrix4,
  x: number,
  y: number,
  z: number,
  sx: number,
  sy: number,
  sz: number,
  rx = 0,
  ry = 0,
  rz = 0
): THREE.Matrix4 {
  scratchPos.set(x, y, z)
  scratchEuler.set(rx, ry, rz)
  scratchQuat.setFromEuler(scratchEuler)
  scratchScale.set(sx, sy, sz)
  scratchLocal.compose(scratchPos, scratchQuat, scratchScale)
  return out.multiplyMatrices(base, scratchLocal)
}

/** The lot's own frame: its centre on the ground, turned to face its street. */
export function lotMatrix(out: THREE.Matrix4, x: number, z: number, rotation: number): THREE.Matrix4 {
  scratchPos.set(x, 0, z)
  scratchEuler.set(0, rotation, 0)
  scratchQuat.setFromEuler(scratchEuler)
  scratchScale.set(1, 1, 1)
  return out.compose(scratchPos, scratchQuat, scratchScale)
}
