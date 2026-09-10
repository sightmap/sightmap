import { describe, expect, it } from 'vitest'
import { advanceBillboardTime, billboardStep } from './billboard'
import { defaultParams, type SceneParams } from './chapters'

const scratch = (): SceneParams => defaultParams()

describe('advanceBillboardTime', () => {
  it('preserves the cycle position across a frameloop pause/resume instead of snapping to t=0', () => {
    // The bug: @react-three/fiber zeros its THREE.Clock on every 'never'→'always'
    // transition (the toggle the IntersectionObserver fires on each scroll
    // in/out), so a clock.getElapsedTime()-driven cycle snaps to t=0 the moment
    // the billboard re-enters the viewport. Accumulating delta in a ref is
    // independent of that clock — it freezes while off-screen (no frames run)
    // and resumes in place — so a scroll-back near mid-cycle stays there.
    let t = 30
    t = advanceBillboardTime(t, 0.016, false) // first frame after re-entry
    expect(t).toBeCloseTo(30.016, 6) // NOT ~0.016 as clock.getElapsedTime() would give
    expect(billboardStep(t, scratch())).toBe(true) // still night — matches the CSS sky
    // The same frame with the buggy clock (t≈0) relights to day in one frame,
    // leaving a day-lit 3D building against the still-night CSS sky.
    expect(billboardStep(advanceBillboardTime(0, 0.016, false), scratch())).toBe(false)
  })

  it('clamps huge frame deltas so a tab refocus cannot jump the cycle', () => {
    expect(advanceBillboardTime(0, 5, false)).toBe(0.1)
    expect(advanceBillboardTime(10, 20, false)).toBe(10.1)
    expect(advanceBillboardTime(5, 0.05, false)).toBe(5.05)
  })

  it('does not advance the cycle when prefers-reduced-motion is set', () => {
    expect(advanceBillboardTime(0, 0.1, true)).toBe(0)
    let t = 0
    for (let i = 0; i < 5000; i++) t = advanceBillboardTime(t, 0.016, true)
    expect(t).toBe(0)
    expect(billboardStep(t, scratch())).toBe(false)
  })
})
