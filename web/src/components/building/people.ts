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

/** The neutral standing pose: upright, limbs hanging. */
export function standPose(out: PartPose[]): PartPose[] {
  const [torso, head, armL, armR, legL, legR] = out
  torso.x = 0
  torso.y = RIG.torsoY
  torso.z = 0
  torso.rx = 0
  head.x = 0
  head.y = RIG.headY
  head.z = 0
  head.rx = 0
  hang(armL, -RIG.armX, RIG.shoulderY, ARM_LEN, 0)
  hang(armR, RIG.armX, RIG.shoulderY, ARM_LEN, 0)
  hang(legL, -RIG.legX, RIG.hipY, LEG_LEN, 0)
  hang(legR, RIG.legX, RIG.hipY, LEG_LEN, 0)
  return out
}
