import { useEffect, useRef, useState } from 'react'
import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import { WEBMCP_DESCRIPTION, WEBMCP_TITLE } from '../../scripts/lib/site'

/**
 * The click-path a computer-use agent walks to do one ordinary thing. Each
 * entry is one model turn; `shot` marks the turns that also cost a screenshot.
 * Deliberately unremarkable — the point is that nothing here is hard, there is
 * just a lot of it.
 */
const TRAJECTORY: { act: string; shot?: boolean }[] = [
  { act: 'screenshot — storefront', shot: true },
  { act: 'click "Search"' },
  { act: 'screenshot — search open', shot: true },
  { act: 'type "desk chair"' },
  { act: 'screenshot — suggestions', shot: true },
  { act: 'press Enter' },
  { act: 'screenshot — results, still loading', shot: true },
  { act: 'screenshot — results settled', shot: true },
  { act: 'read prices, pick the cheapest' },
  { act: 'screenshot — product page', shot: true },
]

const SHOTS = TRAJECTORY.filter((t) => t.shot).length

function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(false)
  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    setReduced(mq.matches)
    const on = (e: MediaQueryListEvent) => setReduced(e.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])
  return reduced
}

/**
 * Plays the trajectory once the tape scrolls into view, then holds. Never runs
 * during prerender (effects don't fire in renderToString), and the initial
 * state shows every step — so the static HTML and the reduced-motion path both
 * render the complete comparison rather than an empty box.
 */
function TurnTape() {
  const reduced = useReducedMotion()
  const [step, setStep] = useState(TRAJECTORY.length)
  const ref = useRef<HTMLDivElement>(null)
  const played = useRef(false)

  useEffect(() => {
    if (reduced || played.current) return
    const el = ref.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver(
      (entries) => {
        if (!entries[0]?.isIntersecting || played.current) return
        played.current = true
        io.disconnect()
        setStep(0)
        let i = 0
        const timer = window.setInterval(() => {
          i += 1
          setStep(i)
          if (i >= TRAJECTORY.length) window.clearInterval(timer)
        }, 260)
      },
      { threshold: 0.3 }
    )
    io.observe(el)
    return () => io.disconnect()
  }, [reduced])

  const shown = reduced ? TRAJECTORY.length : step

  return (
    <div className="wmcp-tape" ref={ref} data-component="TurnTape">
      <div className="wmcp-tape__col">
        <div className="wmcp-tape__head">
          <span className="wmcp-tape__who">Computer-use agent</span>
          <span className="wmcp-tape__count">
            {Math.min(shown, TRAJECTORY.length)} / {TRAJECTORY.length} turns
          </span>
        </div>
        <ol className="wmcp-tape__list">
          {TRAJECTORY.map((t, i) => (
            <li
              key={t.act}
              className="wmcp-tape__step"
              data-on={i < shown ? 'true' : 'false'}
              data-shot={t.shot ? 'true' : 'false'}
            >
              <span className="wmcp-tape__n">{String(i + 1).padStart(2, '0')}</span>
              <span className="wmcp-tape__act">{t.act}</span>
              {t.shot && <span className="wmcp-tape__tag">img</span>}
            </li>
          ))}
        </ol>
        <div className="wmcp-tape__foot">
          {SHOTS} screenshots through the model
        </div>
      </div>

      <div className="wmcp-tape__col wmcp-tape__col--win">
        <div className="wmcp-tape__head">
          <span className="wmcp-tape__who">Same agent, site tools on</span>
          <span className="wmcp-tape__count">1 / 1 turn</span>
        </div>
        <div className="wmcp-tape__call">
          <pre><code>
<span className="c-type">search_products</span>({'{'}{'\n'}
{'  '}<span className="c-key">query</span>: <span className="c-str">"desk chair"</span>,{'\n'}
{'  '}<span className="c-key">max_price</span>: <span className="c-val">200</span>{'\n'}
{'}'}){'\n'}
{'\n'}
<span className="c-com">→ [{'{'} name, price, url {'}'}, …]</span>
          </code></pre>
        </div>
        <div className="wmcp-tape__foot wmcp-tape__foot--win">
          0 screenshots · structured result
        </div>
      </div>
    </div>
  )
}

