/**
 * extension/content.js
 *
 * Content script — runs in every page.
 * Responsibilities:
 *   1. Fetch compiled sightmap from tools/sightmap-server.js (localhost:7891)
 *   2. Poll for version changes and re-fetch on update
 *   3. On hover: resolve component path, draw bounding box overlay
 *   4. On click/input: emit SightmapEvent to background → DevTools panel
 *
 * Keyboard shortcut: Alt+S toggles the overlay on/off.
 */

// resolver.js and types.js inlined — content scripts run as classic scripts
// and don't support ES module import in MV3 declarative content_scripts.
// Build step (esbuild) will restore the modular structure later.

function buildFormatted(eventType, path, element) {
  if (!path || path.length === 0) {
    if (element) {
      const name = element.name ? ` "${element.name.slice(0, 60)}"` : "";
      return `${eventType} (orphaned) ${element.role}${name}`;
    }
    return `${eventType} (unresolved)`;
  }
  const parts = path.map((match) => {
    const propStr = Object.entries(match.properties)
      .map(([k, v]) => {
        const t = v.length > 50 ? v.slice(0, 47) + "\u2026" : v;
        return `${k}="${t.replace(/"/g, '\\"')}"`;
      })
      .join(" ");
    return propStr ? `[${match.name} ${propStr}]` : `[${match.name}]`;
  });
  return `${eventType} ${parts.join(" > ")}`;
}

function domDepth(el) {
  let d = 0,
    n = el;
  while (n && n !== document.documentElement) {
    d++;
    n = n.parentElement;
  }
  return d;
}

function extractProperties(el, descriptors) {
  if (!descriptors || !descriptors.length) return {};
  const result = {};
  for (const desc of descriptors) {
    try {
      const { name, extract, transform } = desc;
      let val = null;
      if (extract === "text") val = el.textContent;
      else if (extract === "inner_text") val = el.innerText;
      else if (extract === "text_only") {
        const c = el.cloneNode(true);
        c.querySelectorAll("img,svg,[alt]").forEach((n) => n.remove());
        val = c.textContent;
      } else if (extract === "inner_html") val = el.innerHTML;
      else if (extract?.startsWith("attr="))
        val = el.getAttribute(extract.slice(5));
      else if (extract?.startsWith("exists:"))
        val = el.querySelector(extract.slice(7)) ? "true" : null;
      else if (typeof extract === "string" && extract.length > 0) {
        const ch = el.querySelector(extract);
        if (ch) val = ch.innerText ?? ch.textContent;
      }
      if (val == null || val === "") continue;
      val = String(val).trim().replace(/\s+/g, " ");
      // Transforms — canonical set mirrored in sightmap/property.go,
      // cmd_snapshot.go jsTemplate, and resolver.js. No-match → value unchanged.
      if (transform === "first_word") val = val.split(/\s+/)[0] ?? val;
      else if (transform === "last_word") val = val.split(/\s+/).pop() ?? val;
      else if (transform === "first_number") {
        const m = val.match(/\d[\d,.]*/);
        if (m) val = m[0];
      } else if (transform === "first_dollar") {
        const m = val.match(/\$[\d,.]+/);
        if (m) val = m[0];
      } else if (transform === "number") {
        val = val.replace(/[^\d.]/g, "");
      } else if (transform === "slug") {
        val = val
          .toLowerCase()
          .replace(/\s+/g, "-")
          .replace(/[^a-z0-9-]/g, "");
      } else if (
        typeof transform === "string" &&
        transform.startsWith("match:")
      ) {
        // capture group 1 if present, else full match; no match → unchanged
        const m = val.match(new RegExp(transform.slice(6)));
        if (m) val = m[1] != null ? m[1] : m[0];
      }
      if (val) result[name] = val.slice(0, 120);
    } catch {
      /* skip */
    }
  }
  return result;
}

function resolveElement(el, components) {
  if (!el || !components || !components.length) return [];
  const matchedElements = new Map();
  const matchList = [];
  for (const comp of components) {
    if (!comp.selector) continue;
    let ancestor;
    try {
      ancestor = el.closest(comp.selector);
    } catch {
      continue;
    }
    if (!ancestor) continue;
    if (comp.parentChain && comp.parentChain.length > 0) {
      const parentName = comp.parentChain[comp.parentChain.length - 1];
      const parentEl = matchedElements.get(parentName);
      if (parentEl) {
        if (parentEl !== ancestor && !parentEl.contains(ancestor)) continue;
      } else {
        const parentComp = components.find((c) => c.name === parentName);
        if (parentComp) {
          let pm;
          try {
            pm = ancestor.closest(parentComp.selector);
          } catch {
            continue;
          }
          if (!pm) continue;
          matchedElements.set(parentName, pm);
        }
      }
    }
    matchedElements.set(comp.name, ancestor);
    matchList.push({
      name: comp.name,
      element: ancestor,
      depth: domDepth(ancestor),
      properties: comp.properties ?? [],
    });
  }
  matchList.sort((a, b) => a.depth - b.depth);
  return matchList.map((m) => ({
    name: m.name,
    properties: extractProperties(m.element, m.properties),
    boundingBox: m.element.getBoundingClientRect(),
  }));
}

