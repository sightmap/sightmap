// Where the per-chapter poster stills live, and what they are called.
//
// The stills are photographs of the real scene, captured by
// `web/scripts/capture-posters.mjs` and committed under `public/building/`.
// The naming is derived, never listed: this module and the capture harness
// agree on one pattern, so a chapter added to chapters.ts needs no edit here —
// it needs a capture run, and until then its missing file is loudly missing
// rather than quietly showing the wrong chapter's artwork.
import { MOBILE_MAX } from './state'

export type PosterVariant = 'desktop' | 'mobile'

/**
 * Capture viewports. Desktop matches the widest common laptop framing; mobile
 * matches a modern phone in portrait — which is the tier's actual audience,
 * since the devices that fail the frame-rate floor are overwhelmingly phones.
 * A single desktop still letterboxed into a portrait viewport would put the
 * tower in a band across the middle of the screen with the story card on top
 * of it, which is the composition the tour deliberately avoids.
 */
export const POSTER_VIEWPORTS: Record<PosterVariant, { width: number; height: number }> = {
  desktop: { width: 1440, height: 900 },
  mobile: { width: 430, height: 860 },
}

/** The breakpoint the `<picture>` switches on — the same one the scene's own
 *  mobile tier uses, so the still matches the framing the tour would have. */
export const POSTER_MOBILE_MEDIA = `(max-width: ${MOBILE_MAX - 1}px)`

export function posterSrc(index: number, id: string, variant: PosterVariant): string {
  return `/building/poster-${variant}-${String(index).padStart(2, '0')}-${id}.webp`
}
