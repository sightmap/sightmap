// The city on the page. Nothing on this module's import path touches
// three.js: the scene is behind React.lazy, so the prerender, a crawler and a
// browser without WebGL get the legend instead, and the WebGL chunk is only
// fetched once a visitor with a working canvas is looking at the stage.
//
// This half owns the three states the research calls for — city, dollhouse,
// listing — and the scene below it only reports what was clicked.
import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { webglAvailable } from '@/components/building/webgl'
import CityPlaceholder from './CityPlaceholder'
import { cityLegend, type CityDocument, type CityListing } from './document'

const CityScene = lazy(() => import('./CityScene'))

export interface CityViewProps {
  city: CityDocument
  listings: CityListing[]
}

export default function CityView({ city, listings }: CityViewProps) {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const legend = useMemo(() => cityLegend(city, listings), [city, listings])
  const names = useMemo(() => new Map(legend.map((e) => [e.slug, e.name])), [legend])

  const [mounted, setMounted] = useState(false)
  const [webgl, setWebgl] = useState(false)
  const [ready, setReady] = useState(false)
  const [reduced, setReduced] = useState(false)
  const [night, setNight] = useState(false)
  const [hover, setHover] = useState<string | null>(null)
  const [focus, setFocus] = useState<string | null>(null)

  // One static file answers every query string for this route, so reading
  // ?focus= on the first render would guarantee a hydration mismatch. The
  // gallery's filters take the same rule: first render ignores the query.
  useEffect(() => {
    setMounted(true)
    setWebgl(webglAvailable())
    const motion = window.matchMedia('(prefers-reduced-motion: reduce)')
    setReduced(motion.matches)
    const wanted = params.get('focus')
    if (wanted && names.has(wanted)) setFocus(wanted)
    // Intentionally a mount-only read: later query changes come from this
    // component itself, and re-running would fight the visitor's clicks.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // The focused building is part of the view, so it belongs in the URL: a
  // visitor who found a building can send someone else straight to it.
  const select = useCallback(
    (slug: string | null) => {
      // The second click on a building already in focus is the third level:
      // the listing itself.
      if (slug && slug === focus) {
        navigate(`/atlas/${slug}`)
        return
      }
      const next = new URLSearchParams(params)
      if (slug) next.set('focus', slug)
      else next.delete('focus')
      setParams(next, { replace: true })
      setFocus(slug)
    },
    [focus, navigate, params, setParams]
  )

  useEffect(() => {
    if (!focus) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') select(null)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [focus, select])

  const focused = focus ? legend.find((e) => e.slug === focus) : undefined
  const hovered = hover ? names.get(hover) : undefined

  return (
    <div className="atlas-city__stage" data-component="CityView" data-ready={ready ? 'true' : 'false'}>
      <CityPlaceholder entries={legend} hidden={ready} />

      {mounted && webgl && (
        <Suspense fallback={null}>
          <CityScene
            city={city}
            listings={listings}
            night={night}
            reduced={reduced}
            focus={focus}
            onReady={() => setReady(true)}
            onHover={setHover}
            onSelect={select}
          />
        </Suspense>
      )}

      {mounted && webgl && (
        <div className="atlas-city__hud">
          <button
            type="button"
            className="atlas-city__button"
            aria-pressed={night}
            onClick={() => setNight((n) => !n)}
          >
            {night ? 'Day' : 'Night'}
          </button>
          {focused && (
            <>
              <button type="button" className="atlas-city__button" onClick={() => select(null)}>
                Back to the city
              </button>
              <a className="atlas-city__button atlas-city__button--go" href={`/atlas/${focused.slug}`}>
                Open listing
              </a>
            </>
          )}
        </div>
      )}

      {mounted && webgl && (focused || hovered) && (
        <p className="atlas-city__caption" aria-live="polite">
          {focused ? (
            <>
              <strong>{focused.name}</strong> · {focused.district} · lot {focused.lot}
            </>
          ) : (
            hovered
          )}
        </p>
      )}
    </div>
  )
}