function resolveTier(el, path, components) {
  if (!path.length) return 3;
  const innermost = path[path.length - 1];
  const innermostComp = components.find((c) => c.name === innermost.name);
  if (innermostComp) {
    try {
      if (el.matches(innermostComp.selector)) return 1;
    } catch {}
  }
  return 2;
}

const POLL_MS = 4000; // check for sightmap version changes every 4s (via background proxy)

// ── State ─────────────────────────────────────────────────────────────────────

// matchRoute — mirrors tools/lib/loader.js for client-side view selection
function matchRoute(pattern, pathname) {
  const re =
    "^" +
    pattern
      .replace(/[.+^${}()|[\]\\]/g, "\\$&")
      .replace(/\*\*/g, "\x01")
      .replace(/\*/g, "[^/]+")
      .replace(/\x01/g, ".*") +
    "$";
  return new RegExp(re).test(pathname);
}

const state = {
  /** @type {import("./types.js").FlatComponent[]} globals always-active */
  globals: [],
  /** @type {Record<string, import("./types.js").FlatComponent[]>} route→comps */
  viewComponents: {},
  /** @type {{name:string, route:string}[]} */
  views: [],
  version: null,
  enabled: true, // Alt+S toggle
  overlayEl: null,
  tooltipEl: null,
  hoverTarget: null,
};

/** Return the active component list for the current URL. */
function activeComponents() {
  const pathname = location.pathname;
  // Build a deduplicated list: globals first, then view-specific (view wins).
  const byName = new Map();
  for (const c of state.globals) byName.set(c.name, c);
  for (const view of state.views) {
    if (!matchRoute(view.route, pathname)) continue;
    for (const c of state.viewComponents[view.route] ?? []) {
      byName.set(c.name, c);
    }
  }
  return [...byName.values()];
}

// ── Sightmap loading ──────────────────────────────────────────────────────────

// Proxy fetches through the background service worker to avoid mixed-content
// blocks (content script on https:// cannot directly fetch http://localhost).
function bgFetch(type) {
  return new Promise((resolve, reject) => {
    chrome.runtime.sendMessage({ type }, (resp) => {
      if (chrome.runtime.lastError)
        return reject(new Error(chrome.runtime.lastError.message));
      if (!resp || !resp.ok)
        return reject(new Error(resp?.error ?? "fetch failed"));
      resolve(resp);
    });
  });
}

async function fetchSightmap() {
  try {
    const resp = await bgFetch("fetch-sightmap");
    const data = resp.data;
    state.globals = data.globals ?? [];
    state.viewComponents = data.viewComponents ?? {};
    state.views = data.views ?? [];
    state.version = data.version;
    const total =
      state.globals.length +
      Object.values(state.viewComponents).reduce((s, a) => s + a.length, 0);
    console.log(
      `[sightmap] loaded ${total} components from ${data.site} v${data.version}`,
    );
    chrome.runtime
      .sendMessage({
        type: "sightmap-loaded",
        version: data.version,
        site: data.site,
        componentCount: total,
      })
      .catch(() => {});
  } catch (err) {
    // Server not running — silent; will retry on next poll
  }
}

async function pollVersion() {
  if (!state.version) {
    // Initial fetch failed (server not yet ready, or service worker was sleeping).
    // Retry until it succeeds.
    await fetchSightmap();
    return;
  }
  try {
    const resp = await bgFetch("fetch-sightmap-version");
    const v = resp.version;
    if (v !== state.version) {
      console.log(
        `[sightmap] version changed (${state.version} → ${v}), reloading`,
      );
      await fetchSightmap();
    }
  } catch {
    // Server went away — keep showing the old sightmap
  }
}

// ── Overlay ───────────────────────────────────────────────────────────────────

const TIER_COLORS = {
  1: "#4CAF50", // green  — T1 direct component
  2: "#2196F3", // blue   — T2 ancestor scope
  3: "#F44336", // red    — T3 orphan
};

