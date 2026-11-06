// Instancing, the way this city needs it: one mesh per part kind, a colour per
// instance, and a note of which lot each instance belongs to so a single
// building can be taken out of the crowd when the camera flies down to it.
import * as THREE from 'three'

export interface InstancedPart {
  mesh: THREE.InstancedMesh
  /** Lot id per instance, or -1 for something that belongs to no lot. */
  owners: Int32Array
  /** The matrices as built, so a hidden instance can be put back. */
  base: Float32Array
}

export interface PartOptions {
  castShadow?: boolean
  receiveShadow?: boolean
  renderOrder?: number
  visible?: boolean
  name?: string
}

/** Collects instances, then hands over one mesh. */
export class PartBuilder {
  private readonly matrices: number[] = []
  private readonly colors: number[] = []
  private readonly owners: number[] = []

  get count(): number {
    return this.owners.length
  }

  add(matrix: THREE.Matrix4, color: THREE.Color | null, owner: number): void {
    for (let i = 0; i < 16; i++) this.matrices.push(matrix.elements[i])
    if (color) this.colors.push(color.r, color.g, color.b)
    else this.colors.push(1, 1, 1)
    this.owners.push(owner)
  }

  build(
    geometry: THREE.BufferGeometry,
    material: THREE.Material,
    opts: PartOptions = {}
  ): InstancedPart | null {
    const n = this.owners.length
    if (n === 0) return null
    const mesh = new THREE.InstancedMesh(geometry, material, n)
    const base = new Float32Array(this.matrices)
    mesh.instanceMatrix.array.set(base)
    mesh.instanceMatrix.needsUpdate = true
    mesh.instanceColor = new THREE.InstancedBufferAttribute(new Float32Array(this.colors), 3)
    mesh.instanceColor.needsUpdate = true
    mesh.castShadow = opts.castShadow ?? false
    mesh.receiveShadow = opts.receiveShadow ?? false
    mesh.visible = opts.visible ?? true
    if (opts.renderOrder !== undefined) mesh.renderOrder = opts.renderOrder
    if (opts.name) mesh.name = opts.name
    mesh.computeBoundingSphere()
    // Frustum culling is per mesh, and every one of these covers the whole
    // city, so it only pays off on the parts that do not: it stays on so a
    // camera down at street level can still drop what is behind it.
    mesh.frustumCulled = true
    return { mesh, owners: new Int32Array(this.owners), base }
  }
}

/**
 * Takes one lot's instances out of every part and puts them back afterwards,
 * so the closed building can step aside for its own dollhouse.
 */
export class LotHider {
  private hidden: number | null = null

  constructor(private readonly parts: InstancedPart[]) {}

  set(lot: number | null): void {
    if (lot === this.hidden) return
    if (this.hidden !== null) this.apply(this.hidden, true)
    if (lot !== null) this.apply(lot, false)
    this.hidden = lot
  }

  private apply(lot: number, restore: boolean): void {
    for (const part of this.parts) {
      const array = part.mesh.instanceMatrix.array as Float32Array
      let touched = false
      for (let i = 0; i < part.owners.length; i++) {
        if (part.owners[i] !== lot) continue
        const o = i * 16
        if (restore) array.set(part.base.subarray(o, o + 16), o)
        else for (let k = 0; k < 16; k++) array[o + k] = 0
        touched = true
      }
      if (touched) part.mesh.instanceMatrix.needsUpdate = true
    }
  }
}