const SURFACES: { label: string; title: string; body: string }[] = [
  {
    label: 'Cobrowsing',
    title: 'The agent sits beside a signed-in person',
    body: 'It is already inside the session, on the real account, on the page the customer is looking at. A tool call acts there directly instead of narrating a click-path back through pixels.',
  },
  {
    label: 'VM agents',
    title: 'Agents driving a whole computer',
    body: 'A browser on a virtual machine pays the full loop tax: every decision starts with a screenshot. Tools cut the turns that carry no judgment and leave the model the ones that do.',
  },
  {
    label: 'Agentic QA',
    title: 'Getting to the state you meant to test',
    body: 'Most of a test run is setup. A generated tool puts an account in a known state in one call — no brittle script for the boring half, and it breaks loudly when the app really changes.',
  },
]

export default function WebMCP() {
  return (
    <>
      <Seo title={WEBMCP_TITLE} description={WEBMCP_DESCRIPTION} />
      <Navigation />

      <main data-component="WebMCP">
        {/* ---- hero ---- */}
        <section className="hero wmcp-hero">
          <div className="container">
            <div className="hero-badge">
              <span className="dot"></span>
              WebMCP · generated from a sightmap
            </div>
            <h1>
              Agents shouldn't have to<br className="hidden md:inline" />{' '}
              look at your website.
            </h1>
            <p className="hero-sub">
              A computer-use agent pays for every screenshot it takes.{' '}
              <code>.sightmap/</code> compiles into{' '}
              <span className="hero-teaches">callable tools</span> a browser agent
              can use instead — for sites that never shipped any.
            </p>
            <div className="hero-ctas">
              <a href="https://docs.sightmap.org/cli/webmcp" className="btn-primary">
                Read the CLI reference
              </a>
              <a
                href="https://github.com/sightmap/sightmap/tree/main/webmcp"
                target="_blank"
                rel="noreferrer"
                className="btn-secondary"
              >
                See the generator →
              </a>
            </div>
          </div>
        </section>

        {/* ---- the tape ---- */}
        <section className="wmcp-band">
          <div className="container container--wide">
            <div className="wmcp-band__cap">
              One ordinary errand — search a store, open the cheapest result —
              as the same agent runs it with and without site tools.
            </div>
            <TurnTape />
            <p className="wmcp-band__note">
              A sketch of the shape of the work, not a benchmark. The turns are
              real turns; how much each one costs you depends on your model and
              your page.
            </p>
          </div>
        </section>

        {/* ---- the loop tax ---- */}
        <section className="wmcp-sec">
          <div className="container">
            <div className="section-label">The loop tax</div>
            <h2>Pixels are an expensive way to ask a question</h2>
            <p className="section-desc">
              Driving a UI means a screenshot before every decision and another
              after every action. Three things compound: image tokens on each
              turn, a round-trip of latency per step, and a chance to
              misread the screen that gets rolled again and again.
            </p>
            <div className="wmcp-grid">
              <div className="wmcp-card">
                <div className="wmcp-card__k">What the model sees</div>
                <p>
                  A rendered page, re-read from scratch each turn. Nothing carries
                  forward except what it wrote down.
                </p>
              </div>
              <div className="wmcp-card">
                <div className="wmcp-card__k">What it actually needed</div>
                <p>
                  Three fields and a link — data the page already fetched as JSON
                  before it drew anything.
                </p>
              </div>
              <div className="wmcp-card">
                <div className="wmcp-card__k">What breaks it</div>
                <p>
                  A moved button, a slow spinner, a cookie banner. None of it
                  changes the task; all of it derails the run.
                </p>
              </div>
            </div>
          </div>
        </section>

        {/* ---- why sightmap ---- */}
        <section className="wmcp-sec wmcp-sec--raised">
          <div className="container">
            <div className="section-label">Why this needs a sightmap</div>
            <h2>WebMCP assumes the site owner writes the tools. Almost none have.</h2>
            <p className="section-desc">
              That is the gap. A sightmap is already the thing a tool needs to
              exist: verified selectors, per-instance properties, view routes, the
              API calls the app actually makes, and the hazard notes explaining
              what goes wrong. Point the generator at it and the tools fall out —
              without waiting for the site to opt in.
            </p>

            <div className="wmcp-pipe">
              <div className="wmcp-pipe__step">
                <span className="wmcp-pipe__n">1</span>
                <div>
                  <b>Map</b>
                  <p>Walk the site with the CLI. Selectors get probed against the running app; requests get captured as they fire.</p>
                </div>
              </div>
              <div className="wmcp-pipe__step">
                <span className="wmcp-pipe__n">2</span>
                <div>
                  <b>Name the goals</b>
                  <p>A short manifest picks the user goals worth a tool and names corpus entities — never raw selectors.</p>
                </div>
              </div>
              <div className="wmcp-pipe__step">
                <span className="wmcp-pipe__n">3</span>
                <div>
                  <b>Generate</b>
                  <p>Every component, property, view, and request resolves at compile time. Anything unresolvable is an error, not a guess.</p>
                </div>
              </div>
              <div className="wmcp-pipe__step">
                <span className="wmcp-pipe__n">4</span>
                <div>
                  <b>Ship it anywhere</b>
                  <p>A snippet to inject, an ES module for the site's own owners, or a userscript for everyone else.</p>
                </div>
              </div>
            </div>

            <div className="code-block wmcp-code">
              <div className="code-header">
                <span className="code-filename">
                  terminal <span className="code-filename-dim">— map once, generate as often as you like</span>
                </span>
                <span className="code-lang">sh</span>
              </div>
              <pre><code>
<span className="c-com"># the corpus you already keep in the repo</span>{'\n'}
sightmap webmcp validate --tools webmcp.tools.yaml{'\n'}
sightmap webmcp generate --tools webmcp.tools.yaml --format all{'\n'}
{'\n'}
<span className="c-com"># regenerate on every corpus change; CI fails on drift</span>{'\n'}
sightmap webmcp generate ... --check
              </code></pre>
            </div>
          </div>
        </section>

        {/* ---- surfaces ---- */}
        <section className="wmcp-sec">
          <div className="container">
            <div className="section-label">Where it pays</div>
            <h2>Anywhere an agent is stuck driving a browser</h2>
            <div className="wmcp-surfaces">
              {SURFACES.map((s) => (
                <div className="wmcp-surface" key={s.label}>
                  <div className="wmcp-surface__label">{s.label}</div>
                  <h3>{s.title}</h3>
                  <p>{s.body}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* ---- in-page ---- */}
        <section className="wmcp-sec wmcp-sec--raised">
          <div className="container">
            <div className="section-label">Runs in the page</div>
            <h2>As the person who is already signed in</h2>
            <p className="section-desc">
              A tool registered on the page runs on that origin, with that
              session. It reaches what the signed-in user can reach and nothing
              else — the server's own rules still decide, exactly as they do for
              the UI. A tool that reads a credential must pin where it sends it,
              and a value it cannot find fails the call instead of quietly asking
              a broader question.
            </p>
            <p className="section-desc">
              Browsers without WebMCP are untouched: the bundle installs a shim
              and the page behaves as it always did.
            </p>
          </div>
        </section>

        {/* ---- cta ---- */}
        <section className="wmcp-cta">
          <div className="container">
            <h2>Map a site. Generate its tools.</h2>
            <p className="section-desc">
              The generator ships inside the Sightmap CLI. The spec, the
              reference implementation, and the agent skill that walks you
              through authoring are all open source.
            </p>
            <div className="hero-ctas">
              <a href="https://docs.sightmap.org/start/quickstart" className="btn-primary">
                Start with the quickstart
              </a>
              <a
                href="https://github.com/sightmap/sightmap"
                target="_blank"
                rel="noreferrer"
                className="btn-secondary"
              >
                GitHub →
              </a>
            </div>
          </div>
        </section>
      </main>

      <Footer />
    </>
  )
}
