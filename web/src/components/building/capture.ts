// The hooks the poster-capture harness (scripts/capture-posters.mjs) needs to
// photograph the tour deterministically. Nothing here changes what a visitor
// sees: `__bldCapture` is set by the harness before the app script runs and by
// nobody else.
//
// Two things make an unattended screenshot of this page unreliable, and both
// are answered here rather than in the harness:
//
//   1. The camera never stops moving. Rig applies a continuous
//      `sin(performance.now())` drift, so two captures of the same chapter are
//      never the same image. Capture mode zeroes it.
//   2. There is no frame on which the scene is "done" — the parameters damp
//      asymptotically toward the chapter target. So the scene publishes how far
//      it still has to travel and the harness waits for that to fall under a
//      threshold, instead of sleeping for a guessed duration and hoping.
//
// The second one is why this is worth a module: sleeping is what bakes
// half-drawn linework into a still, and under demand-mode rendering (which
// reduced motion now uses) a sleep can bake in a scene that never drew at all.

/** What the scene publishes for the harness, once per frame in capture mode. */
export interface CaptureProgress {
  /** Largest remaining distance between a damped scene parameter and its target. */
  delta: number
  /** Frames drawn since the page loaded — 0 means nothing has rendered yet. */
  frames: number
  /** Continuous chapter position the scene is actually rendering. */
  progress: number
}

interface CaptureWindow extends Window {
  __bldCapture?: boolean
  __bldCaptureProgress?: CaptureProgress
}

/** Damped parameters this close to their target are close enough to photograph. */
export const SETTLED_DELTA = 0.002

export function captureMode(): boolean {
  return typeof window !== 'undefined' && (window as CaptureWindow).__bldCapture === true
}

export function publishCaptureProgress(p: CaptureProgress): void {
  ;(window as CaptureWindow).__bldCaptureProgress = p
}
