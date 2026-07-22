/**
 * extension/panel.js
 *
 * DevTools panel — event log + live hover path display.
 * Connects to background.js via chrome.runtime.connect to receive:
 *   - sightmap-status: connection status updates
 *   - sightmap-event: user interaction events
 *   - hover-path: current hover component path (live)
 */

// ── Elements ──────────────────────────────────────────────────────────────────

const statusBadge = document.getElementById("status-badge");
const hoverPathEl = document.getElementById("hover-path");
const logSection = document.getElementById("log-section");
const btnClear = document.getElementById("btn-clear");
const btnPause = document.getElementById("btn-pause");

// ── State ─────────────────────────────────────────────────────────────────────

let paused = false;
const MAX_EVENTS = 200;

// ── Background connection ─────────────────────────────────────────────────────

const port = chrome.runtime.connect({ name: "devtools-panel" });

port.onMessage.addListener((msg) => {
  switch (msg.type) {
    case "sightmap-status":
      onSightmapStatus(msg);
      break;
    case "sightmap-event":
      if (!paused) addEvent(msg.payload);
      break;
    case "hover-path":
      updateHoverPath(msg.formatted || "");
      break;
  }
});

port.onDisconnect.addListener(() => {
  setStatus("disconnected", false);
});

// ── Status ────────────────────────────────────────────────────────────────────

function setStatus(text, connected) {
  statusBadge.textContent = text;
  statusBadge.className = connected ? "connected" : "";
}

function onSightmapStatus(msg) {
  setStatus(`${msg.site} · ${msg.componentCount} components`, true);
}

// ── Hover path ────────────────────────────────────────────────────────────────

function updateHoverPath(formatted) {
  hoverPathEl.textContent = formatted;
}

// ── Event log ─────────────────────────────────────────────────────────────────

/**
 * Render a formatted event path string with syntax highlighting.
 * "[ComponentName prop="val"]" → spans with .comp and .prop classes.
 * @param {string} formatted
 * @returns {DocumentFragment}
 */
function renderFormattedPath(formatted) {
  const frag = document.createDocumentFragment();

  if (!formatted || formatted.includes("(orphaned)")) {
    const span = document.createElement("span");
    span.className = "orphan";
    span.textContent = formatted || "(unresolved)";
    frag.appendChild(span);
    return frag;
  }

  // Tokenise: split on " > " to get path segments, then parse each segment
  const eventMatch = formatted.match(/^(\w+)\s+(.*)/s);
  if (!eventMatch) {
    frag.appendChild(document.createTextNode(formatted));
    return frag;
  }

  const [, eventType, rest] = eventMatch;

  // Event type prefix
  const typeSp = document.createElement("span");
  typeSp.style.color = "#888";
  typeSp.textContent = `${eventType} `;
  frag.appendChild(typeSp);

  // Parse "[Name prop="val" ...] > [Name] > ..."
  const segRe = /\[([^\]]+)\]/g;
  let lastIdx = 0;
  let match;
  const segments = [];
  while ((match = segRe.exec(rest)) !== null) {
    segments.push(match[1]);
  }

  segments.forEach((seg, i) => {
    if (i > 0) {
      const arrow = document.createElement("span");
      arrow.style.color = "#555";
      arrow.textContent = " > ";
      frag.appendChild(arrow);
    }
    frag.appendChild(document.createTextNode("["));

    // Split "Name prop=val prop2=val2" into name + props
    const firstSpace = seg.indexOf(" ");
    const name = firstSpace === -1 ? seg : seg.slice(0, firstSpace);
    const props = firstSpace === -1 ? "" : seg.slice(firstSpace + 1);

    const nameSp = document.createElement("span");
    nameSp.className = "comp";
    nameSp.textContent = name;
    frag.appendChild(nameSp);

    if (props) {
      const propSp = document.createElement("span");
      propSp.className = "prop";
      propSp.textContent = " " + props;
      frag.appendChild(propSp);
    }

    frag.appendChild(document.createTextNode("]"));
  });

  return frag;
}

/**
 * @param {import("./types.js").SightmapEvent} ev
 */
function addEvent(ev) {
  // Trim log if it's getting long
  while (logSection.children.length >= MAX_EVENTS) {
    logSection.removeChild(logSection.lastChild);
  }

  const ts = new Date(ev.timestamp);
  const timeStr = ts.toTimeString().slice(0, 8);

  const row = document.createElement("div");
  row.className = "event-row";

  // Timestamp
  const timeEl = document.createElement("div");
  timeEl.className = "event-time";
  timeEl.textContent = timeStr;

  // Type badge
  const typeEl = document.createElement("div");
  typeEl.className = `event-type ${ev.type}`;
  typeEl.textContent = ev.type;

  // Formatted path
  const pathEl = document.createElement("div");
  pathEl.className = "event-path";
  pathEl.appendChild(renderFormattedPath(ev.formatted));

  // Expandable detail: full JSON
  const detailEl = document.createElement("div");
  detailEl.className = "event-detail";
  detailEl.textContent = JSON.stringify(
    {
      url: ev.url,
      element: ev.element,
      path: ev.path?.map((m) => ({
        name: m.name,
        properties: m.properties,
      })),
    },
    null,
    2,
  );

  row.appendChild(timeEl);
  row.appendChild(typeEl);
  row.appendChild(pathEl);
  row.appendChild(detailEl);

  // Click row to expand/collapse detail
  row.addEventListener("click", () => {
    row.classList.toggle("expanded");
  });

  // Prepend so newest events appear at top
  logSection.insertBefore(row, logSection.firstChild);
}

// ── Controls ──────────────────────────────────────────────────────────────────

btnClear.addEventListener("click", () => {
  logSection.innerHTML = "";
});

btnPause.addEventListener("click", () => {
  paused = !paused;
  btnPause.textContent = paused ? "Resume" : "Pause";
  btnPause.style.color = paused ? "#ffb74d" : "";
});

// ── Initial state ─────────────────────────────────────────────────────────────

setStatus("waiting for sightmap server…", false);
