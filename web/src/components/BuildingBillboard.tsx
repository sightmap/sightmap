import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react'
import Poster from './building/Poster'
import { createSharedState } from './building/state'
import { webglAvailable } from './building/webgl'
import { JOURNEYS } from './building/model'

// Homepage billboard for the /building tour: headline, subcopy, CTA, and a
// live slice of the model. The WebGL chunk loads only once the frame has
// scrolled near the viewport and the frame loop pauses while it is off-screen,
// so the homepage's own bundle and idle cost are unchanged. The SVG poster
// covers the frame until the scene renders, and stays for no-WebGL visitors.
const BillboardScene = lazy(() => import('./building/BillboardScene'))

export default function BuildingBillboard() {
  const shared = useMemo(() => {
    const s = createSharedState()
    s.frame = 'billboard'
    return s
  }, [])
  const frame = useRef<HTMLAnchorElement>(null)
  const [mounted, setMounted] = useState(false)
  const [webgl, setWebgl] = useState(false)
  const [near, setNear] = useState(false)
  const [seen, setSeen] = useState(false)
  const [ready, setReady] = useState(false)
  const [night, setNight] = useState(false)

  useEffect(() => {
    setMounted(true)
    setWebgl(webglAvailable())
    shared.reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const el = frame.current
    if (!el || typeof IntersectionObserver === 'undefined') {
      setNear(true)
      setSeen(true)
      return
    }
    const io = new IntersectionObserver(
      ([entry]) => {
        setNear(entry.isIntersecting)
        if (entry.isIntersecting) setSeen(true)
      },
      { rootMargin: '240px 0px' }
    )
    io.observe(el)
    return () => io.disconnect()
  }, [shared])

  return (
    <section className="billboard" data-component="BuildingBillboard" data-night={night ? 'true' : 'false'}>
      <div className="billboard__grid">
        <div className="billboard__copy">
          <div className="section-label">The Building · interactive tour</div>
          <h2 className="billboard__title">
            Every app is a building.
            <br />
            Take the tour.
          </h2>
          <p className="billboard__body">
            Code is the blueprint. The running app is the building. A sightmap is the wayfinding that lets
            an agent find its way around. Scroll through a living model of how it all fits together, from
            the first drawing to the lights coming on.
          </p>
          <div className="billboard__ctas">
            <a href="/building" className="btn-primary">
              Enter the building →
            </a>
            <span className="billboard__meta">Nine chapters · scroll-driven · works on mobile</span>
          </div>
        </div>

        <a ref={frame} href="/building" className="billboard__frame" aria-label="Enter the building tour">
          <Poster hidden={ready} />
          {mounted && webgl && seen && (
            <Suspense fallback={null}>
              <BillboardScene shared={shared} active={near} onReady={() => setReady(true)} onNight={setNight} />
            </Suspense>
          )}
          <span className="billboard__chip" aria-hidden="true">
            <span className="dot" />
            live · {JOURNEYS.length} journeys walking
          </span>
          <span className="billboard__enter" aria-hidden="true">
            Enter the building →
          </span>
        </a>
      </div>
    </section>
  )
}
