import { describe, expect, it } from 'vitest'
import * as THREE from 'three'
import { isDrawn, isOccluded } from './occlusion'

// A stand-in for the tower: one slab, parented through a group, the way a real
// floor's geometry hangs off the FloorUnit group the scene toggles.
function fixture() {
  const camera = new THREE.OrthographicCamera(-5, 5, 5, -5, 0.1, 100)
  camera.position.set(0, 0, 10)
  camera.lookAt(0, 0, 0)
  camera.updateMatrixWorld(true)
  camera.updateProjectionMatrix()

  const slab = new THREE.Mesh(new THREE.BoxGeometry(4, 0.5, 4), new THREE.MeshBasicMaterial())
  const floor = new THREE.Group()
  floor.add(slab)
  const tower = new THREE.Group()
  tower.add(floor)
  tower.updateMatrixWorld(true)

  return { camera, tower, floor, slab, raycaster: new THREE.Raycaster() }
}

/** A label's anchor, at a world position. */
function anchorAt(x: number, y: number, z: number): THREE.Object3D {
  const o = new THREE.Object3D()
  o.position.set(x, y, z)
  return o
}

describe('isOccluded', () => {
  it('hides a label whose anchor is behind a slab', () => {
    const { camera, tower, raycaster } = fixture()
    expect(isOccluded(anchorAt(0, 0, -3), tower, camera, raycaster)).toBe(true)
  })

  it('leaves a label in front of the slab alone', () => {
    const { camera, tower, raycaster } = fixture()
    expect(isOccluded(anchorAt(0, 0, 5), tower, camera, raycaster)).toBe(false)
  })

  it('leaves a label alone when nothing is on the ray at all', () => {
    const { camera, tower, raycaster } = fixture()
    expect(isOccluded(anchorAt(4, 0, -3), tower, camera, raycaster)).toBe(false)
  })

  it('ignores a hidden blocker', () => {
    const { camera, tower, slab, raycaster } = fixture()
    slab.visible = false
    expect(isOccluded(anchorAt(0, 0, -3), tower, camera, raycaster)).toBe(false)
  })

  // The tower hides whole floors with `visible = false` while it rises, and
  // three's raycaster reports hits on them regardless. A floor that has not
  // been built yet must not occlude anything.
  it('ignores a blocker inside a hidden group', () => {
    const { camera, tower, floor, raycaster } = fixture()
    floor.visible = false
    expect(isOccluded(anchorAt(0, 0, -3), tower, camera, raycaster)).toBe(false)
  })

  it('treats the surface an anchor rests on as contact, not occlusion', () => {
    const { camera, tower, raycaster } = fixture()
    // Just above the slab's top face (y = 0.25), as a room tag sits just above
    // the zone block it names.
    expect(isOccluded(anchorAt(0, 0.27, 0), tower, camera, raycaster)).toBe(false)
  })
})

describe('isDrawn', () => {
  it('walks up to the root looking for a hidden ancestor', () => {
    const { tower, floor, slab } = fixture()
    expect(isDrawn(slab, tower)).toBe(true)
    floor.visible = false
    expect(isDrawn(slab, tower)).toBe(false)
  })

  it('stops at the root rather than following parents past it', () => {
    const { tower, slab } = fixture()
    const stage = new THREE.Group()
    stage.add(tower)
    stage.visible = false
    expect(isDrawn(slab, tower)).toBe(true)
  })
})
