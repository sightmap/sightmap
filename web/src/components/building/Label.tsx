import { Html } from '@react-three/drei'
import type { ReactNode, RefObject } from 'react'
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
// Wrapping drei's Html rather than repeating `portal` at five call sites keeps
// the one thing every label needs in one place.

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

/** Labels never take pointer input; the overlay is inert by design. */
const INERT = { pointerEvents: 'none' } as const

export default function Label({ position, zIndexRange, center, children }: LabelProps) {
  const s = useShared()
  return (
    <Html
      position={position}
      zIndexRange={zIndexRange}
      center={center}
      style={INERT}
      // drei types `portal` as a non-null RefObject, but its implementation
      // reads `portal?.current || <canvas container>` and handles an empty one.
      // Ours is empty until the page mounts, and stays empty on the homepage
      // billboard, which has no overlay and wants the old in-canvas behaviour.
      portal={s.overlay as RefObject<HTMLElement>}
    >
      {children}
    </Html>
  )
}
