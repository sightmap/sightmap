// Mutable, frame-rate state shared between the page (which owns scroll and
// pointer) and the scene (which reads them every frame). Deliberately not
// React state: scroll fires far more often than React should re-render, and
// the scene only needs the latest value when it draws.
import { createContext, useContext, type RefObject } from 'react'
import type * as THREE from 'three'
import { defaultParams, type SceneParams } from './chapters'

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
  mobile: boolean
  reduced: boolean
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
    mobile: false,
    reduced: false,
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
