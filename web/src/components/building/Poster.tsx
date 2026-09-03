// The tour as nine stills.
//
// This is what the prerender ships, what every visitor sees before the WebGL
// chunk arrives, and — for a device that fails the frame-rate floor, or has no
// WebGL at all — the whole visual argument. It used to be a single inline SVG
// line drawing that never changed, so a fallback visitor scrolled nine screens
// of copy past one unchanging image: chapters about the building rising, about
// wayfinding labels, about nightfall, all illustrated by the same slab.
//
// Now each chapter has its own still, photographed from the real scene at its
// own camera (scripts/capture-posters.mjs), and scroll swaps them the way it
// swaps scene state. The stills are transparent, so the page's own day/night
// backdrop still shows through and still cross-fades.
import { useEffect, useState } from 'react'
import { CHAPTERS } from './chapters'
import { POSTER_MOBILE_MEDIA, posterSrc } from './posters'

export interface PosterProps {
  hidden: boolean
  /** Chapter to show. The homepage billboard pins its own. */
  chapter?: number
}

/**
 * Which stills are worth having in the DOM: the current chapter, its
 * neighbours (so the next scroll is instant), and everything already visited.
 *
 * Mounting all nine up front would make a visitor who never leaves the first
 * screen — including every visitor on the full tour, who sees the poster for
 * about a second — pay for nine images. Mounting only the active one would
 * flash empty on every chapter change.
 */
function useNearbyStills(chapter: number): Set<number> {
  const [mounted, setMounted] = useState<Set<number>>(() => new Set([0, 1]))
  useEffect(() => {
    setMounted((prev) => {
      const next = new Set(prev)
      for (const i of [chapter - 1, chapter, chapter + 1]) {
        if (i >= 0 && i < CHAPTERS.length) next.add(i)
      }
      return next.size === prev.size ? prev : next
    })
  }, [chapter])
  return mounted
}

export default function Poster({ hidden, chapter = 0 }: PosterProps) {
  const mounted = useNearbyStills(chapter)
  return (
    <div
      className="bld-poster"
      data-component="BuildingPoster"
      data-hidden={hidden ? 'true' : 'false'}
      aria-hidden="true"
    >
      {CHAPTERS.map((ch, i) =>
        mounted.has(i) ? (
          <picture key={ch.id}>
            <source media={POSTER_MOBILE_MEDIA} srcSet={posterSrc(i, ch.id, 'mobile')} type="image/webp" />
            <img
              className="bld-poster__still"
              data-active={i === chapter ? 'true' : 'false'}
              data-chapter={ch.id}
              src={posterSrc(i, ch.id, 'desktop')}
              alt=""
              decoding="async"
              // The first still is the page's largest paint for everyone, not
              // just fallback visitors: it is on screen before the scene is.
              fetchPriority={i === 0 ? 'high' : 'auto'}
            />
          </picture>
        ) : null,
      )}
    </div>
  )
}
