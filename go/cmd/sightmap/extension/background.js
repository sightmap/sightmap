/**
 * extension/background.js
 *
 * Service worker — message broker + side panel lifecycle.
 *
 * Side panel open/close governs overlay visibility: the content script
 * only draws bounding boxes when at least one side panel is connected.
 *
 * Port names:
 *   "devtools-panel" — DevTools panel (panel.html)
 *   "side-panel"     — Chrome side panel (side-panel.html)
 */

// ── Panel port registry ───────────────────────────────────────────────────────

/** @type {Set<chrome.runtime.Port>} */
const devtoolsPorts = new Set();
/** @type {Set<chrome.runtime.Port>} */
const sidePanelPorts = new Set();

/** Last known sightmap status — replayed to panels that connect after load. */
let lastSightmapStatus = null;

function broadcastToDevtools(msg) {
  for (const p of devtoolsPorts) {
    try {
      p.postMessage(msg);
    } catch {
      devtoolsPorts.delete(p);
    }
  }
}
function broadcastToSidePanel(msg) {
  for (const p of sidePanelPorts) {
    try {
      p.postMessage(msg);
    } catch {
      sidePanelPorts.delete(p);
    }
  }
}
function broadcastToAll(msg) {
  broadcastToDevtools(msg);
  broadcastToSidePanel(msg);
}

/** Push overlay visibility to every content script in every tab. */
function pushOverlayVisible(visible) {
  const msg = { type: "overlay-visible", value: visible };
  chrome.tabs.query({}, (tabs) => {
    for (const tab of tabs) {
      if (tab.id != null) {
        chrome.tabs.sendMessage(tab.id, msg).catch(() => {});
      }
    }
  });
}

/** Build a sightmap-loaded status object from a /sightmap response. */
function statusFromSightmap(data) {
  const componentCount =
    (data.globals?.length ?? 0) +
    Object.values(data.viewComponents ?? {}).reduce((s, a) => s + a.length, 0);
  return {
    type: "sightmap-loaded",
    version: data.version,
    site: data.site,
    componentCount,
  };
}

chrome.runtime.onConnect.addListener((port) => {
  const isSide = port.name === "side-panel";
  const isDev = port.name === "devtools-panel";
  if (!isSide && !isDev) return;

  const set = isSide ? sidePanelPorts : devtoolsPorts;
  set.add(port);

  if (isSide && sidePanelPorts.size === 1) {
    // First side panel opened — activate overlay
    pushOverlayVisible(true);
  }

  // Replay last sightmap status so panels opening after initial load see the
  // correct state immediately rather than staying on "waiting for server…".
  // If lastSightmapStatus is null the service worker restarted and lost it —
  // do a fresh fetch so the panel doesn't stay on "waiting for server…".
  if (lastSightmapStatus) {
    port.postMessage({ ...lastSightmapStatus, type: "sightmap-status" });
  } else {
    getServer()
      .then((srv) => fetch(`${srv}/sightmap`))
      .then((r) => r.json())
      .then((data) => {
        const status = statusFromSightmap(data);
        lastSightmapStatus = status;
        port.postMessage({ ...status, type: "sightmap-status" });
        setBadge("✓", "#4CAF50");
      })
      .catch(() => {});
  }

  port.onDisconnect.addListener(() => {
    set.delete(port);
    if (isSide && sidePanelPorts.size === 0) {
      // Last side panel closed — deactivate overlay
      pushOverlayVisible(false);
    }
  });
});

// ── Action: open side panel on click ─────────────────────────────────────────

chrome.sidePanel
  .setPanelBehavior({ openPanelOnActionClick: true })
  .catch(() => {});

// NOTE: chrome.sidePanel.open() is intentionally NOT called here.
// openPanelOnActionClick:true (set above) is the sole mechanism for opening
// the panel on action-icon click.  Calling open() a second time caused Chrome
// to navigate the panel away from side-panel.html to the active tab's URL.
// If a future code-path needs to open the panel programmatically, call
//   chrome.sidePanel.open({ tabId }) inside a chrome.action.onClicked listener
// and remove setPanelBehavior({ openPanelOnActionClick: true }) to avoid
// the double-open race.

// ── Badge helpers ─────────────────────────────────────────────────────────────

function setBadge(text, color) {
  chrome.action.setBadgeText({ text });
  chrome.action.setBadgeBackgroundColor({ color });
}

