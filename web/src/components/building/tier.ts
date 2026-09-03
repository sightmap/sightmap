// Which tier the tour runs at, decided by measuring this device rather than
// by recognising it.
//
// The alternative — a user-agent allowlist of "iPhone 15 and up" — is wrong
// within a year and fails silently and permanently for every phone released
// after we ship: a device we have never heard of gets the answer we guessed
// for it in 2026 and has no way to argue. Measurement gets newer hardware
// right for free, and gets *this* device right rather than its model number.
//
// The cost of measuring is that a fast device having a bad first second —
// thermal throttling, a busy background tab, a cold shader compile — can be
// demoted for a reason that has nothing to do with what it can do. Two
// defences, both here: hysteresis, so a single bad window is never enough, and
// (in BuildingExperience) a visible override that puts the tour back and stops
// the sampler from second-guessing the visitor.
//
// Pure and clock-free — frame deltas are pushed in, so tier.test.ts can drive
// it with synthetic timings instead of asking a CI runner to be slow on cue.

/** Length of one sample window. Roughly the first second of the tour. */
export const PROBE_MS = 1000
/** Below this the tour is not worth showing; the poster tells the story better. */
export const FLOOR_FPS = 24
/** Consecutive bad windows required before demoting. One is a blip. */
export const BAD_WINDOWS = 2
/**
 * Stop sampling after this many windows without a demotion. The question is
 * "can this device open the tour", not "is it still fast" — a permanently
 * armed sampler would demote someone twenty minutes in because they dragged
 * another window over the tab.
 */
export const MAX_WINDOWS = 4
/**
 * Frames shorter than this are almost certainly a coalesced or duplicated
 * callback rather than a real frame; frames longer are a tab that was
 * backgrounded or a long task elsewhere on the page. Neither says anything
 * about rendering capability, so neither is sampled.
 */
export const MIN_FRAME_MS = 1
export const MAX_FRAME_MS = 2000

export type RenderTier = 'full' | 'poster'

export interface TierWindow {
  /** Median frame rate over the window — median, so one long frame cannot
   *  drag an otherwise healthy window under the floor. */
  fps: number
  /** Frames the window was measured from. */
  frames: number
  /** Consecutive windows under FLOOR_FPS, including this one. */
  consecutiveBad: number
  /** True on the window that trips hysteresis, and only that one. */
  demote: boolean
  /** True once the sampler has stopped, either way. */
  done: boolean
}

export interface TierSamplerOptions {
  probeMs?: number
  floorFps?: number
  badWindows?: number
  maxWindows?: number
}

export interface TierSampler {
  /** Feed one frame delta in milliseconds. Returns a result only on the frame
   *  that closes a window, and null on every other frame. */
  frame(dtMs: number): TierWindow | null
  /** No further windows will be reported. */
  readonly done: boolean
}

function median(values: number[]): number {
  const sorted = [...values].sort((a, b) => a - b)
  const mid = sorted.length >> 1
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
}

export function createTierSampler(options: TierSamplerOptions = {}): TierSampler {
  const probeMs = options.probeMs ?? PROBE_MS
  const floorFps = options.floorFps ?? FLOOR_FPS
  const badWindows = options.badWindows ?? BAD_WINDOWS
  const maxWindows = options.maxWindows ?? MAX_WINDOWS

  let elapsed = 0
  let samples: number[] = []
  let windows = 0
  let consecutiveBad = 0
  let done = false

  return {
    get done() {
      return done
    },
    frame(dtMs: number): TierWindow | null {
      if (done) return null
      // Window length counts every frame's wall time, including the ones
      // excluded from the median: a window made of nothing but 3-second stalls
      // should still close, and should still be reported as bad.
      elapsed += Math.min(Math.max(dtMs, 0), MAX_FRAME_MS)
      if (dtMs >= MIN_FRAME_MS && dtMs <= MAX_FRAME_MS) samples.push(dtMs)
      if (elapsed < probeMs) return null

      windows++
      // A window with no usable frames at all is a device that rendered
      // nothing in a second. That is the worst case, not an unmeasurable one.
      const fps = samples.length ? 1000 / median(samples) : 0
      const bad = fps < floorFps
      consecutiveBad = bad ? consecutiveBad + 1 : 0
      const demote = consecutiveBad >= badWindows
      done = demote || windows >= maxWindows
      const result: TierWindow = { fps, frames: samples.length, consecutiveBad, demote, done }
      elapsed = 0
      samples = []
      return result
    },
  }
}
