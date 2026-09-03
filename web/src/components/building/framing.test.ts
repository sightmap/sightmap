import { describe, expect, it } from 'vitest'
import { computeFraming, makeFraming, type FramingInputs } from './framing'

const DESKTOP: FramingInputs = {
  az: 42,
  el: 27,
  chapterZoom: 1,
  lookY: 6.2,
  mobile: false,
  frame: 'tour',
  width: 1440,
  height: 900,
}

describe('computeFraming', () => {
  it('produces an orthonormal camera basis', () => {
    const out = computeFraming(DESKTOP, makeFraming())
    expect(out.dir.length()).toBeCloseTo(1, 5)
    expect(out.right.length()).toBeCloseTo(1, 5)
    expect(out.up.length()).toBeCloseTo(1, 5)
    expect(out.dir.dot(out.right)).toBeCloseTo(0, 5)
    expect(out.dir.dot(out.up)).toBeCloseTo(0, 5)
    expect(out.right.dot(out.up)).toBeCloseTo(0, 5)
  })

  it('zooms in as the chapter zoom multiplier increases', () => {
    const wide = computeFraming(DESKTOP, makeFraming())
    const tight = computeFraming({ ...DESKTOP, chapterZoom: 1.2 }, makeFraming())
    expect(tight.zoom).toBeGreaterThan(wide.zoom)
  })

  it('shifts the target differently for the mobile card layout than desktop', () => {
    const desktop = computeFraming(DESKTOP, makeFraming())
    const mobile = computeFraming({ ...DESKTOP, mobile: true }, makeFraming())
    expect(mobile.target.x).not.toBeCloseTo(desktop.target.x, 2)
  })

  it('crops tighter for the billboard than the tour at the same viewport', () => {
    const tour = computeFraming(DESKTOP, makeFraming())
    const billboard = computeFraming({ ...DESKTOP, frame: 'billboard' }, makeFraming())
    expect(billboard.zoom).toBeGreaterThan(tour.zoom)
  })

  it('mutates and returns the same scratch object, so a caller can reuse it every frame', () => {
    const scratch = makeFraming()
    const out = computeFraming(DESKTOP, scratch)
    expect(out).toBe(scratch)
  })
})
