// Shared constants between NightfallBloom.tsx (the composer) and the
// emissive geometry it selectively blooms (Tower.tsx's curtain-wall glass).
//
// This is a Three.js layer, not a luminance threshold, because a threshold
// can't do the job here: at night the desk monitors and screens (Tower.tsx's
// furniture materials) read *brighter* than the window glass does — a
// threshold loose enough to catch the windows also catches every monitor and
// screen in the building, and one tight enough to exclude those excludes the
// windows too. Layer membership sidesteps the ordering entirely: bloom only
// ever sees what is tagged onto BLOOM_LAYER, independent of how bright
// anything is.
export const BLOOM_LAYER = 1

/**
 * Below this damped night value, NightfallBloom keeps the plain render path:
 * no composer, no extra passes, and the canvas's own `antialias: true` MSAA
 * still applies. Matches roughly where the glass emissive ramp
 * (`mats.glass.emissiveIntensity = n * 0.9` in Tower.tsx) starts reading as
 * visibly lit, so the takeover happens exactly when there is something to
 * bloom — not for the other eight-ninths of the tour.
 */
export const BLOOM_NIGHT_THRESHOLD = 0.05
