export default function CTASection() {
  return (
    <section id="start" className="get-started" data-component="GetStarted">
      <div className="container container--wide">
        <div className="section-label">Get Started</div>
        <h2>Hand your agent the map.</h2>
        <p className="section-desc">
          Install the CLI, start a browser session, and have your agent map the running app into <code>.sightmap/</code>. Review and commit the YAML so every agent using Sightmap-enabled tooling starts with the same app context.
        </p>

        <div className="gs-steps">
          {/* -------------------- STEP 01 -------------------- */}
          <div className="gs-step">
            <span className="gs-step-num">01</span>
            <div className="gs-step-body">
              <h3>Install the CLI and skills</h3>
              <p className="gs-step-intro">
                The <code>sightmap</code> CLI ships through npm as a prebuilt native binary, so you do not need Go. <code>sightmap skills install</code> adds the agent playbooks to your harness. <code>.sightmap/</code> is plain YAML, with no framework adapter or build step.
              </p>
              <div className="code-block">
                <div className="code-header">
                  <span className="code-filename">your project root</span>
                  <span className="code-lang">shell</span>
                </div>
                <pre><code><span className="c-com">$</span> npm install -g <span className="c-str">@sightmap/sightmap</span>{'\n'}
<span className="c-com">$</span> sightmap skills install{'\n'}
installed 2 sightmap skill(s) → ~/.agents/skills{'\n'}
{'  '}<span className="c-key">sightmap-authoring</span>{'\n'}
{'  '}<span className="c-key">sightmap-browser</span></code></pre>
              </div>
              <p className="gs-followup">
                To build from source, run <code>go install github.com/sightmap/sightmap/go/cmd/sightmap@latest</code>. You can also import the module as a library with <code>go get</code>.
              </p>
            </div>
          </div>

          {/* -------------------- STEP 02 -------------------- */}
          <div className="gs-step">
            <span className="gs-step-num">02</span>
            <div className="gs-step-body">
              <h3>Start a session and iterate</h3>
              <p className="gs-step-intro">
                <code>browser start</code> launches Chrome and a corpus server that reloads YAML changes. <code>iterate</code> snapshots a page and scores each interactive node: named (<strong>T1</strong>), inside a named component (<strong>T2</strong>), or orphaned (<strong>T3</strong>). Repeat until there are no orphans.
              </p>
              <div className="code-block">
                <div className="code-header">
                  <span className="code-filename">your project root</span>
                  <span className="code-lang">shell</span>
                </div>
                <pre><code><span className="c-com">$</span> sightmap browser start{'\n'}
<span className="c-val">●</span> ready  port=7891  cdp=7892  pid=52441  tab=A1B2C3D4{'\n'}
<span className="c-com">$</span> sightmap iterate <span className="c-str">'http://localhost:3000/'</span>{'\n'}
<span className="c-accent">[View: Home <span className="c-str">"http://localhost:3000/"</span>]</span>{'\n'}
<span className="c-accent">[Coverage]</span> (visible only){'\n'}
87 interactive · 61 direct T1 (70%) · 21 scoped T2 (24%) · <span className="c-val">5 orphaned T3 ✗</span></code></pre>
              </div>
              <p className="gs-followup">
                Run <code>sightmap sel-probe '[data-testid="product-pod"]'</code> to verify a selector before adding it.
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
                Point your agent at the app. The bundled <code>sightmap-authoring</code> skill walks routes, names orphaned components, and writes <code>.sightmap/</code>. Review the diff before committing it. The skill checks each definition against the running app before adding it.
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
                Run <code>sightmap validate</code> and <code>sightmap lint</code> in CI. When the app changes, rerun <code>iterate</code> on the affected pages.
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
