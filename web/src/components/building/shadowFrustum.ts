// Fits the directional light's shadow-camera frustum to whatever the viewer
// camera actually has on screen, instead of the fixed ±15 box that covered
// the whole table at every zoom.
//
// Chapters 05 and 07 zoom the *viewer* camera to ~1.2x on a single floor;
// the shadow map was still spending its texels on the five floors and the
// table around them that never appear in frame. Shrinking the light's own
// frustum to the visible region raises effective shadow resolution there for
// free — no shadow-map size increase, no second caster.
import * as THREE from 'three'
import { computeFraming, makeFraming, type FramingInputs } from './framing'
import { FLOOR_D, FLOOR_H, FLOOR_W, FLOORS, TABLE } from './model'

/** The directional light's fixed position (Lights.tsx); its target is the
 *  world origin and neither ever moves — only the light's colour and
 *  intensity animate — so the shadow camera's own local axes can be derived
 *  once instead of read off a live `matrixWorld` every frame. */
const LIGHT_POS = new THREE.Vector3(13, 15, -7)
const WORLD_UP = new THREE.Vector3(0, 1, 0)

// three.js aims a light's shadow camera with `camera.lookAt(target)`, which
// derives the camera's basis from (eye - target) and the world up vector the
// same way `Object3D.lookAt` always does. Matching that convention here is
// what makes `left/right/top/bottom` below line up with the shadow camera's
// actual image plane.
const shadowZ = LIGHT_POS.clone().normalize() // (LIGHT_POS - origin), normalized
const shadowRight = new THREE.Vector3().crossVectors(WORLD_UP, shadowZ).normalize()
const shadowUp = new THREE.Vector3().crossVectors(shadowZ, shadowRight)

/** The frustum this replaces was a fixed ±15 on both axes. Nothing here is
 *  allowed to end up looser than that — only tighter — so a wide azimuth or
 *  an unanticipated case never regresses coverage below the old baseline. */
export const MAX_HALF_EXTENT = 15

// The table + tower envelope the fit is clipped against. The table (where
// chapters 00–02 fan blueprint sheets out) is wider on every side than the
// tower that rises out of its centre (FLOOR_W×FLOOR_D, not TABLE.w×TABLE.d):
// a box using the table's width all the way up the tower's height way
// overshoots the tower's actual top corners, which is why an earlier version
// of this fit pinned `top` at the ±15 safety cap on every chapter regardless
// of camera — the envelope itself, unclipped, already exceeded ±15 there.
// The envelope tapers from the table's footprint at its base to the floor
// footprint at the tower's top, matching what's actually there to shadow.
const BASE_HALF_X = TABLE.w / 2
const BASE_HALF_Z = TABLE.d / 2
const TOP_HALF_X = FLOOR_W / 2
const TOP_HALF_Z = FLOOR_D / 2
const MIN_Y = 0
// The roof (Tower.tsx's `Roof`) sits on top of the highest floor slab and
// carries its own structures — parapet posts, a solar array, a lounge, and a
// decorative sphere — the tallest of which (the lounge's sphere, radius 0.42
// at local y = SLAB_T + 0.95) apexes ~1.55 units above the top floor. Without
// this margin the envelope's top would sit below that geometry and a tighter
// frustum would start clipping its shadow — a regression the old, generous
// fixed ±15 box never had to worry about.
const ROOF_MARGIN = 1.6
const MAX_Y = FLOORS.length * FLOOR_H + ROOF_MARGIN

/** The envelope's 8 corners, fixed in world space. */
const C000 = new THREE.Vector3(-BASE_HALF_X, MIN_Y, -BASE_HALF_Z)
const C001 = new THREE.Vector3(-BASE_HALF_X, MIN_Y, BASE_HALF_Z)
const C010 = new THREE.Vector3(-TOP_HALF_X, MAX_Y, -TOP_HALF_Z)
const C011 = new THREE.Vector3(-TOP_HALF_X, MAX_Y, TOP_HALF_Z)
const C100 = new THREE.Vector3(BASE_HALF_X, MIN_Y, -BASE_HALF_Z)
const C101 = new THREE.Vector3(BASE_HALF_X, MIN_Y, BASE_HALF_Z)
const C110 = new THREE.Vector3(TOP_HALF_X, MAX_Y, -TOP_HALF_Z)
const C111 = new THREE.Vector3(TOP_HALF_X, MAX_Y, TOP_HALF_Z)

/** The envelope's 6 faces, each vertices in perimeter order, for clipping
 *  against the camera's view half-planes (see `fitShadowFrustum`). */
const ENVELOPE_FACES: readonly (readonly THREE.Vector3[])[] = [
  [C000, C001, C011, C010], // -X
  [C100, C101, C111, C110], // +X
  [C000, C100, C101, C001], // -Y
  [C010, C011, C111, C110], // +Y
  [C000, C100, C110, C010], // -Z
  [C001, C101, C111, C011], // +Z
]

const ENVELOPE_CORNERS: readonly THREE.Vector3[] = [C000, C001, C010, C011, C100, C101, C110, C111]

/** Padding added to the tight fit, so a filtered shadow sample just inside
 *  the visible edge doesn't clip against the frustum boundary. */