function ensureOverlay() {
  if (state.overlayEl) return;

  const overlay = document.createElement("div");
  overlay.id = "__sightmap-overlay";
  Object.assign(overlay.style, {
    position: "fixed",
    inset: "0",
    pointerEvents: "none",
    zIndex: "2147483646",
    overflow: "hidden",
  });

  const tooltip = document.createElement("div");
  tooltip.id = "__sightmap-tooltip";
  Object.assign(tooltip.style, {
    position: "fixed",
    background: "rgba(15,15,15,0.92)",
    color: "#e8e8e8",
    fontFamily: "monospace",
    fontSize: "11px",
    lineHeight: "1.5",
    padding: "5px 8px",
    borderRadius: "4px",
    border: "1px solid #444",
    maxWidth: "460px",
    whiteSpace: "pre-wrap",
    wordBreak: "break-all",
    pointerEvents: "none",
    display: "none",
    zIndex: "2147483647",
    boxShadow: "0 2px 8px rgba(0,0,0,0.5)",
  });

  document.documentElement.appendChild(overlay);
  document.documentElement.appendChild(tooltip);
  state.overlayEl = overlay;
  state.tooltipEl = tooltip;
}

function clearOverlay() {
  if (state.overlayEl) state.overlayEl.innerHTML = "";
  if (state.tooltipEl) state.tooltipEl.style.display = "none";
}

/**
 * @param {import("./types.js").ComponentMatch[]} path
 * @param {Element} target
 */
function renderOverlay(path, target) {
  if (!state.overlayEl) return;
  state.overlayEl.innerHTML = "";

  if (!path.length) return;

  const tier = resolveTier(target, path, activeComponents());

  // Draw one box per component in the path (outermost = thinnest)
  path.forEach((match, i) => {
    const isInnermost = i === path.length - 1;
    const r = match.boundingBox;
    if (!r || r.width === 0 || r.height === 0) return;

    const box = document.createElement("div");
    const color = TIER_COLORS[isInnermost ? tier : 2];
    Object.assign(box.style, {
      position: "fixed",
      left: `${r.left}px`,
      top: `${r.top}px`,
      width: `${r.width}px`,
      height: `${r.height}px`,
      border: `${isInnermost ? 2 : 1}px solid ${color}`,
      boxSizing: "border-box",
      // Subtle background on innermost only
      background: isInnermost ? `${color}10` : "transparent",
    });
    state.overlayEl.appendChild(box);
  });

  // Position and populate tooltip near the cursor
  const innermost = path[path.length - 1];
  const lines = path
    .map((m) => {
      const props = Object.entries(m.properties)
        .map(([k, v]) => `  ${k}="${v}"`)
        .join("\n");
      return props ? `[${m.name}]\n${props}` : `[${m.name}]`;
    })
    .join("\n▸ ");

  state.tooltipEl.textContent = lines;

  // Place tooltip: try right of inner box, fall back to left
  const inner = innermost.boundingBox;
  const tW = 460,
    tH = 80; // estimated tooltip size
  const vW = window.innerWidth,
    vH = window.innerHeight;

  let tx = Math.min(inner.right + 8, vW - tW - 4);
  let ty = Math.max(4, Math.min(inner.top, vH - tH - 4));

  state.tooltipEl.style.left = `${tx}px`;
  state.tooltipEl.style.top = `${ty}px`;
  state.tooltipEl.style.display = "block";
}

// ── Hover handling ────────────────────────────────────────────────────────────

let hoverRaf = null;

function onMouseMove(e) {
  if (!state.enabled || !state.globals.length) return;

  // Ignore events on our own overlay elements
  if (e.target?.id?.startsWith?.("__sightmap")) return;

  state.hoverTarget = e.target;

  if (hoverRaf) cancelAnimationFrame(hoverRaf);
  hoverRaf = requestAnimationFrame(() => {
    if (!state.hoverTarget) return;
    const comps = activeComponents();
    const path = resolveElement(state.hoverTarget, comps);
    renderOverlay(path, state.hoverTarget);

    chrome.runtime
      .sendMessage({
        type: "hover-path",
        path,
        formatted: buildFormatted("hover", path),
      })
      .catch(() => {});
  });
}

function onMouseLeave() {
  state.hoverTarget = null;
  clearOverlay();
  chrome.runtime
    .sendMessage({ type: "hover-path", path: [], formatted: "" })
    .catch(() => {});
}

// ── Event capture ─────────────────────────────────────────────────────────────

/**
 * @param {string} eventType
 * @param {Element} target
 */
