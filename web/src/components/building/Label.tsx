import { Html } from '@react-three/drei'
import { useFrame, useThree } from '@react-three/fiber'
import { useMemo, useRef, useState, type CSSProperties, type ReactNode, type RefObject } from 'react'
import type * as THREE from 'three'
import { isOccluded } from './occlusion'
import { useShared } from './state'

// A label anchored in 3D and drawn as DOM.
//
// Every one of these has to render *outside* .bld-stage. The stage is
// `position: fixed`, which makes it a stacking context on its own, so a label
// parented inside it paints at the stage's level however high a z-index it
// asks for — which is why the zIndexRange values below have never had any
// effect against the page chrome. drei's `portal` moves the label's DOM into
// .bld-overlay, a sibling of the chrome, while the 3D anchoring keeps working
// exactly as before: the position is still computed from this component's
// place in the scene graph.
//
// It also has to be depth-tested. Without one, a label whose anchor sits behind
// a floor slab still paints over it, because the DOM knows nothing about the
// depth buffer. See occlusion.ts for the test, and for why drei's own `occlude`
// prop is not the one we use.
//
// Wrapping drei's Html rather than repeating all of this at five call sites
// keeps the one thing every label needs in one place.

/**
 * How often a label re-checks whether it is behind something. Each check is a
 * raycast through six floors of instanced furniture; at 26 labels, doing that
 * every frame is real time taken from a main thread that already runs ~57
 * useFrame callbacks and rewrites DOM transforms on every one of them. Ten a
 * second is fast enough that the camera's slow drift never shows the seam.
 */
const OCCLUSION_HZ = 10
const OCCLUSION_PERIOD = 1 / OCCLUSION_HZ

/**
 * Starting offset for a label's throttle clock, so the 26 checks spread across
 * the window instead of landing together on every sixth frame. Golden-ratio
 * steps stay evenly spread for any number of labels.
 */
let phase = 0
function nextPhase(): number {
  phase = (phase + 0.6180339887) % 1
  return phase * OCCLUSION_PERIOD
}

export interface LabelProps {
  /** Anchor, in world space. */
  position: [number, number, number]
  /**
   * Paint order among labels, handed straight to drei. The spread the page
   * uses is floor tags (4) under route tags (6) under status chips (7); it is
   * a relative ordering inside .bld-overlay, which itself sits at z-index 10.
   */
  zIndexRange: [number, number]
  /** Centre the label's box on the anchor instead of hanging it below-right. */
  center?: boolean
  children: ReactNode
}

export default function Label({ position, zIndexRange, center, children }: LabelProps) {
  const s = useShared()
  const { camera, raycaster } = useThree()
  const anchor = useRef<THREE.Group>(null)
  const since = useRef(nextPhase())
  const [occluded, setOccluded] = useState(false)

  useFrame((_, dt) => {
    since.current += dt
    if (since.current < OCCLUSION_PERIOD) return
    since.current = 0
    const tower = s.tower.current
    if (!tower || !anchor.current) return
    // Re-setting the same value is a no-op in React, so this re-renders on a
    // transition, not ten times a second.
    setOccluded(isOccluded(anchor.current, tower, camera, raycaster))
  })

  // `display`, not `visibility`: several call sites drive their own child's
  // `visibility` every frame to fade labels in, and a descendant can override
  // an inherited `visibility: hidden`. Labels never take pointer input either —
  // the overlay layer is inert by design.
  const style = useMemo<CSSProperties>(
    () => ({ pointerEvents: 'none', display: occluded ? 'none' : undefined }),
    [occluded]
  )

  return (
    <group ref={anchor} position={position}>
      <Html
        zIndexRange={zIndexRange}
        center={center}
        style={style}
        // drei types `portal` as a non-null RefObject, but its implementation
        // reads `portal?.current || <canvas container>` and handles an empty
        // one. Ours is empty until the page mounts, and stays empty on the
        // homepage billboard, which has no overlay and wants the old in-canvas
        // behaviour.
        portal={s.overlay as RefObject<HTMLElement>}
      >
        {children}
      </Html>
    </group>
  )
}
