import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import CopyButton from '@/components/CopyButton'
import SightkickLogo from '@/components/SightkickLogo'
import { SIGHTKICK_DESCRIPTION, SIGHTKICK_TITLE } from '../../scripts/lib/site'

const INSTALL = 'npm install -g @sightmap/sightkick'

// The one-click prompt. Written to be pasted whole into a coding agent, so it
// carries the install, the skills, and the shape of the job — not a summary of
// this page. Kept as one string so the copy button and the rendered block can
// never drift.
const AGENT_PROMPT = `Build a WebMCP tool layer for this app with sightmap + sightkick.

1. npm install -g @sightmap/sightmap @sightmap/sightkick
2. sightmap skills install
   Read sightmap-authoring, sightmap-browser, sightkick-authoring and
   sightkick-debug before you start. They are the source of truth.
3. Start a session against the running app:
   sightmap browser start --url <APP_URL>
4. Follow sightmap-authoring to map the 1-3 pages the tools need. Verify every
   selector with sel-probe before it goes into YAML, get each page to 0
   orphaned nodes, and run sightmap capture on each view.
5. Follow sightkick-authoring to write .sightkick/tools.yaml. Include at least
   one read tool. Declare a journey so results carry guidance.
6. sightkick build . --verify -o tools.ir.json
7. sightkick browser .            # starts the session, persist-injects the tools
   sightmap browser mcp list      # confirm they registered
   sightkick call . <tool> --param k=v --via cli
   sightkick call . <tool> --param k=v --via webmcp
8. Report what you built, the JSON each tool returned, and anything that failed
   with its actual error text.`

const USE_CASES: { tag: string; title: string; body: React.ReactNode }[] = [
  {
    tag: 'testing',
    title: 'Tests that survive a refactor',
    body: (
      <>
        A test written against <code>[data-testid=&quot;row-3&quot;] &gt; button.primary</code> breaks when
        someone reorders a list. A tool call names what it wants. The selector lives in one
        place, the corpus, and every tool that reaches that element is fixed by editing it once.
      </>
    ),
  },
  {
    tag: 'verification',
    title: 'Agentic verification',
    body: (
      <>
        After a deploy, an agent calls the same three tools and compares the structured result
        to what it expected. <code>ok:false</code> and the failing step come back as JSON, so a
        run either passes or says which step it died on and why.
      </>
    ),
  },
  {
    tag: 'computer use',
    title: 'Fewer screenshots per task',
    body: (
      <>
        Driving a UI from pixels costs a screenshot, a guess, and a click, repeated. A tool call
        costs one round trip and returns typed fields. The agent spends its context on the
        decision rather than on working out which of 24 anchors is the right one.
      </>
    ),
  },
  {
    tag: 'agent experience',
    title: 'Your app tells agents what it offers',
    body: (
      <>
        WebMCP is how a page hands the agent in the same tab a list of callable actions. Almost
        no production app declares any yet. Sightkick compiles them from the outside, so you can
        offer that surface without waiting for a rewrite.
      </>
    ),
  },
  {
    tag: 'ergonomics',
    title: 'YAML, not a driver script',
    body: (
      <>
        A tool is a name, its params, ordered steps, and the shape it returns. No page objects,
        no bespoke automation harness, no framework adapter. The compiler resolves every
        reference against the corpus and tells you which name it could not find.
      </>
    ),
  },
  {
    tag: 'guidance',
    title: 'Journeys, so an agent knows what comes next',
    body: (
      <>
        Declare the order tools tend to run in and the compiler attaches breadcrumbs to every
        result. After <code>get_latest_deploy_status</code> the answer itself suggests{' '}
        <code>list_recent_deploys</code>, with the reason you wrote.
      </>
    ),
  },
]

// The install command's face. The whole field is the button, so the trailing
// glyph is only an affordance — it swaps to a check on a successful copy, and
// the sr-only word beside it is what the button's aria-live actually announces.
function installFace(copied: boolean) {
  return (
    <>
      <span className="sk-copycmd__prompt">$</span>
      <span className="sk-copycmd__text">{INSTALL}</span>
      <span className="sk-copycmd__hint" data-copied={copied ? 'true' : 'false'}>
        <svg
          viewBox="0 0 16 16"
          width="15"
          height="15"
          aria-hidden="true"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.4"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {copied ? (
            <path d="M3 8.4 6.3 11.7 13 5" />
          ) : (
            <>
              <rect x="5.75" y="5.75" width="7.5" height="7.5" rx="1.6" />
              <path d="M10.4 3.3V3.1A1.6 1.6 0 0 0 8.8 1.5H4.35a1.6 1.6 0 0 0-1.6 1.6v4.45a1.6 1.6 0 0 0 1.6 1.6h.2" />
            </>
          )}
        </svg>
        <span className="sr-only">{copied ? 'Copied' : 'Copy'}</span>
      </span>
    </>
  )
}