function emitEvent(eventType, target) {
  if (!state.version) return; // sightmap not yet loaded

  const comps = activeComponents();
  const path = resolveElement(target, comps);
  const tier = resolveTier(target, path, comps);

  // Best-effort accessible name
  const role = target.getAttribute("role") || target.tagName.toLowerCase();
  const name = (
    target.getAttribute("aria-label") ||
    (target.getAttribute("aria-labelledby") &&
      document.getElementById(target.getAttribute("aria-labelledby"))
        ?.textContent) ||
    target.textContent ||
    target.getAttribute("placeholder") ||
    ""
  )
    .trim()
    .slice(0, 80);

  /** @type {import("./types.js").SightmapEvent} */
  const ev = {
    type: eventType,
    timestamp: Date.now(),
    url: location.href,
    path,
    formatted: buildFormatted(eventType, path, { role, name, tier }),
    element: { role, name, tier },
  };

  chrome.runtime
    .sendMessage({ type: "sightmap-event", payload: ev })
    .catch(() => {});
  console.log(`[sightmap] ${ev.formatted}`);
}

// ── Event listeners ───────────────────────────────────────────────────────────

function attachListeners() {
  document.addEventListener("mousemove", onMouseMove, { passive: true });
  document.addEventListener("mouseleave", onMouseLeave, { passive: true });

  // Click capture (useCapture=true to see all clicks before stopPropagation)
  document.addEventListener(
    "click",
    (e) => {
      // Never act on clicks within our own overlay UI.
      if (e.target?.id?.startsWith?.("__sightmap")) return;

      if (!state.enabled) return;
      emitEvent("click", e.target);
    },
    true,
  );

  // Input capture
  document.addEventListener(
    "input",
    (e) => {
      if (!state.enabled) return;
      emitEvent("input", e.target);
    },
    true,
  );

  // Alt+S — toggle overlay.
  // Use e.code (physical key): on macOS, holding Option rewrites e.key to a
  // composed glyph (Option+S → "ß"), so e.key checks never fire.
  document.addEventListener("keydown", (e) => {
    if (e.altKey && e.code === "KeyS") {
      state.enabled = !state.enabled;
      if (!state.enabled) clearOverlay();
      console.log(
        `[sightmap] overlay ${state.enabled ? "enabled" : "disabled"}`,
      );
    }
  });
}

// ── Overlay visibility (controlled by side panel open/close) ─────────────────

function setOverlayVisible(v) {
  state.enabled = v;
  if (!v) clearOverlay();
  // Show or hide the floating pill inversely: pill when panel is closed
  const pill = document.getElementById("__sightmap-pill");
  if (pill) pill.style.display = v ? "none" : "flex";
}

chrome.runtime.onMessage.addListener((msg) => {
  if (msg.type === "overlay-visible") setOverlayVisible(msg.value);
});

// ── Bootstrap ───────────────────────────────────────────────────────────────

(async function init() {
  // Guard: only run once per page
  if (window.__sightmapOverlayLoaded) return;
  window.__sightmapOverlayLoaded = true;

  ensureOverlay();
  attachListeners();

  // Re-resolve and re-render the overlay for the current hoverTarget.
  // Called when the DOM or viewport changes so the bounding box stays in sync.
  function refreshOverlay() {
    if (!state.hoverTarget) return;
    if (!document.contains(state.hoverTarget)) {
      clearOverlay();
      state.hoverTarget = null;
      return;
    }
    const path = resolveElement(state.hoverTarget, activeComponents());
    if (path.length) {
      renderOverlay(path, state.hoverTarget);
    } else {
      clearOverlay();
    }
  }

  // DOM mutations — handles removal, new nodes shifting element positions,
  // and conditional re-renders (SPA navigation, Preact reconciliation).
  // Debounced so rapid batched mutations don't cause continuous re-resolution.
  let mutationRaf = null;
  new MutationObserver(() => {
    if (!state.hoverTarget) return;
    if (mutationRaf) return;
    mutationRaf = requestAnimationFrame(() => {
      mutationRaf = null;
      refreshOverlay();
    });
  }).observe(document.body, { childList: true, subtree: true });

  // Scroll — element positions change relative to the fixed overlay boxes.
  // Use capture so we catch scrolls on any scrollable container, not just window.
  window.addEventListener(
    "scroll",
    () => {
      if (!state.hoverTarget) return;
      requestAnimationFrame(refreshOverlay);
    },
    { passive: true, capture: true },
  );

  // Overlay starts hidden — enabled only when side panel is open
  state.enabled = false;
  chrome.runtime.sendMessage({ type: "query-overlay-visible" }, (resp) => {
    if (resp?.visible) setOverlayVisible(true);
  });

  await fetchSightmap();

  // Poll for sightmap changes
  setInterval(pollVersion, POLL_MS);
})();
