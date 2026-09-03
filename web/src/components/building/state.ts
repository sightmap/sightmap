// Mutable, frame-rate state shared between the page (which owns scroll and
// pointer) and the scene (which reads them every frame). Deliberately not
// React state: scroll fires far more often than React should re-render, and
// the scene only needs the latest value when it draws.
import { createContext, useContext, useEffect, useMemo, type RefObject } from 'react'
import type * as THREE from 'three'
import { defaultParams, type SceneParams } from './chapters'

/**
 * A person the instanced crowd draws on someone else's behalf. The scripted
 * vignettes (the front-desk visitor, the self-healing test) animate one figure
 * each on their own schedule, so they own the numbers and People.tsx owns the
 * meshes — that way there is still exactly one body-part system in the scene.
 * Plain numbers rather than an Object3D: the slot crosses from a vignette into
 * the crowd's instance matrices, and never needs a node of its own.
 */
export interface PersonSlot {
  x: number
  y: number
  z: number
  /** Heading in radians; 0 faces +Z, matching `atan2(dx, dz)`. */
  ry: number
  scale: number
  visible: boolean
  /** Shirt colour; the crowd derives trousers from it. */
  color: string
}

export function createPersonSlot(color: string): PersonSlot {
  return { x: 0, y: 0, z: 0, ry: 0, scale: 1, visible: false, color }
}

export interface SharedState {
  /** Continuous chapter position, 0 … CHAPTERS.length-1. */
  progress: number
  /** Pointer in [-1, 1] each axis, for parallax. */
  pointer: { x: number; y: number }
  /** Damped scene parameters, written by Scene's updater each frame. */
  cur: SceneParams
  /** Scratch target, refilled each frame. */
  target: SceneParams
  /** Self-healing demo: how far ContinueButton has slid to its new spot. */
  healShift: number
  /** Journey the crowd is reduced to (null = everyone). */
  focus: string | null
  /** Figures driven by a scripted vignette, drawn by the shared crowd meshes. */
  slots: PersonSlot[]
  /**
   * The quality tier, for the per-frame readers only (the Rig's camera framing,
   * the pointer handler). Anything chosen at *render* time — DPR, shadow-map
   * size, walker count — must read `useMobileTier()` instead: a mutable field
   * re-renders nothing, so those decisions would stay pinned to whatever the
   * viewport was at first mount.
   */
  mobile: boolean
  reduced: boolean
  /** Walkers People is actually animating, published for the perf harness. */
  walkers: number
  /** 'tour' frames the whole table for the /building page; 'billboard' is
   *  the tight, centred crop used by the homepage slice. */
  frame: 'tour' | 'billboard'
  /** The .bld-overlay element the scene's labels portal into. Owned by the
   *  page, read by the scene: the canvas is a separate reconciler, so a plain
   *  ref object carried on the shared state is how the two sides meet. Null
   *  until the page mounts; drei falls back to the canvas container. */
  overlay: RefObject<HTMLDivElement | null>
  /** The tower's root group. Labels raycast against it to decide whether the
   *  building is standing between them and the camera. */
  tower: RefObject<THREE.Group | null>
}

export function createSharedState(): SharedState {
  return {
    progress: 0,
    pointer: { x: 0, y: 0 },
    cur: defaultParams(),
    target: defaultParams(),
    healShift: 0,
    focus: null,
    slots: [],
    mobile: false,
    reduced: false,
    walkers: 0,
    frame: 'tour',
    overlay: { current: null },
    tower: { current: null },
  }
}

export const SharedStateContext = createContext<SharedState | null>(null)

export function useShared(): SharedState {
  const s = useContext(SharedStateContext)
  if (!s) throw new Error('useShared: no SharedStateContext')
  return s
}

/** Claim a figure in the shared crowd meshes for as long as this component lives. */
export function usePersonSlot(color: string): PersonSlot {
  const s = useShared()
  const slot = useMemo(() => createPersonSlot(color), [color])
  useEffect(() => {
    s.slots.push(slot)
    return () => {
      const i = s.slots.indexOf(slot)
      if (i >= 0) s.slots.splice(i, 1)
    }
  }, [s, slot])
  return slot
}
/** Viewport width at or below which the scene drops to its cheaper tier. */
export const MOBILE_MAX = 900

export const isMobileViewport = (): boolean => window.innerWidth < MOBILE_MAX

/**
 * The quality tier as real React state, so a resize across MOBILE_MAX actually
 * re-renders the things that are decided at render time. The canvas is a
 * separate reconciler, so this is re-provided inside it, next to the shared
 * state.
 */
export const MobileTierContext = createContext(false)

export const useMobileTier = (): boolean => useContext(MobileTierContext)
