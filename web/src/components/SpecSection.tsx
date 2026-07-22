export default function SpecSection() {
  return (
    <section id="spec" data-component="SpecSection">
      <div className="container container--wide">
        <div className="section-label">The Spec</div>
        <h2>Views, components, requests.</h2>
        <p className="section-desc">
          A sightmap has three kinds of definitions. <strong>Views</strong> name the screens your app renders. <strong>Components</strong> name the parts of each screen. <strong>Requests</strong> name the API routes behind them. Define them in YAML under <code>.sightmap/</code> and agents pick them up automatically.
        </p>

        {/* -------------------- VIEWS -------------------- */}
        <div className="concept">
          <div className="concept-header">
            <span className="concept-num">01</span>
            <h3>Views</h3>
          </div>
          <p className="concept-intro">
            A view is a screen or route. When an agent lands on a matching URL, its snapshot gets a header — <em>"you are here"</em>.
          </p>

          <div className="concept-grid">
            <div className="code-block">
              <div className="code-header">
                <span className="code-filename">.sightmap/views.yaml</span>
                <span className="code-lang">yaml</span>
              </div>
              <pre><code><span className="c-key">version</span>: <span className="c-val">1</span>{'\n'}
{'\n'}
<span className="c-key">views</span>:{'\n'}
{'  '}- <span className="c-key">name</span>: <span className="c-str">FlightSearch</span>{'\n'}
{'    '}<span className="c-key">route</span>: <span className="c-str">/search</span>{'\n'}
{'    '}<span className="c-key">description</span>: <span className="c-str">Primary booking flow</span>{'\n'}
{'    '}<span className="c-key">source</span>: <span className="c-str">src/pages/FlightSearch.tsx</span>{'\n'}
{'\n'}
{'  '}- <span className="c-key">name</span>: <span className="c-str">BookingConfirmation</span>{'\n'}
{'    '}<span className="c-key">route</span>: <span className="c-str">/bookings/*</span>{'\n'}
{'    '}<span className="c-key">source</span>: <span className="c-str">src/pages/Booking.tsx</span></code></pre>
            </div>

            <div className="code-block">
              <div className="code-header">
                <span className="code-filename">what the agent sees</span>
                <span className="code-lang">snapshot</span>
              </div>
              <pre><code><span className="c-accent">[View: FlightSearch <span className="c-str">"/search"</span>]</span>{'\n'}
{'\n'}
<span className="c-com">uid=1_0</span> RootWebArea <span className="c-str">"Book a flight"</span>{'\n'}
<span className="c-com">uid=1_1</span>   main <span className="c-val">visible</span>{'\n'}
<span className="c-com">uid=1_2</span>     heading <span className="c-str">"Find your flight"</span>{'\n'}
{'         '}<span className="c-com">…</span>{'\n'}
{'\n'}
<span className="c-com">// on /bookings/42</span>{'\n'}
<span className="c-accent">[View: BookingConfirmation <span className="c-str">"/bookings/42"</span>]</span></code></pre>
            </div>
          </div>

          <div className="schema-table-wrap">
            <table className="schema-table">
              <thead>
                <tr><th>Field</th><th>Type</th><th>Description</th></tr>
              </thead>
              <tbody>
                <tr>
                  <td>name <span className="required-badge">required</span></td>
                  <td>string</td>
                  <td>Shown in the snapshot header</td>
                </tr>
                <tr>
                  <td>route <span className="required-badge">required</span></td>
                  <td>string</td>
                  <td>Glob pattern. <code>*</code> matches one segment, <code>**</code> any depth. Most specific match wins; declaration order tiebreaks.</td>
                </tr>
                <tr>
                  <td>description</td>
                  <td>string</td>
                  <td>Optional, not uploaded — useful for humans reading the file</td>
                </tr>
                <tr>
                  <td>source</td>
                  <td>string</td>
                  <td>Relative path to the source file</td>
                </tr>
                <tr>
                  <td>dependencies</td>
                  <td>string[]</td>
                  <td>Optional globs naming secondary files (hooks, stores, styles) whose changes should also trigger re-curation. Reverse lookup walks these too.</td>
                </tr>
                <tr>
                  <td>components</td>
                  <td>Component[]</td>
                  <td>View-scoped components, merged additively with globals. Accepts <code>{'{ $ref: Name }'}</code> to reference a globally-defined component.</td>
                </tr>
                <tr>
                  <td>requests</td>
                  <td>Request[]</td>
                  <td>View-scoped requests, merged additively with globals</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        {/* -------------------- COMPONENTS -------------------- */}
        <div className="concept">
          <div className="concept-header">
            <span className="concept-num">02</span>
            <h3>Components</h3>
          </div>
          <p className="concept-intro">
            A component maps a CSS selector to a semantic name. Agents see your names — <code>DepartureDatePicker</code>, not <code>div.react-datepicker__day</code> — in every snapshot. The <code>memory</code> field is where the running app's quirks get written down.
          </p>

          <div className="concept-grid">
            <div className="code-block">
              <div className="code-header">
                <span className="code-filename">.sightmap/pages/flights.yaml</span>
                <span className="code-lang">yaml</span>
              </div>
              <pre><code><span className="c-key">components</span>:{'\n'}
{'  '}- <span className="c-key">name</span>: <span className="c-str">DepartureDatePicker</span>{'\n'}
{'    '}<span className="c-key">selector</span>: <span className="c-str">'[data-picker="departure"]'</span>{'\n'}
{'    '}<span className="c-key">source</span>: <span className="c-str">src/components/DatePicker.tsx</span>{'\n'}
{'    '}<span className="c-key">dependencies</span>:{'\n'}
{'      '}- <span className="c-str">src/components/DatePicker.module.css</span>{'\n'}
{'      '}- <span className="c-str">src/hooks/useDateRange.ts</span>{'\n'}
{'    '}<span className="c-key">memory</span>:{'\n'}
{'      '}- <span className="c-str">Accepts typed YYYY-MM-DD — skips the calendar</span>{'\n'}
{'      '}- <span className="c-str">Arrow keys navigate; Enter selects; Esc closes</span>{'\n'}
{'      '}- <span className="c-str">Past dates render but are aria-disabled</span>{'\n'}
{'      '}- <span className="c-str">Range: 1st click = start, 2nd = end, 3rd resets</span>{'\n'}
{'    '}<span className="c-key">children</span>:{'\n'}
{'      '}- <span className="c-key">name</span>: <span className="c-str">date-input</span>{'\n'}
{'        '}<span className="c-key">selector</span>: <span className="c-str">input</span>{'\n'}
{'      '}- <span className="c-key">name</span>: <span className="c-str">day-grid</span>{'\n'}
{'        '}<span className="c-key">selector</span>: <span className="c-str">'[role="grid"]'</span>{'\n'}
{'      '}- <span className="c-key">name</span>: <span className="c-str">day</span>{'\n'}
{'        '}<span className="c-key">selector</span>: <span className="c-str">'[role="gridcell"]'</span></code></pre>
            </div>

            <div className="code-block">
              <div className="code-header">
                <span className="code-filename">what the agent sees</span>
                <span className="code-lang">snapshot</span>
              </div>
              <pre><code><span className="c-com">uid=1_3</span> <span className="c-key">DepartureDatePicker</span> <span className="c-com">[src: src/components/DatePicker.tsx]</span>{'\n'}
{'         '}<span className="c-val">visible interactive</span>{'\n'}
<span className="c-com">uid=1_4</span>   <span className="c-key">date-input</span> <span className="c-val">visible interactive</span>{'\n'}
<span className="c-com">uid=1_11</span>  <span className="c-key">day-grid</span> <span className="c-val">visible</span>{'\n'}
<span className="c-com">uid=1_19</span>    <span className="c-key">day</span> <span className="c-str">"Choose Tuesday, July 1st, 2025"</span>{'\n'}
{'           '}<span className="c-val">visible interactive</span>{'\n'}
<span className="c-com">uid=1_20</span>    <span className="c-key">day</span> <span className="c-str">"Choose Wednesday, July 2nd, 2025"</span>{'\n'}
{'           '}<span className="c-val">visible interactive</span>{'\n'}
{'             '}<span className="c-com">…</span></code></pre>
            </div>
          </div>

          <div className="callout">
            <strong>Selectors can be a list.</strong> When multiple CSS patterns should match the same component — say, both a class name and an ARIA role — pass a YAML array instead of a string.
          </div>

          <div className="callout">
            <strong>Reference shared components by name.</strong> Inside any <code>components:</code> array, an entry of the form <code>{'{ $ref: SiteHeader }'}</code> inlines a deep copy of a component defined elsewhere. Define a header, footer, or chat widget once, and every view that contains it attests to its presence — drift checks treat <em>attested but missing</em> as a distinct signal.
          </div>

          <div className="schema-table-wrap">
            <table className="schema-table">
              <thead>
                <tr><th>Field</th><th>Type</th><th>Description</th></tr>
              </thead>
              <tbody>
                <tr>
                  <td>name <span className="required-badge">required</span></td>
                  <td>string</td>
                  <td>Replaces the generic a11y role in snapshots</td>
                </tr>
                <tr>
                  <td>selector <span className="required-badge">required</span></td>
                  <td>string | string[]</td>
                  <td>CSS selector, or a list of alternatives</td>
                </tr>
                <tr>
                  <td>source</td>
                  <td>string</td>
                  <td>Path to the source file — rendered inline as <code>[src: …]</code></td>
                </tr>
                <tr>
                  <td>dependencies</td>
                  <td>string[]</td>
                  <td>Optional globs naming supplementary files (styles, helpers) that influence this component. Used by reverse-lookup and change-propagation tools; not uploaded.</td>
                </tr>
                <tr>
                  <td>description</td>
                  <td>string</td>
                  <td>Optional, not uploaded</td>
                </tr>
                <tr>
                  <td>memory</td>
                  <td>string[]</td>
                  <td>Notes that appear in the <code>[Guide]</code> section of every matched snapshot</td>
                </tr>
                <tr>
                  <td>children</td>
                  <td>Component[]</td>
                  <td>Nested components; their selectors are scoped to the parent's subtree</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        {/* -------------------- REQUESTS -------------------- */}
        <div className="concept">
          <div className="concept-header">
            <span className="concept-num">03</span>
            <h3>Requests</h3>
            <span className="concept-badge">new</span>
          </div>
          <p className="concept-intro">
            Requests map API endpoints to semantic names with optional field schemas. The network tab gets the same treatment as the DOM — agents see the names you wrote, not a wall of URLs.
          </p>

          <div className="concept-grid">
            <div className="code-block">
              <div className="code-header">
                <span className="code-filename">.sightmap/requests.yaml</span>
                <span className="code-lang">yaml</span>
              </div>
              <pre><code><span className="c-key">requests</span>:{'\n'}
{'  '}- <span className="c-key">name</span>: <span className="c-str">SearchFlights</span>{'\n'}
{'    '}<span className="c-key">route</span>: <span className="c-str">/api/flights/search</span>{'\n'}
{'    '}<span className="c-key">method</span>: <span className="c-val">POST</span>{'\n'}
{'    '}<span className="c-key">description</span>: <span className="c-str">Search for available flights</span>{'\n'}
{'    '}<span className="c-key">source</span>: <span className="c-str">src/api/flights.ts</span>{'\n'}
{'    '}<span className="c-key">request</span>:{'\n'}
{'      '}<span className="c-key">fields</span>:{'\n'}
{'        '}- <span className="c-key">name</span>: <span className="c-str">origin</span>{'\n'}
{'          '}<span className="c-key">type</span>: <span className="c-val">string</span>{'\n'}
{'          '}<span className="c-key">description</span>: <span className="c-str">Origin airport code</span>{'\n'}
{'        '}- <span className="c-key">name</span>: <span className="c-str">destination</span>{'\n'}
{'          '}<span className="c-key">type</span>: <span className="c-val">string</span>{'\n'}
{'        '}- <span className="c-key">name</span>: <span className="c-str">date</span>{'\n'}
{'          '}<span className="c-key">type</span>: <span className="c-val">string</span>{'\n'}
{'          '}<span className="c-key">description</span>: <span className="c-str">YYYY-MM-DD</span>{'\n'}
{'    '}<span className="c-key">response</span>:{'\n'}
{'      '}<span className="c-key">fields</span>:{'\n'}
{'        '}- <span className="c-key">name</span>: <span className="c-str">flights</span>{'\n'}
{'          '}<span className="c-key">type</span>: <span className="c-val">array</span>{'\n'}
{'        '}- <span className="c-key">name</span>: <span className="c-str">total</span>{'\n'}
{'          '}<span className="c-key">type</span>: <span className="c-val">number</span></code></pre>
            </div>

            <div className="code-block">
              <div className="code-header">
                <span className="code-filename">what the agent sees</span>
                <span className="code-lang">network</span>
              </div>
              <pre><code><span className="c-com"># network list</span>{'\n'}
<span className="c-com">reqid=1</span> <span className="c-key">SearchFlights</span> POST /api/flights/search <span className="c-val">[200]</span>{'\n'}
<span className="c-com">reqid=2</span> GET /assets/logo.png <span className="c-val">[200]</span>{'\n'}
{'\n'}
<span className="c-com"># network detail → reqid=1</span>{'\n'}
<span className="c-accent">### Sightmap: SearchFlights</span>{'\n'}
Search for available flights{'\n'}
Source: src/api/flights.ts{'\n'}
Expected request fields:{'\n'}
<span className="c-val">-</span> origin (string) — Origin airport code{'\n'}
<span className="c-val">-</span> destination (string){'\n'}
<span className="c-val">-</span> date (string) — YYYY-MM-DD{'\n'}
Expected response fields:{'\n'}
<span className="c-val">-</span> flights (array){'\n'}
<span className="c-val">-</span> total (number)</code></pre>
            </div>
          </div>

          <div className="schema-table-wrap">
            <table className="schema-table">
              <thead>
                <tr><th>Field</th><th>Type</th><th>Description</th></tr>
              </thead>
              <tbody>
                <tr>
                  <td>name <span className="required-badge">required</span></td>
                  <td>string</td>
                  <td>Shown in network list and detail output</td>
                </tr>
                <tr>
                  <td>route <span className="required-badge">required</span></td>
                  <td>string</td>
                  <td>Glob pattern. Express-style <code>:param</code> segments convert to <code>*</code>.</td>
                </tr>
                <tr>
                  <td>method</td>
                  <td>string</td>
                  <td>Optional HTTP method filter (<code>GET</code>, <code>POST</code>, …)</td>
                </tr>
                <tr>
                  <td>description</td>
                  <td>string</td>
                  <td>What the endpoint does</td>
                </tr>
                <tr>
                  <td>source</td>
                  <td>string</td>
                  <td>Path to the source file</td>
                </tr>
                <tr>
                  <td>request</td>
                  <td>object</td>
                  <td>Expected payload: <code>fields[]</code> (name, type, description)</td>
                </tr>
                <tr>
                  <td>response</td>
                  <td>object</td>
                  <td>Expected response: same shape as <code>request</code></td>
                </tr>
                <tr>
                  <td>headers</td>
                  <td>string[]</td>
                  <td>Notable header names to highlight in the detail view</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        {/* -------------------- DIRECTORY CALLOUT -------------------- */}
        <div className="concept concept--last">
          <div className="concept-header">
            <h3 className="concept-closer">Organize however.</h3>
          </div>
          <p className="concept-intro">
            All <code>*.yaml</code> and <code>*.yml</code> files under <code>.sightmap/</code> are discovered recursively and merged. Structure the directory however makes sense — one file per feature, one per view, or one big file.
          </p>

          <div className="code-block">
            <div className="code-header">
              <span className="code-filename">your-project/</span>
              <span className="code-lang">tree</span>
            </div>
            <pre><code>.sightmap/{'\n'}
├── <span className="c-str">components.yaml</span>        <span className="c-com"># global NavBar, Footer</span>{'\n'}
├── <span className="c-str">requests.yaml</span>          <span className="c-com"># API endpoints</span>{'\n'}
└── <span className="c-str">pages/</span>{'\n'}
{'    '}├── <span className="c-str">flights.yaml</span>       <span className="c-com"># FlightSearch view + its components</span>{'\n'}
{'    '}└── <span className="c-str">booking.yaml</span>       <span className="c-com"># BookingConfirmation view</span></code></pre>
          </div>
        </div>
      </div>
    </section>
  )
}
