// The body-part rig every person in the building shares: six parts — torso,
// head, two arms, two legs — sized for a figure ~1 world unit tall and posed
// in the person's own local frame (origin between the feet, +Y up, +Z the
// direction they face, which is what `rotation.y = atan2(dx, dz)` produces).
//
// Pure numbers, no three.js: the pose is an array of six {x, y, z, rx} entries
// that People.tsx turns into instance matrices. Keeping it here means the
// maths is unit-testable and one rig drives walkers, seated figures and the
// scripted demo figures alike, instead of a mesh tree per population.

export type PartKey = 'torso' | 'head' | 'armL' | 'armR' | 'legL' | 'legR'

/** Instance-mesh order. An index into a pose array is an index into this list. */
export const PART_KEYS: readonly PartKey[] = ['torso', 'head', 'armL', 'armR', 'legL', 'legR']

/**
 * Limb capsules hang from a pivot (shoulder, hip) rather than sitting at their
 * own centre, so a swing is a rotation about that pivot and the pose maths
 * only has to place the resulting centre.
 */
export const RIG = {
  torsoR: 0.11,
  torsoLen: 0.2,
  torsoY: 0.62,
  /** Front-to-back squash, so a torso reads wider than it is deep. */
  torsoFlat: 0.72,
  headR: 0.1,
  headY: 0.93,
  shoulderY: 0.78,
  armR: 0.042,
  armLen: 0.26,
  armX: 0.14,
  hipY: 0.42,
  legR: 0.052,
  legLen: 0.3,
  legX: 0.062,
} as const

/** Tip-to-tip length of an arm / leg capsule (cylinder plus both caps). */
export const ARM_LEN = RIG.armLen + RIG.armR * 2
export const LEG_LEN = RIG.legLen + RIG.legR * 2

/** Height of a figure at scale 1, for callers sizing one against the model. */
export const PERSON_HEIGHT = RIG.headY + RIG.headR

export interface PartPose {
  x: number
  y: number
  z: number
  /** Rotation about the person's local X axis; positive swings the part back. */
  rx: number
}

/** A scratch pose array, one entry per part in `PART_KEYS` order. */
export function makePose(): PartPose[] {
  return PART_KEYS.map(() => ({ x: 0, y: 0, z: 0, rx: 0 }))
}

/**
 * Centre of a limb whose pivot is at (`px`, `py`) once it has swung `rx` about
 * that pivot. A limb hangs down, so its centre starts half its length below
 * the pivot and rotates around it.
 */
function hang(out: PartPose, px: number, py: number, len: number, rx: number): void {
  const half = len / 2
  out.x = px
  out.y = py - Math.cos(rx) * half
  out.z = -Math.sin(rx) * half
  out.rx = rx
}

/** Peak swing of each limb pair in radians, and the hip dip at mid-stride. */
export const SWING = { arm: 0.5, leg: 0.62 } as const
export const BOB = 0.024
/** Forward lean of the torso at full walking speed. */
export const LEAN = 0.07

/**
 * Ground covered by one full two-step cycle, in world units. Phase advances
 * with distance travelled rather than with wall time, so a figure easing out
 * of a stop takes slower steps instead of moon-walking at a fixed cadence.
 */
export const STRIDE = 1.15

/** Speed, in world units per second, at which the gait reaches full swing. */
export const FULL_SWING_SPEED = 1.4

const TAU = Math.PI * 2

/** Advance gait phase by `travelled` world units, wrapped to [0, TAU). */
export function advancePhase(phase: number, travelled: number, stride: number = STRIDE): number {
  const next = phase + (travelled / stride) * TAU
  return ((next % TAU) + TAU) % TAU
}

/** How much of the swing a figure moving at `speed` should be showing. */
export function gaitAmplitude(speed: number, reduced: boolean): number {
  if (reduced) return 0
  return Math.min(1, Math.max(0, speed / FULL_SWING_SPEED))
}

/**
 * The walk cycle: the legs swing in antiphase, each arm opposes the leg on
 * its own side, and the hips dip when the legs are furthest apart. `amp`
 * scales all of it, so amp 0 is a figure standing neutrally rather than one
 * frozen mid-stride — which is what reduced motion asks for.
 *
 * At this camera you never see a face, so the swing is the whole read; the
 * amplitudes are tuned for legibility at 6–10px, not for anatomy.
 */
export function gaitPose(phase: number, amp: number, out: PartPose[]): PartPose[] {
  const swing = Math.sin(phase)
  const dip = -Math.abs(swing) * BOB * amp
  const [torso, head, armL, armR, legL, legR] = out
  torso.x = 0
  torso.y = RIG.torsoY + dip
  torso.z = 0
  torso.rx = LEAN * amp
  head.x = 0
  head.y = RIG.headY + dip
  head.z = 0
  head.rx = 0
  hang(armL, -RIG.armX, RIG.shoulderY + dip, ARM_LEN, -swing * SWING.arm * amp)
  hang(armR, RIG.armX, RIG.shoulderY + dip, ARM_LEN, swing * SWING.arm * amp)
  hang(legL, -RIG.legX, RIG.hipY + dip, LEG_LEN, swing * SWING.leg * amp)
  hang(legR, RIG.legX, RIG.hipY + dip, LEG_LEN, -swing * SWING.leg * amp)
  return out
}

/** The neutral standing pose: upright, limbs hanging. */
export function standPose(out: PartPose[]): PartPose[] {
  return gaitPose(0, 0, out)
}

/** Per-person gait state, advanced from where that person was last frame. */
export interface Gait {
  phase: number
  amp: number
  /** Last horizontal position, so travel is measured rather than assumed. */
  px: number
  pz: number
  seeded: boolean
}

export function makeGait(phase = 0): Gait {
  return { phase, amp: 0, px: 0, pz: 0, seeded: false }
}

/** A jump larger than this is a teleport (a journey restarting), not a stride. */
const TELEPORT = 0.5

/** How fast the swing amplitude chases the speed it should be showing. */
const AMP_RESPONSE = 10

/**
 * Advance one figure's gait to its new position. Horizontal travel only: a
 * walker riding the core between floors is moving, but it is not stepping.
 */
export function stepGait(g: Gait, x: number, z: number, dt: number, reduced: boolean): Gait {
  let travelled = 0
  if (g.seeded) {
    const dist = Math.hypot(x - g.px, z - g.pz)
    if (dist < TELEPORT) travelled = dist
  }
  g.px = x
  g.pz = z
  g.seeded = true
  if (reduced) {
    g.amp = 0
    return g
  }
  const target = gaitAmplitude(travelled / Math.max(dt, 1e-4), false)
  g.amp += (target - g.amp) * Math.min(1, dt * AMP_RESPONSE)
  g.phase = advancePhase(g.phase, travelled)
  return g
}
