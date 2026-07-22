export default function CTASection() {
  return (
    <section id="start" className="get-started" data-component="GetStarted">
      <div className="container container--wide">
        <div className="section-label">Get Started</div>
        <h2>Hand your agent the map.</h2>
        <p className="section-desc">
          Install the CLI, start a browser session, and let your agent map the running app into <code>.sightmap/</code>. One YAML directory, learned from what the app actually does, shared across every agent. Three steps below.
        </p>

        <div className="gs-steps">
          {/* -------------------- STEP 01 -------------------- */}
          <div className="gs-step">
            <span className="gs-step-num">01</span>
            <div className="gs-step-body">
              <h3>Install the CLI and skills</h3>
              <p className="gs-step-intro">
                The <code>sightmap</code> CLI ships as a prebuilt native binary via npm — no Go toolchain required — or build it from source with <code>go install</code>. <code>skills install</code> drops the agent playbooks into your harness. There is no framework adapter and no build step; <code>.sightmap/</code> is plain YAML.
              </p>
              <div className="code-block">
                <div className="code-header">
                  <span className="code-filename">your project root</span>
                  <span className="code-lang">shell</span>
                </div>
                <pre><code><span className="c-com">$</span> npm install -g <span className="c-str">@sightmap/sightmap</span>{'\n'}
<span className="c-com">$</span> sightmap skills install{'\n'}
<span className="c-val">✓</span> installed <span className="c-key">sightmap-authoring</span> + <span className="c-key">sightmap-browser</span></code></pre>
              </div>
              <p className="gs-followup">
                Prefer source? <code>go install github.com/sightmap/sightmap/go/cmd/sightmap@latest</code>. The same module is <code>go get</code>-able as a library.
              </p>
            </div>
          </div>

          {/* -------------------- STEP 02 -------------------- */}
          <div className="gs-step">
            <span className="gs-step-num">02</span>
            <div className="gs-step-body">
              <h3>Start a session and iterate</h3>
              <p className="gs-step-intro">
                <code>browser start</code> launches Chrome and a corpus server that hot-reloads your YAML. <code>iterate</code> snaps a page and scores coverage — every interactive node is named (<strong>T1</strong>), inside a named component (<strong>T2</strong>), or orphaned (<strong>T3</strong>). You drive toward zero orphans.
              </p>
              <div className="code-block">
                <div className="code-header">
                  <span className="code-filename">your project root</span>
                  <span className="code-lang">shell</span>
                </div>
                <pre><code><span className="c-com">$</span> sightmap browser start{'\n'}
<span className="c-val">●</span> running{'\n'}
<span className="c-com">$</span> sightmap iterate <span className="c-str">'http://localhost:3000/'</span>{'\n'}
<span className="c-accent">[View: Home]</span>{'\n'}
87 interactive · 61 named · 21 scoped · <span className="c-val">5 orphaned</span></code></pre>
              </div>
              <p className="gs-followup">
                Verify a candidate before you write it: <code>sightmap sel-probe '[data-testid="product-pod"]'</code>.
              </p>
            </div>
          </div>

          {/* -------------------- STEP 03 -------------------- */}
          <div className="gs-step">
            <span className="gs-step-num">03</span>
            <div className="gs-step-body">
              <h3>
                Let the agent curate <span className="gs-step-badge">agent-driven</span>
              </h3>
              <p className="gs-step-intro">
                Point your agent at the app. The bundled <code>sightmap-authoring</code> skill walks the routes, names the orphaned components, and writes <code>.sightmap/</code> — you review the diff. The corpus is agent-curated, not generated; nothing writes it from source.
              </p>
              <div className="code-block">
                <div className="code-header">
                  <span className="code-filename">to your agent</span>
                  <span className="code-lang">prompt</span>
                </div>
                <pre><code><span className="c-com">&gt;</span> Bootstrap a sightmap for this app: start a{'\n'}
<span className="c-com">&gt;</span> session, iterate the main routes, and name{'\n'}
<span className="c-com">&gt;</span> components until coverage is clean.</code></pre>
              </div>
              <p className="gs-followup">
                Keep it honest in CI with <code>sightmap validate</code> and <code>sightmap lint</code>. Reconcile as the app changes by re-running <code>iterate</code> on the affected pages.
              </p>
            </div>
          </div>
        </div>

        <div className="gs-footer">
          <div className="gs-ctas">
            <a href="https://github.com/sightmap/sightmap" target="_blank" rel="noreferrer" className="btn-primary">
              View on GitHub
            </a>
            <a href="https://www.npmjs.com/package/@sightmap/sightmap" target="_blank" rel="noreferrer" className="btn-secondary">
              <code>npm i -g @sightmap/sightmap</code>
            </a>
            <a href="https://docs.sightmap.org" target="_blank" rel="noreferrer" className="btn-secondary">
              Read the docs →
            </a>
          </div>
        </div>
      </div>
    </section>
  )
}
