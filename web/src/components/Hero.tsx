export default function Hero() {
  return (
    <section className="hero" data-component="Hero">
      <div className="container">
        <div className="hero-badge">
          <span className="dot"></span>
          Open-source spec · v0.1
        </div>
        <h1>
          Teach agents how to see<br className="hidden md:inline" />{" "}
          and use your site.
        </h1>
        <p className="hero-sub">
          <code>sitemap.xml</code> tells search engines how to crawl your site.<br />
          <code>.sightmap/</code> <span className="hero-teaches">teaches</span> agents how to use it.
        </p>
        <div className="hero-ctas">
          <a href="https://github.com/sightmap/sightmap" target="_blank" rel="noreferrer" className="btn-primary">
            View the spec on GitHub
          </a>
          <a href="https://docs.sightmap.org/" className="btn-secondary">
            Read the docs →
          </a>
        </div>
        <p className="hero-robust">
          A YAML map of your app's views, components, and API routes — checked into your repo, shared across every agent, and learned from the running app, not the source code.
        </p>
      </div>

      <div className="hero-diff-wrap">
        <div className="hero-diff-caption">
          What a coding agent sees when it opens a date picker —
          <br className="hidden md:inline" />{" "}
          before and after a sightmap.
        </div>
        <div className="hero-diff">
          <div className="code-block">
            <div className="code-header">
              <span className="code-filename">
                snapshot <span className="code-filename-dim">— without</span> <code>.sightmap/</code>
              </span>
              <span className="code-lang">a11y</span>
            </div>
            <pre><code>
<span className="c-com">uid=1_0</span>  RootWebArea <span className="c-str">"Book a flight"</span>{'\n'}
<span className="c-com">uid=1_3</span>  textbox <span className="c-str">"Departure date"</span>{'\n'}
<span className="c-com">uid=1_8</span>  button <span className="c-str">"Previous Month"</span>{'\n'}
<span className="c-com">uid=1_9</span>  button <span className="c-str">"Next Month"</span>{'\n'}
<span className="c-com">uid=1_10</span> generic <span className="c-str">"July 2025"</span>{'\n'}
<span className="c-com">uid=1_19</span> gridcell <span className="c-str">"Choose Tuesday, July 1st, 2025"</span>{'\n'}
<span className="c-com">uid=1_20</span> gridcell <span className="c-str">"Choose Wednesday, July 2nd, 2025"</span>{'\n'}
<span className="c-com">uid=1_21</span> gridcell <span className="c-str">"Choose Thursday, July 3rd, 2025"</span>{'\n'}
         <span className="c-com">…28 more gridcells…</span>
            </code></pre>
          </div>

          <div className="code-block code-block--highlight">
            <div className="code-header">
              <span className="code-filename">
                snapshot <span className="code-filename-dim">— with</span> <code>.sightmap/</code>
              </span>
              <span className="code-lang">a11y</span>
            </div>
            <pre><code>
<span className="c-accent">[View: FlightSearch <span className="c-str">"/search"</span>]</span>{'\n'}
<span className="c-accent">[Guide]</span>{'\n'}
<span className="c-val">-</span> DepartureDatePicker accepts typed <span className="c-str">YYYY-MM-DD</span>{'\n'}
  <span className="c-com">// skips opening the calendar</span>{'\n'}
<span className="c-val">-</span> Arrow keys navigate the grid; Enter selects; Esc closes{'\n'}
<span className="c-val">-</span> Past dates render but are <span className="c-str">aria-disabled</span>{'\n'}
<span className="c-val">-</span> Range: 1st click = start, 2nd = end, 3rd resets{'\n'}
{'\n'}
<span className="c-com">uid=1_0</span>  RootWebArea <span className="c-str">"Book a flight"</span>{'\n'}
<span className="c-com">uid=1_1</span>  <span className="c-key">FlightSearchForm</span> <span className="c-val">visible</span>{'\n'}
<span className="c-com">uid=1_3</span>    <span className="c-key">DepartureDatePicker</span> <span className="c-com">[src: src/components/DatePicker.tsx]</span> <span className="c-val">visible interactive</span>{'\n'}
<span className="c-com">uid=1_4</span>      <span className="c-key">date-input</span> <span className="c-val">visible interactive</span>{'\n'}
<span className="c-com">uid=1_8</span>      <span className="c-key">prev-month</span> <span className="c-val">visible interactive</span>{'\n'}
<span className="c-com">uid=1_9</span>      <span className="c-key">next-month</span> <span className="c-val">visible interactive</span>{'\n'}
<span className="c-com">uid=1_10</span>     <span className="c-key">month-label</span> <span className="c-str">"July 2025"</span> <span className="c-val">visible</span>{'\n'}
<span className="c-com">uid=1_19</span>     <span className="c-key">day</span> <span className="c-str">"Choose Tuesday, July 1st, 2025"</span> <span className="c-val">visible interactive</span>{'\n'}
         <span className="c-com">…30 more days…</span>
            </code></pre>
          </div>
        </div>
      </div>
    </section>
  )
}
