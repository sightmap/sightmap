export default function AgentSection() {
  return (
    <section id="memory" data-component="MemorySection">
      <div className="container container--wide">
        <div className="section-label">Memory</div>
        <h2>What agents learn, checked in.</h2>
        <p className="section-desc">
          Sightmap starts as vocabulary — names for views, components, and API routes. It becomes <strong>memory</strong> when agents attach notes to them: the quirks, invariants, and shortcuts that never show up in source code. Those notes appear in a <code>[Guide]</code> section at the top of every snapshot, so the next agent picks up where the last one left off.
        </p>

        <div className="memory-grid">
          <div className="code-block">
            <div className="code-header">
              <span className="code-filename">.sightmap/pages/flights.yaml</span>
              <span className="code-lang">yaml</span>
            </div>
            <pre><code><span className="c-key">version</span>: <span className="c-val">1</span>{'\n'}
{'\n'}
<span className="c-key">components</span>:{'\n'}
{'  '}- <span className="c-key">name</span>: <span className="c-str">DepartureDatePicker</span>{'\n'}
{'    '}<span className="c-key">selector</span>: <span className="c-str">'[data-picker="departure"]'</span>{'\n'}
{'    '}<span className="c-key">source</span>: <span className="c-str">src/components/DatePicker.tsx</span>{'\n'}
{'    '}<span className="c-key">memory</span>:{'\n'}
{'      '}- <span className="c-str">Accepts typed YYYY-MM-DD —</span>{'\n'}
{'        '}<span className="c-str">skips opening the calendar entirely</span>{'\n'}
{'      '}- <span className="c-str">Arrow keys navigate the grid;</span>{'\n'}
{'        '}<span className="c-str">Enter selects; Esc closes</span>{'\n'}
{'      '}- <span className="c-str">Past dates render but are aria-disabled —</span>{'\n'}
{'        '}<span className="c-str">check before clicking</span>{'\n'}
{'      '}- <span className="c-str">Range: 1st click = start, 2nd = end,</span>{'\n'}
{'        '}<span className="c-str">3rd resets to new start</span></code></pre>
          </div>

          <div className="code-block code-block--highlight">
            <div className="code-header">
              <span className="code-filename">what the agent sees</span>
              <span className="code-lang">snapshot</span>
            </div>
            <pre><code><span className="c-accent">[View: FlightSearch <span className="c-str">"/search"</span>]</span>{'\n'}
<span className="c-accent">[Guide]</span>{'\n'}
<span className="c-val">-</span> DepartureDatePicker accepts typed <span className="c-str">YYYY-MM-DD</span>{'\n'}
{'  '}<span className="c-com">// skips opening the calendar entirely</span>{'\n'}
<span className="c-val">-</span> Arrow keys navigate the grid;{'\n'}
{'  '}Enter selects; Esc closes{'\n'}
<span className="c-val">-</span> Past dates render but are <span className="c-str">aria-disabled</span>{'\n'}
<span className="c-val">-</span> Range: 1st click = start, 2nd = end,{'\n'}
{'  '}3rd resets to new start{'\n'}
{'\n'}
<span className="c-com">uid=1_0</span> RootWebArea <span className="c-str">"Book a flight"</span>{'\n'}
<span className="c-com">uid=1_3</span>   <span className="c-key">DepartureDatePicker</span> <span className="c-val">visible interactive</span>{'\n'}
{'         '}<span className="c-com">…</span></code></pre>
          </div>
        </div>

        <div className="memory-points">
          <div className="memory-point">
            <div className="memory-point-num">01</div>
            <div>
              <strong>Written once, read by every agent.</strong>{" "}
              One memory entry helps Claude Code, Cursor, Codex, and whoever comes next — all from the same YAML.
            </div>
          </div>
          <div className="memory-point">
            <div className="memory-point-num">02</div>
            <div>
              <strong>Accumulates.</strong>{" "}
              Each agent's run is a chance to record something the next one shouldn't have to rediscover. Sightmap is the place those learnings land.
            </div>
          </div>
          <div className="memory-point">
            <div className="memory-point-num">03</div>
            <div>
              <strong>Reviewed like code.</strong>{" "}
              Memory entries are lines in a YAML file. They show up in diffs, land in PRs, and can be challenged, edited, or rolled back like anything else in your repo.
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
