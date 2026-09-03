import * as THREE from 'three'

// Depth-testing DOM labels against the model.
//
// A label is DOM, so the depth buffer means nothing to it: with no test at all
// it paints over whatever geometry happens to stand in front of its anchor.
// The test is the one drei uses — raycast from the camera through the anchor's
// screen position, and if something is nearer than the anchor, the anchor is
// behind it.
//
// We run those semantics ourselves rather than handing drei `occlude={[ref]}`,
// for three reasons that all bite on this page:
//
//   * drei raycasts on every frame with no way to slow it down. One raycast
//     here walks six floors of instanced furniture, and there are 26 labels.
//   * drei rewrites zIndexRange to its top half whenever occlude is set, which
//     would collapse the 4 / 6 / 7 ordering the labels depend on.
//   * three's raycaster reports hits on objects with `visible = false`, and the
//     tower hides whole floors while it rises — so floors that have not been
//     built yet would occlude the labels above them.
//
// Only the last of those changes the answer; the first two change what it costs
// and what else it breaks.

/** Hits this close to the anchor are the surface it sits on, not a blocker. */
const CONTACT_BIAS = 0.05

const anchorPos = new THREE.Vector3()
const projected = new THREE.Vector3()
const ndc = new THREE.Vector2()

/**
 * Is `node` actually drawn — that is, is it and every ancestor up to `root`
 * visible? three's raycaster ignores `visible`, so this is the caller's job.
 */
export function isDrawn(node: THREE.Object3D, root: THREE.Object3D): boolean {
  for (let o: THREE.Object3D | null = node; o; o = o.parent) {
    if (!o.visible) return false
    if (o === root) break
  }
  return true
}

/**
 * Is any drawn part of `blocker` between the camera and `anchor`?
 *
 * Mutates the raycaster, and reuses module-level scratch vectors — call it from
 * one place at a time, which the frame loop guarantees.
 */
export function isOccluded(
  anchor: THREE.Object3D,
  blocker: THREE.Object3D,
  camera: THREE.Camera,
  raycaster: THREE.Raycaster
): boolean {
  anchor.updateWorldMatrix(true, false)
  anchorPos.setFromMatrixPosition(anchor.matrixWorld)
  projected.copy(anchorPos).project(camera)
  ndc.set(projected.x, projected.y)
  raycaster.setFromCamera(ndc, camera)

  const reach = anchorPos.distanceTo(raycaster.ray.origin) - CONTACT_BIAS
  // Hits arrive sorted by distance, so the first one at or past the anchor ends
  // the search: nothing behind it can be in front of the label.
  for (const hit of raycaster.intersectObject(blocker, true)) {
    if (hit.distance >= reach) return false
    if (isDrawn(hit.object, blocker)) return true
  }
  return false
}