const MARGIN = 1.5

export interface ShadowBounds {
  left: number
  right: number
  top: number
  bottom: number
}

/** Reusable scratch: one `fitShadowFrustum` caller owns one of these. */
export interface ShadowFitScratch {
  framing: ReturnType<typeof makeFraming>
}

export function makeShadowFitScratch(): ShadowFitScratch {
  return { framing: makeFraming() }
}

/** A single half-space clip plane: a point `v` is inside when
 *  `v.dot(normal) <= limit`. */
interface ClipPlane {
  normal: THREE.Vector3
  limit: number
}

/** Sutherland–Hodgman: clips a (planar, convex) polygon against one
 *  half-space, keeping the inside portion and interpolating new vertices at
 *  the boundary crossings. */
function clipPolygon(poly: readonly THREE.Vector3[], plane: ClipPlane): THREE.Vector3[] {
  if (poly.length === 0) return []
  const out: THREE.Vector3[] = []
  for (let i = 0; i < poly.length; i++) {
    const current = poly[i]
    const next = poly[(i + 1) % poly.length]
    const curDist = current.dot(plane.normal) - plane.limit
    const nextDist = next.dot(plane.normal) - plane.limit
    if (curDist <= 0) out.push(current)
    if (curDist <= 0 !== nextDist <= 0) {
      out.push(current.clone().lerp(next, curDist / (curDist - nextDist)))
    }
  }
  return out
}

/**
 * Fits the shadow frustum to what the viewer camera actually sees: clips the
 * table/tower envelope's 6 faces against the camera's own screen-space
 * half-planes (`|s| <= halfW`, `|t| <= halfH`, in the camera's `right`/`up`
 * axes), then bounds whatever survives in the shadow light's local axes.
 * This is the exact envelope ∩ view-slab intersection, not an approximation
 * — an orthographic camera's view is unbounded in depth (no far-plane taper
 * the way a perspective camera has), so the intersection with the envelope
 * really is just those two screen-space half-planes.
 *
 * Two simpler approaches were tried and both failed to track zoom at all:
 * clamping the *camera's* frustum corners into the envelope's world axes
 * collapses to the envelope's own bounds regardless of camera angle (a wide
 * enough depth sweep to guarantee coverage always overshoots on every side);
 * clamping only the envelope's 8 *corners* into the camera's view (dropping
 * the depth axis) leaves each corner's full, unclamped depth contribution in
 * the result, which swamps the screen-space clamp and pins the fit to the
 * ±15 safety cap on every chapter. Clipping the envelope's faces — not just
 * its corners — against the view is what actually shrinks with zoom, because
 * a tighter view slab cuts a smaller cross-section out of the envelope on
 * every face, corners included.
 *
 * Takes the *raw* chapter az/el/zoom/lookY, not the Rig's damped, drifting
 * copy: a shadow frustum that wobbled with the idle drift or flinched at the
 * pointer would be a worse light, not a tighter one.
 */
export function fitShadowFrustum(inp: FramingInputs, scratch: ShadowFitScratch): ShadowBounds {
  const { framing } = scratch
  computeFraming(inp, framing)

  const halfW = inp.width / (2 * framing.zoom)
  const halfH = inp.height / (2 * framing.zoom)
  const targetRight = framing.target.dot(framing.right)
  const targetUp = framing.target.dot(framing.up)

  const planes: ClipPlane[] = [
    { normal: framing.right, limit: targetRight + halfW }, // s <= halfW
    { normal: framing.right.clone().negate(), limit: -targetRight + halfW }, // s >= -halfW
    { normal: framing.up, limit: targetUp + halfH }, // t <= halfH
    { normal: framing.up.clone().negate(), limit: -targetUp + halfH }, // t >= -halfH
  ]

  let left = Infinity
  let right = -Infinity
  let top = -Infinity
  let bottom = Infinity
  let sawVertex = false

  for (const face of ENVELOPE_FACES) {
    let clipped: THREE.Vector3[] = face.slice()
    for (const plane of planes) clipped = clipPolygon(clipped, plane)
    for (const v of clipped) {
      sawVertex = true
      const lx = v.dot(shadowRight)
      const ly = v.dot(shadowUp)
      if (lx < left) left = lx
      if (lx > right) right = lx
      if (ly < bottom) bottom = ly
      if (ly > top) top = ly
    }
  }

  // The camera view should always overlap the envelope in practice (every
  // chapter frames the table). If it somehow doesn't, fall back to the full
  // envelope rather than shipping a degenerate empty frustum.
  if (!sawVertex) {
    for (const c of ENVELOPE_CORNERS) {
      const lx = c.dot(shadowRight)
      const ly = c.dot(shadowUp)
      if (lx < left) left = lx
      if (lx > right) right = lx
      if (ly < bottom) bottom = ly
      if (ly > top) top = ly
    }
  }

  return {
    left: Math.max(left - MARGIN, -MAX_HALF_EXTENT),
    right: Math.min(right + MARGIN, MAX_HALF_EXTENT),
    top: Math.min(top + MARGIN, MAX_HALF_EXTENT),
    bottom: Math.max(bottom - MARGIN, -MAX_HALF_EXTENT),
  }
}
