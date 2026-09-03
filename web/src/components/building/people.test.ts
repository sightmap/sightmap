import { describe, expect, it } from 'vitest'
import {
  ARM_LEN,
  LEG_LEN,
  PART_KEYS,
  RIG,
  STRIDE,
  advancePhase,
  gaitAmplitude,
  gaitPose,
  makeGait,
  makePose,
  standPose,
  stepGait,
} from './people'

const TAU = Math.PI * 2
const [TORSO, HEAD, ARM_L, ARM_R, LEG_L, LEG_R] = PART_KEYS.map((_, i) => i)

describe('advancePhase', () => {
  it('completes one cycle per stride of travel', () => {
    expect(advancePhase(0, STRIDE)).toBeCloseTo(0, 6)
    expect(advancePhase(0, STRIDE / 2)).toBeCloseTo(Math.PI, 6)
  })

  it('stays inside [0, TAU) however far the figure walks', () => {
    let phase = 0
    for (let i = 0; i < 100; i++) phase = advancePhase(phase, 0.37)
    expect(phase).toBeGreaterThanOrEqual(0)
    expect(phase).toBeLessThan(TAU)
  })
})

describe('gaitAmplitude', () => {
  it('scales with speed and saturates at full swing', () => {
    expect(gaitAmplitude(0, false)).toBe(0)
    expect(gaitAmplitude(0.7, false)).toBeGreaterThan(0)
    expect(gaitAmplitude(99, false)).toBe(1)
  })

  it('is zero under reduced motion however fast the figure moves', () => {
    expect(gaitAmplitude(99, true)).toBe(0)
  })
})

describe('gaitPose', () => {
  it('is a neutral standing figure at zero amplitude, whatever the phase', () => {
    // Reduced motion holds amp at 0 mid-journey, so this is what stops a
    // walker freezing mid-stride.
    for (const phase of [0, 1.1, Math.PI, 5.5]) {
      const pose = gaitPose(phase, 0, makePose())
      // Signed zero is fine here; what matters is that nothing is rotated.
      for (const part of pose) expect(part.rx).toBeCloseTo(0, 12)
      expect(pose[TORSO].y).toBeCloseTo(RIG.torsoY, 6)
      expect(pose[HEAD].y).toBeCloseTo(RIG.headY, 6)
      expect(pose[LEG_L].y).toBeCloseTo(RIG.hipY - LEG_LEN / 2, 6)
      expect(pose[ARM_R].y).toBeCloseTo(RIG.shoulderY - ARM_LEN / 2, 6)
    }
  })

  it('matches the standing pose', () => {
    expect(gaitPose(2.4, 0, makePose())).toEqual(standPose(makePose()))
  })

  it('swings the legs in antiphase with the arms opposing them', () => {
    const pose = gaitPose(Math.PI / 2, 1, makePose())
    expect(pose[LEG_L].rx).toBeCloseTo(-pose[LEG_R].rx, 6)
    expect(pose[ARM_L].rx).toBeCloseTo(-pose[ARM_R].rx, 6)
    // Each arm opposes the leg on its own side.
    expect(Math.sign(pose[ARM_L].rx)).toBe(-Math.sign(pose[LEG_L].rx))
    expect(Math.abs(pose[LEG_L].rx)).toBeGreaterThan(0.3)
  })

  it('dips the hips when the legs are furthest apart', () => {
    const spread = gaitPose(Math.PI / 2, 1, makePose())
    const together = gaitPose(0, 1, makePose())
    expect(spread[TORSO].y).toBeLessThan(together[TORSO].y)
    // The head rides the same dip, so the figure does not stretch.
    expect(together[HEAD].y - spread[HEAD].y).toBeCloseTo(together[TORSO].y - spread[TORSO].y, 6)
  })

  it('swings a limb about its pivot rather than sliding it', () => {
    const pose = gaitPose(Math.PI / 2, 1, makePose())
    const leg = pose[LEG_L]
    // Distance from hip to limb centre is half the limb, at any swing angle.
    const dy = RIG.hipY + (pose[TORSO].y - RIG.torsoY) - leg.y
    expect(Math.hypot(dy, leg.z)).toBeCloseTo(LEG_LEN / 2, 6)
  })
})

describe('stepGait', () => {
  it('measures travel rather than assuming it: no movement, no swing', () => {
    const g = makeGait()
    stepGait(g, 3, 3, 1 / 60, false)
    for (let i = 0; i < 60; i++) stepGait(g, 3, 3, 1 / 60, false)
    expect(g.amp).toBeCloseTo(0, 3)
    expect(g.phase).toBe(0)
  })

  it('advances phase with distance and works the amplitude up to full swing', () => {
    const g = makeGait()
    const dt = 1 / 60
    const step = 1.75 * dt
    let x = 0
    stepGait(g, x, 0, dt, false)
    for (let i = 0; i < 60; i++) {
      x += step
      stepGait(g, x, 0, dt, false)
    }
    expect(g.amp).toBeGreaterThan(0.9)
    // One second at 1.75 units/s is 1.75 / STRIDE cycles.
    const cycles = (60 * step) / STRIDE
    expect(g.phase).toBeCloseTo((cycles % 1) * TAU, 3)
  })

  it('ignores a teleport, so a restarting journey does not spin the legs', () => {
    const g = makeGait()
    stepGait(g, 0, 0, 1 / 60, false)
    stepGait(g, 40, 12, 1 / 60, false)
    expect(g.phase).toBe(0)
    expect(g.amp).toBe(0)
  })

  it('holds phase and drops the swing under reduced motion', () => {
    const g = makeGait(1.2)
    const dt = 1 / 60
    stepGait(g, 0, 0, dt, false)
    stepGait(g, 0.4, 0, dt, false)
    const moving = g.phase
    expect(g.amp).toBeGreaterThan(0)
    stepGait(g, 0.8, 0, dt, true)
    expect(g.amp).toBe(0)
    expect(g.phase).toBe(moving)
  })

  it('tracks travel, not wall time: half the speed is half the step rate', () => {
    const fast = makeGait()
    const slow = makeGait()
    const dt = 1 / 60
    stepGait(fast, 0, 0, dt, false)
    stepGait(slow, 0, 0, dt, false)
    for (let i = 1; i <= 30; i++) {
      stepGait(fast, i * 0.02, 0, dt, false)
      stepGait(slow, i * 0.01, 0, dt, false)
    }
    expect(fast.phase).toBeCloseTo(slow.phase * 2, 6)
  })
})
