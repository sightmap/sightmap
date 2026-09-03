import { Html } from '@react-three/drei'
import { useFrame, useThree } from '@react-three/fiber'
import { useMemo, useRef, useState, type CSSProperties, type ReactNode, type RefObject } from 'react'
import type * as THREE from 'three'
import { isOccluded } from './occlusion'
import { useShared } from './state'

/**
 * Breathing room kept between a label's edge and the viewport edge, so a
 * clamped label doesn't sit flush against the glass.
 */
const VIEWPORT_MARGIN = 12

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
  const wrapper = useRef<HTMLDivElement>(null)
  const since = useRef(nextPhase())
  const [occluded, setOccluded] = useState(false)

  useFrame((_, dt) => {
    since.current += dt
    if (since.current < OCCLUSION_PERIOD) return
    since.current = 0
    const tower = s.tower.current
    if (tower && anchor.current) {
      // Re-setting the same value is a no-op in React, so this re-renders on
      // a transition, not ten times a second.
      setOccluded(isOccluded(anchor.current, tower, camera, raycaster))
    }
    clampToViewport(wrapper.current)
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
        {/* A label's own box is sized by its text (`white-space: nowrap`) and
            positioned by drei from the 3D anchor, which knows nothing about
            the viewport it's projecting into. Near an edge that pushes the
            box past the glass, where .bld-overlay's own `overflow: hidden`
            used to just clip it mid-word. This wrapper is the one place
            every label passes through (Wayfinding, Table, Trajectory,
            FrontDesk, HealDemo), so the correction lives here once instead
            of five times. */}
        <div ref={wrapper} style={CLAMP_STYLE}>
          {children}
        </div>
      </Html>
    </group>
  )
}

const CLAMP_STYLE: CSSProperties = { display: 'inline-block' }

/**
 * Shifts `el` horizontally so its rendered box stays within
 * [VIEWPORT_MARGIN, innerWidth - VIEWPORT_MARGIN], without touching its font
 * size or wrapping. Anchors near the tower's right edge otherwise push their
 * box — which hangs to the right of the anchor by default — straight past
 * the glass; anchors near the left edge (memory notes, which hang left) can
 * do the same in the other direction.
 */
function clampToViewport(el: HTMLDivElement | null): void {
  if (!el) return
  // Undo any previous correction before measuring, so a label that has
  // scrolled back into bounds isn't measured with yesterday's shift baked in.
  el.style.transform = ''
  const rect = el.getBoundingClientRect()
  // clientWidth, not window.innerWidth: the page can carry a vertical
  // scrollbar (it does at 390px), and innerWidth includes that scrollbar's
  // own width while the actual content box a label can occupy does not.
  // Clamping against innerWidth let a label sit a scrollbar's-width past
  // the real right edge -- exactly the residual overflow this fix exists
  // to remove.
  const viewportWidth = document.documentElement.clientWidth
  const overflowRight = rect.right - (viewportWidth - VIEWPORT_MARGIN)
  const overflowLeft = VIEWPORT_MARGIN - rect.left
  const shift = overflowRight > 0 ? -overflowRight : overflowLeft > 0 ? overflowLeft : 0
  if (shift !== 0) el.style.transform = `translateX(${shift.toFixed(1)}px)`
}