export default function Sightkick() {
  return (
    <>
      <Seo title={SIGHTKICK_TITLE} description={SIGHTKICK_DESCRIPTION} />
      <Navigation />

      {/* ---------------- HERO ---------------- */}
      <section className="sk-hero sk-dark" data-component="SightkickHero">
        <div className="sk-hero__plate" aria-hidden="true">
          <img src="/sightkick/hero.webp" alt="" width={2400} height={1600} />
        </div>
        <div className="container sk-hero__body">
          <SightkickLogo className="sk-hero__logo" />
          <h1>
            The front desk<br className="hidden md:inline" />{' '}
            for your web app.
          </h1>
          <p className="sk-hero__sub">
            Sightkick compiles a <code>.sightmap/</code> corpus and a short YAML tool layer into{' '}
            <a href="https://webmachinelearning.github.io/webmcp/" target="_blank" rel="noreferrer">
              WebMCP
            </a>{' '}
            tools. Agents call <code>search_flights(origin, destination, date)</code> instead of
            hunting for the search box.
          </p>

          <div className="sk-hero__ctas">
            <CopyButton
              className="sk-copycmd"
              value={INSTALL}
              label={installFace(false)}
              done={installFace(true)}
              title="Copy the install command"
            />
            <CopyButton
              className="btn-primary sk-promptbtn"
              value={AGENT_PROMPT}
              label={
                <>
                  <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="currentColor">
                    <path d="M5.5 1.5h5a1 1 0 0 1 1 1v1h1.5a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1h-9a1 1 0 0 1-1-1V4.5a1 1 0 0 1 1-1H5.5v-1a1 1 0 0 1 1-1Zm0 2h5v-1h-5v1Zm-2 1v9h9v-9h-9Z" />
                  </svg>
                  Copy prompt for your agent
                </>
              }
              done="Prompt copied"
              title="Copy a prompt that gets an agent building a tool layer"
            />
          </div>
          <p className="sk-hero__note">
            Paste the prompt into Claude Code, Cursor, or any agent that can run a shell. It maps
            your app and builds a working tool layer.
          </p>
        </div>
      </section>

      {/* ---------------- OVERVIEW ---------------- */}
      <section className="sk-overview" data-component="SightkickOverview">
        <div className="container">
          <div className="section-label">What it is</div>
          <h2>Agents arrive at your app with no idea where anything is.</h2>
          <p className="section-desc">
            They land in the lobby holding the blueprints. So they read the DOM, guess at a
            selector, click, screenshot, and guess again. It works often enough to be tempting and
            breaks the first time someone reorders a list.
          </p>
          <p className="section-desc">
            Real buildings solve this with a desk by the door. You walk up, say what you came for,
            and someone tells you the floor. Sightkick puts that desk in your app. The building
            already knows its own rooms, because a <code>.sightmap/</code> corpus named them, and
            Sightkick turns that knowledge into a short list of things an agent can ask for by
            name.
          </p>

          <div className="sk-pair">
            <div className="sk-pair__col">
              <div className="sk-pair__label">Without a tool layer</div>
              <div className="code-block">
                <div className="code-header">
                  <span className="code-filename">agent transcript</span>
                  <span className="code-lang">wandering</span>
                </div>
                <pre><code><span className="c-com">→</span> screenshot the page{'\n'}
<span className="c-com">→</span> find the search box{'\n'}
<span className="c-com">→</span> click <span className="c-str">div.sc-hKgILt &gt; input</span>{'\n'}
<span className="c-com">→</span> type, screenshot, did it take?{'\n'}
<span className="c-com">→</span> find the submit button{'\n'}
<span className="c-com">→</span> screenshot, parse the results{'\n'}
<span className="c-com">…</span> 11 steps, 6 screenshots</code></pre>
              </div>
            </div>
            <div className="sk-pair__col">
              <div className="sk-pair__label sk-pair__label--good">With a tool layer</div>
              <div className="code-block code-block--highlight">
                <div className="code-header">
                  <span className="code-filename">agent transcript</span>
                  <span className="code-lang">front desk</span>
                </div>
                <pre><code><span className="c-com">→</span> <span className="c-key">search_flights</span>(<span className="c-str">&quot;SFO&quot;</span>, <span className="c-str">&quot;JFK&quot;</span>, <span className="c-str">&quot;2026-10-02&quot;</span>){'\n'}
{'\n'}
{'{'} <span className="c-key">&quot;ok&quot;</span>: <span className="c-val">true</span>,{'\n'}
{'  '}<span className="c-key">&quot;items&quot;</span>: [{'{'} <span className="c-key">&quot;fare&quot;</span>: <span className="c-str">&quot;$214&quot;</span>, <span className="c-key">&quot;stops&quot;</span>: <span className="c-str">&quot;nonstop&quot;</span> {'}'}, …],{'\n'}
{'  '}<span className="c-key">&quot;guidance&quot;</span>: [{'{'} <span className="c-key">&quot;tool&quot;</span>: <span className="c-str">&quot;select_fare&quot;</span> {'}'}]{'\n'}
{'}'}{'\n'}
{'\n'}
<span className="c-com">…</span> 1 step, 0 screenshots</code></pre>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ---------------- USE CASES ---------------- */}
      <section className="sk-cases" data-component="SightkickUseCases">
        <div className="container container--wide">
          <div className="section-label">What it is for</div>
          <h2>Six jobs that get easier.</h2>
          <div className="sk-cases__grid">
            {USE_CASES.map((c) => (
              <article key={c.tag} className="sk-case">
                <div className="sk-case__tag">{c.tag}</div>
                <h3>{c.title}</h3>
                <p>{c.body}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      {/* ---------------- HOW IT SITS ON SIGHTMAP ---------------- */}
      <section className="sk-stack sk-dark" data-component="SightkickStack">
        <div className="sk-stack__plate" aria-hidden="true">
          <img src="/sightkick/wide.webp" alt="" width={2400} height={1600} loading="lazy" />
        </div>
        <div className="container sk-stack__body">
          <div className="section-label">How it is built</div>
          <h2>Sightkick is the second half of Sightmap.</h2>
          <p className="section-desc">
            Sightmap maps the building. Sightkick opens the desk. They are separate CLIs and
            separate npm packages, and Sightkick reads the corpus Sightmap produces, so the
            selectors, component names, and memory notes you already committed are the thing the
            tools are written against.
          </p>

          <ol className="sk-layers">
            <li>
              <span className="sk-layers__num">01</span>
              <div>
                <h3><code>.sightmap/</code> — the map</h3>
                <p>
                  Views, components and the properties extracted off them, authored with the
                  Sightmap CLI against the running app. This is the only place a CSS selector
                  appears.
                </p>
              </div>
            </li>
            <li>
              <span className="sk-layers__num">02</span>
              <div>
                <h3><code>.sightkick/</code> — the tool layer</h3>
                <p>
                  Any number of YAML files, merged. Each tool names its params, its ordered steps,
                  and the shape it returns, all addressed by component name rather than by
                  selector.
                </p>
              </div>
            </li>
            <li>
              <span className="sk-layers__num">03</span>
              <div>
                <h3><code>sightkick build</code> — the IR</h3>
                <p>
                  One self-contained JSON artifact. The compiler resolves every reference against
                  the corpus and names the ones it cannot find. <code>--verify</code> checks each
                  extractor against a captured snapshot and warns on fields that come back empty.
                </p>
              </div>
            </li>
            <li>
              <span className="sk-layers__num">04</span>
              <div>
                <h3>The runtime — the desk itself</h3>
                <p>
                  A 19&nbsp;KB bundle registers the IR on <code>document.modelContext</code>. On
                  Chrome for Testing that is the browser&rsquo;s native WebMCP surface, so any
                  WebMCP client reads the tools the same way it would read a site&rsquo;s own.
                </p>
              </div>
            </li>
          </ol>
        </div>
      </section>

      {/* ---------------- GETTING STARTED / README ---------------- */}
      <section id="start" className="sk-start" data-component="SightkickStart">
        <div className="container container--wide">
          <div className="section-label">Get started</div>
          <h2>From an unmapped app to a callable tool.</h2>
          <p className="section-desc">
            Sightkick needs a corpus to compile against. If the app has no <code>.sightmap/</code>{' '}
            yet, step 01 builds one; if it does, start at step 02.
          </p>

          <div className="gs-steps">
            <div className="gs-step">
              <span className="gs-step-num">01</span>
              <div className="gs-step-body">
                <h3>Install both CLIs and the skills</h3>
                <p className="gs-step-intro">
                  Both ship as prebuilt native binaries through npm, so no Go toolchain is needed.
                  <code>skills install</code> writes the four agent playbooks into{' '}
                  <code>~/.agents/skills</code>.
                </p>
                <div className="code-block">
                  <div className="code-header">
                    <span className="code-filename">your project root</span>
                    <span className="code-lang">shell</span>
                  </div>
                  <pre><code><span className="c-com">$</span> npm install -g <span className="c-str">@sightmap/sightmap @sightmap/sightkick</span>{'\n'}
<span className="c-com">$</span> sightkick skills install{'\n'}
installed 2 sightkick skill(s) → ~/.agents/skills{'\n'}
{'  '}<span className="c-key">sightkick-authoring</span>{'\n'}
{'  '}<span className="c-key">sightkick-debug</span>{'\n'}
installing the supporting sightmap skills …{'\n'}
{'  '}<span className="c-key">sightmap-authoring</span>{'\n'}
{'  '}<span className="c-key">sightmap-browser</span></code></pre>
                </div>
              </div>
            </div>

            <div className="gs-step">
              <span className="gs-step-num">02</span>
              <div className="gs-step-body">
                <h3>Write the tool layer</h3>
                <p className="gs-step-intro">
                  <code>.sightkick/</code> sits beside <code>.sightmap/</code> and every{' '}
                  <code>*.yaml</code> inside it merges into one manifest. A tool is params, steps
                  and a returns shape. <code>ensure_view</code> scopes it to one view in the
                  corpus.
                </p>
                <div className="code-block">
                  <div className="code-header">
                    <span className="code-filename">.sightkick/tools.yaml</span>
                    <span className="code-lang">yaml</span>
                  </div>
                  <pre><code><span className="c-key">version</span>: <span className="c-val">1</span>{'\n'}
<span className="c-key">name</span>: flights{'\n'}
{'\n'}
<span className="c-key">tools</span>:{'\n'}
{'  '}- <span className="c-key">name</span>: search_flights{'\n'}
{'    '}<span className="c-key">description</span>: Search flights for a route and date.{'\n'}
{'    '}<span className="c-key">ensure_view</span>: FlightSearch{'\n'}
{'    '}<span className="c-key">params</span>:{'\n'}
{'      '}- {'{'} <span className="c-key">name</span>: origin, <span className="c-key">type</span>: string, <span className="c-key">required</span>: <span className="c-val">true</span> {'}'}{'\n'}
{'      '}- {'{'} <span className="c-key">name</span>: destination, <span className="c-key">type</span>: string, <span className="c-key">required</span>: <span className="c-val">true</span> {'}'}{'\n'}
{'    '}<span className="c-key">steps</span>:{'\n'}
{'      '}- <span className="c-key">fill</span>: {'{'} <span className="c-key">query</span>: OriginInput, <span className="c-key">value</span>: <span className="c-str">&quot;{'{{origin}}'}&quot;</span> {'}'}{'\n'}
{'      '}- <span className="c-key">fill</span>: {'{'} <span className="c-key">query</span>: DestinationInput, <span className="c-key">value</span>: <span className="c-str">&quot;{'{{destination}}'}&quot;</span> {'}'}{'\n'}
{'      '}- <span className="c-key">click</span>: {'{'} <span className="c-key">query</span>: SearchButton {'}'}{'\n'}
{'      '}- <span className="c-key">wait_for</span>: {'{'} <span className="c-key">query</span>: <span className="c-str">&apos;FareCard#0&apos;</span> {'}'}{'\n'}
{'    '}<span className="c-key">returns</span>:{'\n'}
{'      '}<span className="c-key">list</span>:{'\n'}
{'        '}<span className="c-key">rows</span>: FareCard{'\n'}
{'        '}<span className="c-key">fields</span>: {'{'} <span className="c-key">fare</span>: price, <span className="c-key">stops</span>: stops {'}'}</code></pre>
                </div>
                <p className="gs-followup">
                  Every name here — <code>FareCard</code>, <code>price</code>, <code>stops</code> —
                  is a component or property declared in <code>.sightmap/</code>. A name the corpus
                  does not have is a compile error with the candidates printed.
                </p>
              </div>
            </div>

            <div className="gs-step">
              <span className="gs-step-num">03</span>
              <div className="gs-step-body">
                <h3>Compile and check it</h3>
                <p className="gs-step-intro">
                  <code>--verify</code> runs the returns extractors against a captured snapshot of
                  the view, so a field that resolves empty on every row is caught before an agent
                  ever calls the tool.
                </p>
                <div className="code-block">
                  <div className="code-header">
                    <span className="code-filename">your project root</span>
                    <span className="code-lang">shell</span>
                  </div>
                  <pre><code><span className="c-com">$</span> sightkick build . --verify -o tools.ir.json{'\n'}
<span className="c-val">✓</span> wrote 2 tool(s) to tools.ir.json</code></pre>
                </div>
              </div>
            </div>

            <div className="gs-step">
              <span className="gs-step-num">04</span>
              <div className="gs-step-body">
                <h3>Run the tools on the live page</h3>
                <p className="gs-step-intro">
                  <code>sightkick browser</code> builds the IR, starts a session and injects the
                  runtime so it survives navigations. <code>call</code> invokes one tool and prints
                  its result as JSON, exiting non-zero when a tool reports failure.
                </p>
                <div className="code-block">
                  <div className="code-header">
                    <span className="code-filename">your project root</span>
                    <span className="code-lang">shell</span>
                  </div>
                  <pre><code><span className="c-com">$</span> sightkick browser .{'\n'}
<span className="c-val">✓</span> sightkick tools are live on the page{'\n'}
<span className="c-com">$</span> sightmap browser mcp list{'\n'}
WebMCP (native) — 2 tool(s){'\n'}
<span className="c-com">$</span> sightkick call . search_flights --param origin=SFO --param destination=JFK{'\n'}
{'{'} <span className="c-key">&quot;ok&quot;</span>: <span className="c-val">true</span>, <span className="c-key">&quot;items&quot;</span>: [{'{'} <span className="c-key">&quot;fare&quot;</span>: <span className="c-str">&quot;$214&quot;</span>, <span className="c-key">&quot;stops&quot;</span>: <span className="c-str">&quot;nonstop&quot;</span> {'}'}] {'}'}</code></pre>
                </div>
                <p className="gs-followup">
                  <code>--via cli</code> drives real browser input and runs from any page.{' '}
                  <code>--via webmcp</code> asks the page&rsquo;s own registered tool to run itself,
                  which is the path a real WebMCP client takes.
                </p>
              </div>
            </div>
          </div>

          {/* one-click prompt, repeated where someone is ready to act */}
          <div className="sk-promptcard">
            <div className="sk-promptcard__head">
              <div>
                <div className="section-label">Or hand it to an agent</div>
                <h3>One prompt, the whole loop.</h3>
              </div>
              <CopyButton
                className="btn-primary sk-promptbtn"
                value={AGENT_PROMPT}
                label="Copy prompt"
                done="Copied"
                title="Copy the agent prompt"
              />
            </div>
            <div className="code-block">
              <div className="code-header">
                <span className="code-filename">to your agent</span>
                <span className="code-lang">prompt</span>
              </div>
              <pre className="sk-promptcard__pre"><code>{AGENT_PROMPT}</code></pre>
            </div>
          </div>
        </div>
      </section>

      {/* ---------------- CLI REFERENCE ---------------- */}
      <section className="sk-ref" data-component="SightkickReference">
        <div className="container container--wide">
          <div className="section-label">Reference</div>
          <h2>The whole CLI.</h2>
          <div className="sk-ref__table">
            {[
              ['sightkick build <dir>', 'Compile .sightkick/ + .sightmap/ into tool IR. --verify checks extractors against captured snapshots.'],
              ['sightkick browser <dir>', 'Build, start a sightmap session, and persist-inject the runtime so tools re-register on every document.'],
              ['sightkick call <dir> <tool>', 'Invoke one tool with --param k=v and print its ToolResult as JSON. --via cli or --via webmcp.'],
              ['sightkick runtime', 'Emit the runtime bundle to inject into a page you serve yourself.'],
              ['sightkick skills install', 'Install the sightkick and sightmap agent skills into ~/.agents/skills.'],
            ].map(([cmd, desc]) => (
              <div className="sk-ref__row" key={cmd}>
                <code>{cmd}</code>
                <span>{desc}</span>
              </div>
            ))}
          </div>

          <div className="sk-ref__ctas">
            <a href="https://github.com/sightmap/sightkick" target="_blank" rel="noreferrer" className="btn-primary">
              View on GitHub
            </a>
            <a href="https://docs.sightmap.org/sightkick" className="btn-secondary">
              Read the docs →
            </a>
            <a href="/building#web-mcp" className="btn-secondary">
              See it in the building →
            </a>
          </div>
        </div>
      </section>

      <Footer />
    </>
  )
}
