import { lazy, Suspense, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { invalidate } from '@react-three/fiber'
import { CHAPTERS } from './chapters'
import { createSharedState, isMobileViewport, SharedStateContext } from './state'
import { webglAvailable } from './webgl'
import Poster from './Poster'
import BuildingNav from './BuildingNav'

// The page: a fixed stage (poster, then the WebGL scene once it loads) behind
// a column of scroll chapters. Scroll position is measured against the
// chapter sections and written into the shared mutable state that the scene
// reads each frame; React only re-renders when the active chapter changes.
//
// Everything a crawler or a no-JS visitor needs is in the DOM: the chapter
// text is real markup and the poster is inline SVG. The scene is decoration
// on top — aria-hidden, lazy, and skipped entirely when WebGL is missing.
const Scene = lazy(() => import('./Scene'))

/** How long to keep drawing after a scroll or resize, in demand mode. */
const SETTLE_MS = 2500

/** Renders `code` spans in chapter copy. */
function inline(text: string): ReactNode[] {
  return text.split('`').map((part, i) => (i % 2 === 1 ? <code key={i}>{part}</code> : part))
}

export default function BuildingExperience() {
  const shared = useMemo(createSharedState, [])
  const [mounted, setMounted] = useState(false)
  const [webgl, setWebgl] = useState(false)
  const [ready, setReady] = useState(false)
  const [active, setActive] = useState(0)
  // Real state, not just shared.mobile: DPR, shadow-map size and walker count
  // are chosen at render time, so the tier has to be able to re-render them
  // when the viewport crosses the boundary.
  const [mobile, setMobile] = useState(false)
  // Reduced motion is settled in the same effect as the tier, but it decides
  // the frame loop, so it has to be state too.
  const [reduced, setReduced] = useState(false)
  // The stage is fixed, so it costs a full GPU frame whether or not any of it
  // is on screen. Gate on both the scroll position and the tab, the way
  // BuildingBillboard already gates its own canvas.
  const [onScreen, setOnScreen] = useState(true)
  const [visible, setVisible] = useState(true)
  const stage = useRef<HTMLDivElement>(null)
  const sections = useRef<(HTMLElement | null)[]>([])

  useEffect(() => {
    setMounted(true)
    setWebgl(webglAvailable())
    shared.reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    setReduced(shared.reduced)
    setMobile(isMobileViewport())

    // Under frameloop="demand" nothing redraws unless it is asked to, and the
    // scene's damping is asymptotic — there is no frame on which it is exactly
    // finished. Rather than have each component guess when it has arrived,
    // pump frames for a fixed window after the last scroll or resize, long
    // enough for the slowest damping (the camera, λ 3) to converge, then stop
    // dead. Harmless in 'always' mode, where the frames were coming anyway.
    let pumpUntil = 0
    let pumpRaf = 0
    const pump = () => {
      pumpUntil = performance.now() + SETTLE_MS
      if (pumpRaf) return
      const step = () => {
        invalidate()
        pumpRaf = performance.now() < pumpUntil ? requestAnimationFrame(step) : 0
      }
      pumpRaf = requestAnimationFrame(step)
    }

    let raf = 0
    const measure = () => {
      const vh = window.innerHeight
      const y = window.scrollY + vh * 0.5
      const n = CHAPTERS.length
      let p = n - 1
      for (let i = 0; i < n; i++) {
        const el = sections.current[i]
        if (!el) continue
        const top = el.offsetTop
        const h = el.offsetHeight
        if (y < top) {
          p = Math.max(0, i - 0.5)
          break
        }
        if (y < top + h) {
          p = i - 0.5 + (y - top) / h
          break
        }
      }
      p = Math.min(Math.max(p, 0), n - 1)
      shared.progress = p
      const a = Math.round(p)
      setActive((prev) => (prev === a ? prev : a))
    }
    const onScroll = () => {
      if (!raf) {
        raf = requestAnimationFrame(() => {
          raf = 0
          measure()
          pump()
        })
      }
    }
    const onResize = () => {
      setMobile(isMobileViewport())
      onScroll()
    }
    const onVisibility = () => setVisible(document.visibilityState === 'visible')
    const onMove = (e: PointerEvent) => {
      if (shared.reduced || shared.mobile) return
      shared.pointer.x = (e.clientX / window.innerWidth) * 2 - 1
      shared.pointer.y = (e.clientY / window.innerHeight) * 2 - 1
    }
    measure()
    pump()
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onResize)
    window.addEventListener('pointermove', onMove, { passive: true })
    document.addEventListener('visibilitychange', onVisibility)

    const io = stage.current
      ? new IntersectionObserver((entries) => setOnScreen(entries.some((e) => e.isIntersecting)), { threshold: 0 })
      : null
    if (io && stage.current) io.observe(stage.current)

    return () => {
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onResize)
      window.removeEventListener('pointermove', onMove)
      document.removeEventListener('visibilitychange', onVisibility)
      io?.disconnect()
      if (raf) cancelAnimationFrame(raf)
      if (pumpRaf) cancelAnimationFrame(pumpRaf)
    }
  }, [shared])

  // The per-frame readers (camera framing, the pointer handler) still take the
  // tier off the shared object, so keep the two in step.
  useEffect(() => {
    shared.mobile = mobile
  }, [mobile, shared])

  useEffect(() => {
    shared.focus = CHAPTERS[active].focus ?? null
  }, [active, shared])

  const night = CHAPTERS[active].scene.night >= 0.5
  const hud = CHAPTERS[active].hud

  // Off-screen or backgrounded: stop entirely. Reduced motion: the scene is a
  // still once it settles, so draw on demand instead of paying for a redraw of
  // an unchanging image 60 times a second.
  const frameloop = !onScreen || !visible ? 'never' : reduced ? 'demand' : 'always'

  const goTo = (i: number) => {
    sections.current[i]?.scrollIntoView({ behavior: shared.reduced ? 'auto' : 'smooth', block: 'start' })
  }

  return (
    <SharedStateContext.Provider value={shared}>
      <div className="bld" data-night={night ? 'true' : 'false'} data-ready={ready ? 'true' : 'false'}>
        <BuildingNav />

        <div className="bld-stage" data-component="BuildingStage" aria-hidden="true" ref={stage}>
          <Poster hidden={ready} />
          {mounted && webgl && (
            <Suspense fallback={null}>
              <Scene shared={shared} mobile={mobile} frameloop={frameloop} onReady={() => setReady(true)} />
            </Suspense>
          )}
        </div>

        {/* Where the scene's 3D-anchored labels land. A sibling of the stage,
            never a child of it — see .bld-overlay in building.css. Left
            readable by assistive tech: the text in it (floor directory, room
            tags, memory notes) is content, not decoration like the canvas. */}
        <div className="bld-overlay" data-component="BuildingOverlay" ref={shared.overlay} />

        <main className="bld-story">
          {CHAPTERS.map((ch, i) => {
            const isActive = i === active
            const Heading = i === 0 ? 'h1' : 'h2'
            return (
              <section
                key={ch.id}
                id={ch.id}
                ref={(el) => {
                  sections.current[i] = el
                }}
                className="bld-chapter"
                data-component="BuildingChapter"
                data-chapter={ch.id}
                data-active={isActive ? 'true' : 'false'}
              >
                <div className="bld-card">
                  <div className="bld-card__eyebrow">
                    {ch.eyebrow}
                    {ch.badge && <span className="bld-card__badge">{ch.badge}</span>}
                  </div>
                  <Heading className="bld-card__title">{ch.title}</Heading>
                  <p className="bld-card__body">{inline(ch.body)}</p>
                  {ch.ctas && (
                    <div className="bld-card__ctas">
                      {ch.ctas.map((c) => (
                        <a
                          key={c.href}
                          href={c.href}
                          className={c.primary ? 'btn-primary' : 'btn-secondary'}
                          {...(c.external ? { target: '_blank', rel: 'noreferrer' } : {})}
                        >
                          {c.label}
                        </a>
                      ))}
                    </div>
                  )}
                  {i === 0 && (
                    <button type="button" className="bld-hint" onClick={() => goTo(1)}>
                      Scroll to build <span aria-hidden="true">↓</span>
                    </button>
                  )}
                </div>
              </section>
            )
          })}
        </main>

        <aside className="bld-hud" data-component="BuildingHud" aria-hidden="true" key={active}>
          <div className="bld-hud__title">
            {hud.title}
            <span className="bld-hud__live">live</span>
          </div>
          {hud.rows.map(([k, v]) => (
            <div key={k} className="bld-hud__row">
              <span>{k}</span>
              <i />
              <b>{v}</b>
            </div>
          ))}
        </aside>

        <div className="bld-rail" data-component="BuildingRail" role="navigation" aria-label="Chapters">
          <div className="bld-rail__track">
            <div className="bld-rail__fill" style={{ width: `${(active / (CHAPTERS.length - 1)) * 100}%` }} />
          </div>
          <div className="bld-rail__dots">
            {CHAPTERS.map((ch, i) => (
              <button
                key={ch.id}
                type="button"
                className="bld-rail__dot"
                aria-pressed={i === active}
                aria-label={ch.eyebrow}
                onClick={() => goTo(i)}
              >
                <span className="bld-rail__label">{ch.short}</span>
              </button>
            ))}
          </div>
        </div>

        <footer className="bld-footer">
          <span>
            sightmap · open-source spec by{' '}
            <a href="https://subtext.fullstory.com" target="_blank" rel="noreferrer">
              Subtext
            </a>
          </span>
          <a href="/">Home</a>
          <a href="/atlas">Atlas</a>
          <a href="/blog">Blog</a>
          <a href="https://docs.sightmap.org">Docs</a>
        </footer>
      </div>
    </SharedStateContext.Provider>
  )
}
