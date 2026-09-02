// Mutable, frame-rate state shared between the page (which owns scroll and
// pointer) and the scene (which reads them every frame). Deliberately not
// React state: scroll fires far more often than React should re-render, and
// the scene only needs the latest value when it draws.
import { createContext, useContext } from 'react'
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
  }
}

export const SharedStateContext = createContext<SharedState | null>(null)

export function useShared(): SharedState {
  const s = useContext(SharedStateContext)
  if (!s) throw new Error('useShared: no SharedStateContext')
  return s
}
