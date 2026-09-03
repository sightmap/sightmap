// Pure camera-framing math, extracted from Scene.tsx's `Rig` so the shadow
// frustum fit (shadowFrustum.ts) can compute exactly the same camera basis
// and target the viewer camera actually uses, without duplicating the math
// or importing React/Three's scene-graph types.
//
// The az/el/damping/drift that feed this are stateful (per-frame refs) and
// stay in Rig — this module only takes the *resolved* angles for a frame and
// turns them into a camera basis, target, and zoom.
import * as THREE from 'three'

/**
 * Above this width the desktop framing applies unchanged; below `mobile`'s
 * own 900px threshold the mobile framing takes over. In between — 1024 is
 * the audited case — the card's intermediate max-width step (building.css)
 * still leaves it wider, proportionally, than at 1440, so the tower needs to
 * sit smaller and further from the card's edge or the card lands on it and
 * crops its right face. Matches the `.bld-card` intermediate breakpoint.
 */
export const COMPACT_MAX_WIDTH = 1279

export interface FramingInputs {
  /** Camera azimuth and elevation, in degrees — already resolved for this
   *  frame (damped, with drift and pointer parallax folded in, or the raw
   *  chapter value for a consumer that wants neither). */
  az: number
  el: number
  /** The chapter's `zoom` multiplier (`SceneParams.zoom`), not the camera's
   *  resulting orthographic `zoom` — that's `Framing.zoom` below. */
  chapterZoom: number
  lookY: number
  mobile: boolean
  frame: 'tour' | 'billboard'
  width: number
  height: number
}

/** The camera basis and target for a frame, plus the orthographic zoom that
 *  frames the model at the requested viewport. All vectors are world-space. */
export interface Framing {
  /** Unit vector from the target toward the camera. */
  dir: THREE.Vector3
  /** Unit vector to the camera's right on screen. */
  right: THREE.Vector3
  /** Unit vector to the camera's up on screen. */
  up: THREE.Vector3
  /** World-space point the camera looks at. */
  target: THREE.Vector3
  zoom: number
}

/** A fresh scratch object for `computeFraming` to write into — one per
 *  caller, reused every frame. */
export function makeFraming(): Framing {
  return {
    dir: new THREE.Vector3(),
    right: new THREE.Vector3(),
    up: new THREE.Vector3(),
    target: new THREE.Vector3(),
    zoom: 1,
  }
}

/**
 * Frames the model: the table's isometric footprint is ~27 world units wide
 * and, with the tower on it, ~18 tall, whatever the viewport. The billboard
 * crops in tight on the tower instead.
 *
 * Mutates and returns `out`, so a per-frame caller (Rig, the shadow fit)
 * allocates one scratch object and reuses it instead of paying a `Vector3`
 * allocation every frame.
 */
export function computeFraming(inp: FramingInputs, out: Framing): Framing {
  const a = THREE.MathUtils.degToRad(inp.az)
  const e = THREE.MathUtils.degToRad(inp.el)

  const billboard = inp.frame === 'billboard'
  const compact = !billboard && !inp.mobile && inp.width <= COMPACT_MAX_WIDTH
  const fit = billboard
    ? Math.min(inp.width / 15, inp.height / 12.5)
    : Math.min(inp.width / 27, inp.height / 22)
  const zoom = fit * inp.chapterZoom * (inp.mobile && !billboard ? 1.3 : compact ? 0.86 : 1)

  out.dir.set(Math.cos(e) * Math.sin(a), Math.sin(e), Math.cos(e) * Math.cos(a))
  out.right.set(Math.cos(a), 0, -Math.sin(a))
  out.up.crossVectors(out.dir, out.right).normalize()

  // Desktop: the story card sits on the left, so push the model right.
  // Mobile: the card sits at the bottom, so push the model up. Compact
  // (the 900–1279px band the card's intermediate step covers): push it
  // right less than desktop does, since the narrower viewport leaves less
  // room before that push runs the tower into the card.
  const shiftRight = billboard
    ? (-inp.width * 0.07) / zoom
    : inp.mobile
      ? (-inp.width * 0.12) / zoom
      : compact
        ? (inp.width * 0.08) / zoom
        : (inp.width * 0.13) / zoom
  const shiftUp = billboard || !inp.mobile ? 0 : (inp.height * 0.11) / zoom
  out.target.set(0, inp.lookY, 0).addScaledVector(out.right, -shiftRight).addScaledVector(out.up, -shiftUp)

  out.zoom = zoom
  return out
}
