/**
 * extension/side-panel.js
 *
 * Chrome side panel — live hover path + structured event log.
 */

// ── DOM refs ──────────────────────────────────────────────────────────────────

const statusDot = document.getElementById("status-dot");
const statusLine = document.getElementById("status-line");
const hoverTree = document.getElementById("hover-path-tree");
const log = document.getElementById("log");
const btnPause = document.getElementById("btn-pause");
const btnClear = document.getElementById("btn-clear");


// ── State ─────────────────────────────────────────────────────────────────────

let paused = false;
const MAX_EVENTS = 300;

// ── Background connection ─────────────────────────────────────────────────────

// The MV3 service worker may still be starting when the panel first opens,
// causing the port to disconnect immediately. Auto-reconnect with backoff.
let port = null;
let reconnectTimer = null;

function connectToBackground() {
  if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }

  port = chrome.runtime.connect({ name: "side-panel" });

  port.onMessage.addListener((msg) => {
    switch (msg.type) {
      case "sightmap-status":
        onStatus(msg);
        break;
      case "hover-path":
        onHover(msg);
        break;
      case "sightmap-event":
        if (!paused) onEvent(msg.payload);
        break;
    }
  });

  port.onDisconnect.addListener(() => {
    port = null;
    setStatus(false, "reconnecting…");
    reconnectTimer = setTimeout(connectToBackground, 800);
  });
}

connectToBackground();

// ── Status ────────────────────────────────────────────────────────────────────

function setStatus(connected, text) {
  statusDot.className = "status-dot" + (connected ? " on" : "");
  statusLine.textContent = text;
}

function onStatus(msg) {
  setStatus(true, `${msg.site} · ${msg.componentCount} components`);
}

// ── Shared: render a single component line ────────────────────────────────────
//
// Produces:  [ComponentName key="val" key2="val2"]
// With syntax-coloured spans. Used in both hover tree and event body.
//
// @param {import("./types.js").ComponentMatch} match
// @param {Object}  opts
// @param {boolean} opts.arrow   - prefix with "▸ " indent marker (hover tree)
// @param {number}  opts.depth   - indent depth (hover tree)
// @param {boolean} opts.showProps - whether to render properties inline

function renderCompLine(
  match,
  { arrow = false, depth = 0, showProps = true } = {},
) {
  const div = document.createElement("div");
  div.className = "comp-line";
  if (depth > 0) div.style.paddingLeft = depth * 10 + "px";

  if (arrow) {
    const arrowEl = document.createElement("span");
    arrowEl.className = "harrow";
    arrowEl.textContent = "▸ ";
    div.appendChild(arrowEl);
  }

  // Opening bracket
  const open = document.createElement("span");
  open.className = "cbr";
  open.textContent = "[";
  div.appendChild(open);

  // Component name
  const name = document.createElement("span");
  name.className = "cn";
  name.textContent = match.name;
  div.appendChild(name);

  // Properties inline: key="value" …
  if (showProps) {
    const props = Object.entries(match.properties ?? {});
    for (const [k, v] of props) {
      div.appendChild(document.createTextNode(" "));

      const keyEl = document.createElement("span");
      keyEl.className = "pk";
      keyEl.textContent = k + "=";
      div.appendChild(keyEl);

      const valEl = document.createElement("span");
      valEl.className = "pv";
      const display = v.length > 36 ? v.slice(0, 33) + "…" : v;
      valEl.textContent = `"${display}"`;
      valEl.title = v;
      div.appendChild(valEl);
    }
  }

  // Closing bracket
  const close = document.createElement("span");
  close.className = "cbr";
  close.textContent = "]";
  div.appendChild(close);

  return div;
}

// ── Hover tree ────────────────────────────────────────────────────────────────

function onHover(msg) {
  renderHoverTree(msg.path ?? []);
}

function renderHoverTree(path) {
  hoverTree.innerHTML = "";

  if (!path || !path.length) {
    const empty = document.createElement("div");
    empty.className = "hover-empty";
    empty.textContent = "—";
    hoverTree.appendChild(empty);
    return;
  }

  path.forEach((match, i) => {
    hoverTree.appendChild(
      renderCompLine(match, { arrow: i > 0, depth: i, showProps: true }),
    );
  });
}

// ── Event log ─────────────────────────────────────────────────────────────────

function onEvent(ev) {
  while (log.children.length >= MAX_EVENTS) log.removeChild(log.lastChild);

  const time = new Date(ev.timestamp).toTimeString().slice(0, 8);
  const path = ev.path ?? [];

  const card = document.createElement("div");
  card.className = "ev";

  // Copy — absolutely pinned top-right, out of document flow
  const copyBtn = document.createElement("button");
  copyBtn.className = "ev-copy";
  copyBtn.textContent = "⧉";
  copyBtn.title = ev.formatted;
  copyBtn.addEventListener("click", (e) => {
    e.stopPropagation();
    navigator.clipboard
      .writeText(ev.formatted)
      .then(() => {
        copyBtn.textContent = "✓";
        copyBtn.classList.add("copied");
        setTimeout(() => {
          copyBtn.textContent = "⧉";
          copyBtn.classList.remove("copied");
        }, 1200);
      })
      .catch(() => {});
  });
  card.appendChild(copyBtn);

  // Meta: time · type
  const meta = document.createElement("div");
  meta.className = "ev-meta";
  const timeEl = document.createElement("span");
  timeEl.className = "ev-time";
  timeEl.textContent = time;
  const dot = document.createElement("span");
  dot.className = "ev-dot";
  dot.textContent = " · ";
  const typeEl = document.createElement("span");
  typeEl.className = `ev-type ${ev.type}`;
  typeEl.textContent = ev.type;
  meta.appendChild(timeEl);
  meta.appendChild(dot);
  meta.appendChild(typeEl);
  card.appendChild(meta);

  // Body: one comp-line per component, props inline
  if (!path.length) {
    const orphan = document.createElement("div");
    orphan.className = "comp-line comp-orphan";
    orphan.textContent = "(orphaned)";
    card.appendChild(orphan);
  } else {
    path.forEach((match) =>
      card.appendChild(renderCompLine(match, { showProps: true })),
    );
  }

  log.insertBefore(card, log.firstChild);
}

// ── Controls ──────────────────────────────────────────────────────────────────

btnClear.addEventListener("click", () => {
  log.innerHTML = "";
});

btnPause.addEventListener("click", () => {
  paused = !paused;
  btnPause.textContent = paused ? "▶" : "⏸";
  btnPause.title = paused ? "Resume" : "Pause";
  btnPause.classList.toggle("active", paused);
});