// ── Fetch proxy (content script can't fetch http:// from https:// page) ──────
// Discover which port the sightmap HTTP server is running on.
// Strategy:
//   1. Read chrome.storage.local.serverPort (injected by `browser start` via CDP).
//      chrome.storage.local is profile-scoped, so sessions on different profiles
//      can't cross-talk even when running concurrently.
//   2. Verify that port actually responds. If not, probe 7891–7900.
//   3. Cache the result for this service-worker lifetime.

async function pingPort(port) {
  try {
    const resp = await fetch(`http://localhost:${port}/sightmap/version`, {
      signal: AbortSignal.timeout(400),
    });
    return resp.ok;
  } catch {
    return false;
  }
}

async function discoverServer() {
  // Try hint from chrome.storage.local first (injected by `sightmap browser start`
  // via CDP after Chrome launches — profile-scoped so sessions can't cross-talk).
  let hintPort = 7891;
  try {
    const stored = await chrome.storage.local.get("serverPort");
    if (stored.serverPort) hintPort = stored.serverPort;
  } catch {
    /* storage unavailable — use default hint */
  }

  if (await pingPort(hintPort)) return `http://localhost:${hintPort}`;

  // Hint port not responding — probe the range as fallback.
  for (let port = 7891; port <= 7900; port++) {
    if (port !== hintPort && (await pingPort(port))) {
      return `http://localhost:${port}`;
    }
  }
  // Nothing found — return hint anyway and let fetches fail visibly.
  return `http://localhost:${hintPort}`;
}

// Lazy server promise — resolved on first use, not at module load time.
// IMPORTANT: do NOT top-level await here. Chrome registers event listeners
// only after top-level awaits resolve, so any messages that arrive during
// discoverServer() (e.g. content script's query-overlay-visible on page
// load) would be lost. Keeping this lazy means listeners register
// synchronously at startup and server discovery happens on first fetch.
let _serverPromise = null;
function getServer() {
  if (!_serverPromise) _serverPromise = discoverServer();
  return _serverPromise;
}

// ── Message handling ──────────────────────────────────────────────────────────

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  switch (msg.type) {
    case "ping":
      sendResponse({
        type: "pong",
        version: chrome.runtime.getManifest().version,
      });
      return false;

    case "fetch-sightmap":
      getServer()
        .then((srv) => fetch(`${srv}/sightmap`))
        .then((r) => r.json())
        .then((data) => {
          // Update panel status here, in the service worker, rather than
          // waiting for content script to send sightmap-loaded back. In MV3
          // the service worker can sleep in that round-trip gap and silently
          // drop the message.
          const status = statusFromSightmap(data);
          lastSightmapStatus = status;
          broadcastToAll({ ...status, type: "sightmap-status" });
          setBadge("✓", "#4CAF50");
          sendResponse({ ok: true, data });
        })
        .catch((err) => sendResponse({ ok: false, error: err.message }));
      return true;

    case "fetch-sightmap-version":
      getServer()
        .then((srv) => fetch(`${srv}/sightmap/version`))
        .then((r) => r.text())
        .then((v) => sendResponse({ ok: true, version: v.trim() }))
        .catch((err) => sendResponse({ ok: false, error: err.message }));
      return true;

    case "sightmap-loaded":
      lastSightmapStatus = msg;
      broadcastToAll({ ...msg, type: "sightmap-status" });
      setBadge("✓", "#4CAF50");
      // Let content scripts in all tabs know the overlay is visible IF a panel is open
      if (sidePanelPorts.size > 0) pushOverlayVisible(true);
      return false;

    case "sightmap-event":
      broadcastToAll({ type: "sightmap-event", payload: msg.payload });
      return false;

    case "hover-path":
      broadcastToAll({
        type: "hover-path",
        path: msg.path,
        formatted: msg.formatted,
      });
      return false;

    case "query-overlay-visible":
      // Content script asking whether it should show the overlay on load
      sendResponse({ visible: sidePanelPorts.size > 0 });
      return false;
  }
  return false;
});

// ── Startup ───────────────────────────────────────────────────────────────────

chrome.runtime.onInstalled.addListener(() => {
  setBadge("", "#888");
});
chrome.runtime.onStartup.addListener(() => {
  setBadge("", "#888");
});
