import { describe, expect, it } from 'vitest'
import {
  BAD_WINDOWS,
  FLOOR_FPS,
  MAX_WINDOWS,
  PROBE_MS,
  createTierSampler,
  type TierWindow,
} from './tier'

/**
 * Frame times are injected, never measured. The sandbox this was written on has
 * no GPU (WebGL runs on SwiftShader), and a CI runner cannot be asked to drop
 * to 18fps on cue — so the thing under test is the decision, and the numbers
 * that reach it are synthetic on purpose.
 */
const feed = (
  sampler: ReturnType<typeof createTierSampler>,
  dtMs: number,
  windows = 1,
): TierWindow[] => {
  const out: TierWindow[] = []
  // Frame-count arithmetic would be off by one whenever the accumulated float
  // lands a hair under the probe interval, so drive it by outcome: feed frames
  // until the windows arrive or the sampler stops reporting.
  const limit = Math.ceil((windows * PROBE_MS) / dtMs) + windows + 2
  for (let i = 0; i < limit && out.length < windows; i++) {
    if (sampler.done) break
    const w = sampler.frame(dtMs)
    if (w) out.push(w)
  }
  return out
}

/** 60fps and 15fps in frame-time terms. */
const FAST = 1000 / 60
const SLOW = 1000 / 15

describe('createTierSampler', () => {
  it('reports a window roughly every probe interval', () => {
    const s = createTierSampler()
    const windows = feed(s, FAST, 2)
    expect(windows).toHaveLength(2)
    expect(windows[0].fps).toBeCloseTo(60, 0)
  })

  it('does not demote a fast device', () => {
    const s = createTierSampler()
    const windows = feed(s, FAST, MAX_WINDOWS)
    expect(windows.some((w) => w.demote)).toBe(false)
  })

  it('does not demote on one bad window — that is a blip', () => {
    const s = createTierSampler()
    const [bad] = feed(s, SLOW)
    expect(bad.fps).toBeLessThan(FLOOR_FPS)
    expect(bad.consecutiveBad).toBe(1)
    expect(bad.demote).toBe(false)
  })

  it('demotes on the second consecutive bad window, and not before', () => {
    const s = createTierSampler()
    const [first] = feed(s, SLOW)
    const [second] = feed(s, SLOW)
    expect(first.demote).toBe(false)
    expect(second.consecutiveBad).toBe(BAD_WINDOWS)
    expect(second.demote).toBe(true)
    expect(second.done).toBe(true)
  })

  it('forgets a bad window once a good one follows it', () => {
    const s = createTierSampler()
    feed(s, SLOW) // bad
    const [good] = feed(s, FAST) // recovery: cold shader compile, then fine
    const [badAgain] = feed(s, SLOW)
    expect(good.consecutiveBad).toBe(0)
    expect(badAgain.consecutiveBad).toBe(1)
    expect(badAgain.demote).toBe(false)
  })

  it('treats a cold first window followed by a fast one as a healthy device', () => {
    // The exact case the hysteresis exists for: one 12fps window of shader
    // compilation, then a flagship running at 60.
    const s = createTierSampler()
    const [cold] = feed(s, 1000 / 12)
    const rest = feed(s, FAST, 3)
    expect(cold.demote).toBe(false)
    expect(rest.some((w) => w.demote)).toBe(false)
  })

  it('stops sampling after the decision so a later stall cannot demote', () => {
    const s = createTierSampler()
    expect(feed(s, FAST, MAX_WINDOWS)).toHaveLength(MAX_WINDOWS)
    expect(s.done).toBe(true)
    expect(feed(s, SLOW, 4)).toEqual([])
  })

  it('reports zero fps when a window drew nothing usable', () => {
    // Every frame is a multi-second stall: excluded from the median, but the
    // window still closes and still counts as bad.
    const s = createTierSampler()
    const first = s.frame(3000)
    expect(first).not.toBeNull()
    expect(first?.fps).toBe(0)
    expect(first?.frames).toBe(0)
    expect(first?.demote).toBe(false)
    expect(s.frame(3000)?.demote).toBe(true)
  })

  it('is not dragged under the floor by a single long frame', () => {
    // One 500ms hitch in an otherwise 60fps second: the mean would read ~28fps,
    // the median still reads 60.
    const s = createTierSampler()
    let closed: TierWindow | null = null
    closed = s.frame(500)
    for (let t = 0; t < PROBE_MS && !closed; t += FAST) closed = s.frame(FAST)
    expect(closed?.fps).toBeCloseTo(60, 0)
    expect(closed?.demote).toBe(false)
  })

  it('honours injected thresholds', () => {
    const s = createTierSampler({ probeMs: 200, floorFps: 50, badWindows: 3 })
    const windows = feed(s, 1000 / 30, 3)
    expect(windows).toHaveLength(3)
    expect(windows.map((w) => w.demote)).toEqual([false, false, true])
  })
})
