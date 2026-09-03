// Deterministic, seamlessly tiling noise for the building's texture bake, and
// the two derivations the bake needs from a height field: a tangent-space
// normal, and ambient occlusion.
//
// Everything here is a pure function of its arguments — same seed, same bytes,
// every run — because the .ktx2 files are committed and a bake that drifted
// would show up as a diff nobody asked for.

/** Integer hash → [0, 1). Cheap, well-mixed enough for surface grain. */
function hash(ix: number, iy: number, seed: number): number {
  let h = Math.imul(ix, 374761393) ^ Math.imul(iy, 668265263) ^ Math.imul(seed, 2147483647)
  h = Math.imul(h ^ (h >>> 13), 1274126177)
  return ((h ^ (h >>> 16)) >>> 0) / 4294967296
}

const fade = (t: number): number => t * t * t * (t * (t * 6 - 15) + 10)

/**
 * Value noise on a lattice of `periodX` × `periodY` cells across the unit square.
 *
 * Periodic by construction: lattice coordinates are taken modulo the period,
 * so u=0 and u=1 sample the same corner and the texture tiles without a seam.
 * That matters more here than noise quality — these atlases are sampled with
 * `fract()` inside their tile, so a seam would draw a visible grid.
 *
 * The two periods are separate because most of these surfaces are anisotropic:
 * brushed steel streaks along one axis and timber runs along the grain, and a
 * square lattice cannot express either.
 */
export function valueNoise(u: number, v: number, periodX: number, periodY: number, seed: number): number {
  const x = u * periodX
  const y = v * periodY
  const x0 = Math.floor(x)
  const y0 = Math.floor(y)
  const fx = fade(x - x0)
  const fy = fade(y - y0)
  const wrapX = (n: number): number => ((n % periodX) + periodX) % periodX
  const wrapY = (n: number): number => ((n % periodY) + periodY) % periodY
  const a = hash(wrapX(x0), wrapY(y0), seed)
  const b = hash(wrapX(x0 + 1), wrapY(y0), seed)
  const c = hash(wrapX(x0), wrapY(y0 + 1), seed)
  const d = hash(wrapX(x0 + 1), wrapY(y0 + 1), seed)
  return (a + (b - a) * fx) * (1 - fy) + (c + (d - c) * fx) * fy
}

/**
 * Sum of `octaves` doubling-frequency value-noise layers, normalised to [0, 1].
 *
 * Periods double per octave, so every octave stays periodic on the unit square
 * and the sum tiles too.
 */
export function fbm(
  u: number,
  v: number,
  periodX: number,
  periodY: number,
  octaves: number,
  seed: number
): number {
  let sum = 0
  let amp = 1
  let norm = 0
  for (let o = 0; o < octaves; o++) {
    sum += valueNoise(u, v, periodX * 2 ** o, periodY * 2 ** o, seed + o * 101) * amp
    norm += amp
    amp *= 0.5
  }
  return sum / norm
}

/**
 * Tangent-space normal at (x, y) of a height field, by central difference.
 *
 * `height` is sampled in texel units and must wrap the way the caller's image
 * does, or the normal seams where the height field does not. Returns each
 * component already encoded to [0, 255].
 *
 * `strength` scales the gradient: these are surface grain, not displacement,
 * and an over-strong normal on a flat architectural slab reads as crumpled
 * foil the moment the environment map hits it.
 */
export function heightToNormal(
  height: (x: number, y: number) => number,
  x: number,
  y: number,
  strength: number
): [number, number, number] {
  const dx = (height(x + 1, y) - height(x - 1, y)) * strength
  const dy = (height(x, y + 1) - height(x, y - 1)) * strength
  // Normal of the surface z = h(x, y) is (-dh/dx, -dh/dy, 1), normalised.
  const len = Math.hypot(dx, dy, 1)
  return [
    Math.round(((-dx / len) * 0.5 + 0.5) * 255),
    Math.round(((-dy / len) * 0.5 + 0.5) * 255),
    Math.round((1 / len) * 0.5 * 255 + 127.5),
  ]
}

export interface AoOptions {
  /** Ring radii to sample, in texels. Small radii read as contact, large as room shade. */
  radii: readonly number[]
  /** Samples per ring. */
  spokes: number
  /** How much a fully-occluded texel darkens: 0 keeps white, 1 reaches black. */
  strength: number
  /**
   * Height difference, in world units, at which an occluder counts as fully
   * blocking. Anything taller is clamped, so a bookshelf and a tower do not
   * darken the floor differently.
   */
  saturation: number
}

/**
 * Ambient occlusion of a height field, sampled on rings.
 *
 * For every ring sample the occluder's elevation above the receiving texel is
 * turned into an occlusion angle — `rise / distance`, the tangent of the angle
 * the occluder subtends — and the mean over all samples is the fraction of the
 * hemisphere that is blocked. Near samples therefore dominate, which is
 * exactly the cue this is for: the crease where a desk leg meets the floor,
 * not a general dimming of the room.
 *
 * `height` is in world units above the receiving plane and is read in texel
 * coordinates; out-of-range reads are the caller's problem to clamp.
 */
export function heightFieldAo(
  height: (x: number, y: number) => number,
  x: number,
  y: number,
  texelsPerUnit: number,
  o: AoOptions
): number {
  const base = height(x, y)
  let blocked = 0
  let samples = 0
  for (const r of o.radii) {
    for (let s = 0; s < o.spokes; s++) {
      // Offset every ring so the spokes do not line up into visible spokes.
      const a = ((s + 0.5) / o.spokes) * Math.PI * 2 + r * 0.7
      const rise = height(x + Math.cos(a) * r, y + Math.sin(a) * r) - base
      if (rise > 0) {
        const distance = r / texelsPerUnit
        // tan of the subtended angle, clamped so tall occluders do not
        // over-darken, then mapped to [0, 1] against a 45° reference.
        blocked += Math.min(1, Math.min(rise, o.saturation) / distance)
      }
      samples++
    }
  }
  return 1 - (blocked / samples) * o.strength
}
