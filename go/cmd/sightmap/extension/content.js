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

// resolver.js and types.js are inlined below: MV3 declarative content_scripts
// run as CLASSIC scripts, with no `import` and no "type": "module" to opt in.
// The resolver half is GENERATED from resolver.js by
// scripts/sync-extension-resolver.mjs; resolver.test.js fails on drift. Edit
// resolver.js, not the block between the SHARED-WITH-CONTENT markers.

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

// ─── SHARED-WITH-CONTENT:START ───

/** Count how many elements are between el and documentElement. */
function domDepth(el) {
  let depth = 0;
  let node = el;
  while (node && node !== document.documentElement) {
    depth++;
    node = node.parentElement;
  }
  return depth;
}

// ── Component addressing ──────────────────────────────────────────────────────

/**
 * Separator joining a component's ancestor names into its address.
 *
 * NUL, because the spec puts no charset restriction on a component `name`
 * (`{type: "string", minLength: 1}`), so any printable separator could in
 * principle occur inside one. Addresses are internal keys and are never shown.
 */
const ADDRESS_SEP = "\u0000";

/**
 * A component's identity: its ancestor chain plus its own name.
 *
 * Component names are unique only WITHIN A PARENT — that scoping is the whole
 * point of `children:` in the spec ("this is how Sightmap avoids naming
 * collisions between, say, two different card components that both contain a
 * button.primary"). So a bare name is NOT an identity, and any map keyed by one
 * silently merges unrelated components.
 *
 * That is not an edge case on real pages. A production sign-in corpus we measured
 * has 104 components under 54 distinct names — `Text` ×18, `Label` ×13 — so a
 * name-keyed map discards 48% of the map before matching even starts.
 *
 * @param {import("./types.js").FlatComponent} comp
 * @returns {string}
 */
function componentAddress(comp) {
  return [...(comp.parentChain ?? []), comp.name].join(ADDRESS_SEP);
}

/** The address of a component's parent, or "" for a root component. */
function parentAddress(comp) {
  return (comp.parentChain ?? []).join(ADDRESS_SEP);
}

/**
 * Collapse component lists into one active set, later entries winning at the
 * same ADDRESS — the merge a caller wants when overlaying view-scoped components
 * onto file-root globals.
 *
 * Keyed by address, not name: a view component should override an identically
 * *addressed* global, but two components merely sharing a leaf name are
 * unrelated and must both survive.
 *
 * @param {...import("./types.js").FlatComponent[]} lists  low → high precedence
 * @returns {import("./types.js").FlatComponent[]}
 */
function dedupeByAddress(...lists) {
  const byAddress = new Map();
  for (const list of lists) {
    for (const comp of list ?? []) byAddress.set(componentAddress(comp), comp);
  }
  return [...byAddress.values()];
}

/** The component named `name` whose parent is `ownerAddress`, or null. */
function childNamed(components, ownerAddress, name) {
  return (
    components.find(
      (c) => parentAddress(c) === ownerAddress && c.name === name,
    ) ?? null
  );
}

// ── Property extraction ───────────────────────────────────────────────────────

/**
 * First element in el's subtree (document order, excluding el) matched by `def`.
 * Mirrors the Go matcher's "first descendant matched component named X".
 */
function firstDescendantEl(el, def) {
  if (!def || !def.selector) return null;
  try {
    return el.querySelectorAll(def.selector)[0] ?? null;
  } catch {
    return null;
  }
}

/**
 * Walk a dotted component-name path down from `el`, resolving each segment among
 * the CHILDREN of the component reached so far — starting from `ownerAddress`.
 *
 * Resolving a segment by bare name instead would pick an arbitrary namesake from
 * anywhere in the corpus: a submit button's `Label.label` resolved `Label` to a
 * text field's label somewhere else entirely, found nothing inside the button,
 * and silently dropped the property.
 *
 * @returns {{el: Element, def: object}|null} the deepest segment's element + def
 */
function resolvePath(el, path, components, ownerAddress) {
  let cur = el;
  let addr = ownerAddress;
  let def = null;
  for (const seg of path.split(".")) {
    if (!seg) return null;
    def = childNamed(components, addr, seg);
    if (!def) return null;
    const next = firstDescendantEl(cur, def);
    if (!next) return null;
    cur = next;
    addr = componentAddress(def);
  }
  return cur === el ? null : { el: cur, def };
}

/**
 * Resolve one SEP-0010 extract directive against el, over the component tree.
 * References descend only, so recursion always terminates.
 */
function resolveExtract(el, extract, components, ownerAddress) {
  if (extract === "text") return el.textContent;
  if (typeof extract !== "string") return null;
  if (extract.startsWith("attr=")) return el.getAttribute(extract.slice(5));
  if (extract.startsWith("exists:")) {
    return resolvePath(el, extract.slice(7), components, ownerAddress)
      ? "true"
      : null;
  }
  // PATH.prop — the descendant component's own extracted property.
  const dot = extract.lastIndexOf(".");
  if (dot <= 0 || dot === extract.length - 1) return null;
  const hit = resolvePath(el, extract.slice(0, dot), components, ownerAddress);
  if (!hit) return null;
  const pd = (hit.def.properties || []).find(
    (p) => p.name === extract.slice(dot + 1),
  );
  if (!pd) return null;
  return resolveExtract(
    hit.el,
    pd.extract,
    components,
    componentAddress(hit.def),
  );
}

/**
 * Extract property values for a matched element, resolved over the component
 * tree (SEP-0010): text/attr read the element itself; PATH.prop and exists:PATH
 * reference descendant components. `text` is the element's DOM text content — the
 * extension's implementation-defined accessible text.
 *
 * `ownerAddress` is the address of the component these descriptors belong to;
 * descendant references resolve among ITS children. It defaults to "" (a root
 * component), which is also what a caller with no hierarchy wants.
 *
 * @param {Element}             el
 * @param {import("./types.js").PropertyDescriptor[]} descriptors
 * @param {import("./types.js").FlatComponent[]}      components
 * @param {string}              [ownerAddress]
 * @returns {Record<string,string>}
 */
function extractProperties(
  el,
  descriptors,
  components,
  ownerAddress = "",
) {
  if (!descriptors || !descriptors.length) return {};
  const result = {};
  for (const desc of descriptors) {
    try {
      let val = resolveExtract(el, desc.extract, components || [], ownerAddress);
      if (val == null || val === "") continue;
      val = String(val).trim().replace(/\s+/g, " ");
      if (val) result[desc.name] = val.slice(0, 120); // cap at 120 chars
    } catch {
      // Extraction error — skip this property
    }
  }
  return result;
}

// ── Component resolution ──────────────────────────────────────────────────────

/**
 * Find all sightmap components that contain `el`, sorted outermost → innermost.
 *
 * Uses el.closest(selector) which:
 *  - Handles compound selectors (descendant combinators, attribute filters, etc.)
 *  - Returns the nearest ancestor-or-self that matches the selector
 *  - Is O(depth) per component — fast enough for 60fps hover
 *
 * Parent scoping is enforced BY ADDRESS: a child is included only if its own
 * parent matched, and matched inside it. Components are visited ancestor-first so
 * that "its parent matched" is always decidable, whatever order the caller
 * supplied them in.
 *
 * @param {Element}                              el         - Hovered or clicked element
 * @param {import("./types.js").FlatComponent[]} components - Flat list from CompiledSightmap
 * @returns {import("./types.js").ComponentMatch[]}          Outermost → innermost
 */
function resolveElement(el, components) {
  if (!el || !components || !components.length) return [];

  // Ancestor-first. A component can only be scoped once its parent has been
  // resolved, and the caller's order is not guaranteed to be depth-first.
  const ordered = [...components].sort(
    (a, b) => (a.parentChain?.length ?? 0) - (b.parentChain?.length ?? 0),
  );

  /** @type {Map<string, Element>} component ADDRESS → its matched ancestor */
  const matchedByAddress = new Map();
  const matchList = []; // { comp, element, depth }

  for (const comp of ordered) {
    if (!comp.selector) continue;

    let ancestor;
    try {
      ancestor = el.closest(comp.selector);
    } catch {
      continue; // invalid selector
    }
    if (!ancestor) continue;

    if ((comp.parentChain?.length ?? 0) > 0) {
      // A component whose own parent did not match is not in scope, however much
      // its selector looks like a hit. Looking the parent up by bare name would
      // accept an unrelated namesake's match as authority.
      const parentEl = matchedByAddress.get(parentAddress(comp));
      if (!parentEl) continue;
      if (parentEl !== ancestor && !parentEl.contains(ancestor)) continue;
    }

    matchedByAddress.set(componentAddress(comp), ancestor);
    matchList.push({ comp, element: ancestor, depth: domDepth(ancestor) });
  }

  // Sort by depth ascending (shallowest = outermost first)
  matchList.sort((a, b) => a.depth - b.depth);

  return matchList.map((m) => ({
    name: m.comp.name,
    address: componentAddress(m.comp),
    properties: extractProperties(
      m.element,
      m.comp.properties ?? [],
      components,
      componentAddress(m.comp),
    ),
    boundingBox: m.element.getBoundingClientRect(),
  }));
}

/**
 * Descend from a hover/hit target INTO the component subtree below it.
 *
 * Bottom-up resolveElement() (via closest()) fails when the target is an
 * ANCESTOR of the real component — e.g. a control with pointer-events:none whose
 * hit-test bubbles up to a styling wrapper that sits ABOVE the component
 * container. Among active components, return the deepest element inside `root`
 * whose bounding box contains the pointer (x, y viewport coords), so the caller
 * can resolve from a real component element. Returns null when nothing matches.
 *
 * @param {Element}                             root
 * @param {number}                              x
 * @param {number}                              y
 * @param {import("./types.js").FlatComponent[]} components
 * @returns {Element|null}
 */
function deepestComponentAt(root, x, y, components) {
  if (!root || !components) return null;
  let best = null;
  let bestDepth = -1;
  for (const comp of components) {
    if (!comp.selector) continue;
    let els;
    try {
      els = root.querySelectorAll(comp.selector);
    } catch {
      continue; // invalid selector
    }
    for (const el of els) {
      const r = el.getBoundingClientRect();
      if (r.width === 0 || r.height === 0) continue;
      if (x < r.left || x > r.right || y < r.top || y > r.bottom) continue;
      const d = domDepth(el);
      if (d > bestDepth) {
        bestDepth = d;
        best = el;
      }
    }
  }
  return best;
}

/**
 * Determine the coverage tier of an element given a resolved path.
 * T1 = element itself has a direct component match
 * T2 = element is inside a matched component but not directly matched
 * T3 = no component context at all
 *
 * @param {Element}                              el
 * @param {import("./types.js").ComponentMatch[]} path
 * @param {import("./types.js").FlatComponent[]}  components
 * @returns {1|2|3}
 */
function resolveTier(el, path, components) {
  if (!path.length) return 3;
  // T1: the element itself is the innermost match's element. Looked up by
  // address — by name would test an unrelated namesake's selector.
  const innermost = path[path.length - 1];
  const innermostComp = components.find(
    (c) => componentAddress(c) === innermost.address,
  );
  if (innermostComp) {
    try {
      if (el.matches(innermostComp.selector)) return 1;
    } catch {}
  }
  return 2;
}

// ─── SHARED-WITH-CONTENT:END ───

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
  /** @type {{name:string, route:string, components:import("./types.js").FlatComponent[]}[]} */
  views: [],
  version: null,
  enabled: true, // Alt+S toggle
  overlayEl: null,
  tooltipEl: null,
  hoverTarget: null,
  hoverX: 0,
  hoverY: 0,
};

/**
 * Normalize a corpus ComponentDef (wire shape: a selectors[] array, components
 * nested under each view) into the flat component the resolver consumes: a
 * single closest()-compatible selector string plus parentChain/properties.
 * Keeping the selector join here means resolver.js stays agnostic to the wire
 * shape.
 */
function normalizeComp(c) {
  return {
    name: c.name,
    selector: (c.selectors ?? []).join(", "),
    parentChain: c.parentChain ?? [],
    properties: c.properties ?? [],
  };
}

/**
 * Return the active component list for the current URL.
 *
 * Deduplication is by component ADDRESS, not by name. The intent is only that a
 * view-scoped component should win over an identically-ADDRESSED global; keying
 * by bare name instead threw away every nested namesake, because component names
 * are unique only within a parent. On a real page that is most of the corpus:
 * a production sign-in map of 104 components has 54 distinct names, so 50 of
 * them vanished before matching, and a click resolved two levels deep instead of
 * six.
 */
function activeComponents() {
  const pathname = location.pathname;
  // Globals first, then view-specific (view wins at the same address).
  // Globals first, then every matching view (view wins at the same address).
  return dedupeByAddress(
    state.globals,
    ...state.views
      .filter((v) => matchRoute(v.route, pathname))
      .map((v) => v.components),
  );
}

/**
 * True once a corpus has actually loaded. fetchSightmap only records
 * state.version when the fetch came back with a non-empty component set, so the
 * version stamp IS the "loaded and usable" signal.
 *
 * Do NOT use state.globals.length for this. `globals` is the optional file-root
 * components.yaml list, and it is omitempty on the wire — a corpus whose
 * components are all view-scoped (the common case) ships no `globals` key at
 * all. Gating on it left hover permanently dead while clicks kept resolving,
 * because the click path already gated on state.version.
 */
function corpusLoaded() {
  return Boolean(state.version);
}

// ── Sightmap loading ──────────────────────────────────────────────────────────

// The content script prefers a DIRECT fetch to the local sightmap server. On an
// app served over http (the common dev case) the page can fetch http://localhost
// directly, so we bypass the background service-worker proxy entirely: the MV3 SW
// round-trip is unreliable on a cold/just-woken worker and was silently
// delivering an empty corpus at page-load time. The SW proxy (bgFetch) remains a
// fallback for https-origin pages, where mixed-content blocks a direct http
// fetch. host_permissions grants http://localhost:*, and the server sends
// Access-Control-Allow-Origin:*, so the cross-port fetch is allowed.
let directServerBase = null; // cached resolved base, e.g. "http://localhost:7891"

async function pingServer(base) {
  try {
    const r = await fetch(`${base}/sightmap/version`, {
      signal: AbortSignal.timeout(400),
    });
    return r.ok;
  } catch {
    return false;
  }
}

// Resolve the local server by direct probing: the hint from chrome.storage.local
// (injected by `browser start`), then the 7891–7900 range. Returns null when
// nothing responds or the page origin blocks the http fetch (https mixed
// content) — callers then fall back to the SW proxy.
async function discoverDirect() {
  if (directServerBase && (await pingServer(directServerBase)))
    return directServerBase;
  directServerBase = null;
  let hint = 7891;
  try {
    const s = await chrome.storage.local.get("serverPort");
    if (s.serverPort) hint = s.serverPort;
  } catch {
    /* storage unavailable — use default hint */
  }
  const ports = [hint];
  for (let p = 7891; p <= 7900; p++) if (p !== hint) ports.push(p);
  for (const p of ports) {
    const base = `http://localhost:${p}`;
    if (await pingServer(base)) {
      directServerBase = base;
      return base;
    }
  }
  return null;
}

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

// Fetch the full corpus. Prefers a direct fetch; falls back to the SW proxy
// (https pages). Returns the parsed payload, or null on failure.
async function fetchSightmapData() {
  const base = await discoverDirect();
  if (base) {
    try {
      const r = await fetch(`${base}/sightmap`);
      if (r.ok) return await r.json();
    } catch {
      directServerBase = null; // direct path failed — fall through to the proxy
    }
  }
  try {
    const resp = await bgFetch("fetch-sightmap");
    return resp.data;
  } catch {
    return null;
  }
}

// Fetch just the version string, same direct-then-proxy strategy.
async function fetchVersionValue() {
  const base = await discoverDirect();
  if (base) {
    try {
      const r = await fetch(`${base}/sightmap/version`);
      if (r.ok) return (await r.json()).version;
    } catch {
      directServerBase = null;
    }
  }
  const resp = await bgFetch("fetch-sightmap-version");
  return resp.version;
}

async function fetchSightmap() {
  const data = await fetchSightmapData();
  if (!data) return false; // server unreachable — retry on next poll
  const corpus = data.corpus ?? {};
  const globals = (corpus.globals ?? []).map(normalizeComp);
  const views = (corpus.views ?? []).map((v) => ({
    name: v.name,
    route: v.route,
    components: (v.components ?? []).map(normalizeComp),
  }));
  const total =
    globals.length + views.reduce((s, v) => s + v.components.length, 0);
  // An empty corpus means the server answered before its corpus finished loading
  // (or a degraded proxy delivered a stripped payload). Do NOT cache it or record
  // its version — leave state untouched so pollVersion keeps retrying instead of
  // sticking on an empty overlay for the life of the page.
  if (total === 0) return false;
  state.globals = globals;
  state.views = views;
  state.version = data.version;
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
  return true;
}

async function pollVersion() {
  // Keep retrying until we have a non-empty corpus. state.version is only set
  // once a load succeeded WITH components, so this also covers the "server
  // answered empty at startup" case that used to stick permanently.
  if (!corpusLoaded()) {
    await fetchSightmap();
    return;
  }
  try {
    const v = await fetchVersionValue();
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
  if (!state.enabled || !corpusLoaded()) return;

  // Ignore events on our own overlay elements
  if (e.target?.id?.startsWith?.("__sightmap")) return;

  state.hoverTarget = e.target;
  state.hoverX = e.clientX;
  state.hoverY = e.clientY;

  if (hoverRaf) cancelAnimationFrame(hoverRaf);
  hoverRaf = requestAnimationFrame(() => {
    if (!state.hoverTarget) return;
    const comps = activeComponents();
    let target = state.hoverTarget;
    // Prefer the deepest component actually UNDER the pointer. Bottom-up
    // closest() from the hover target only sees components the target is INSIDE,
    // so when the target is a wrapper above the component it resolves a coarse
    // ancestor (e.g. ExpansionPanelContent) — or nothing, when the real control
    // has pointer-events:none. Descending to the deepest component element whose
    // box contains the pointer and resolving from there matches the top-down
    // "innermost wins" the CLI resolver produces.
    const inner = deepestComponentAt(target, state.hoverX, state.hoverY, comps);
    if (inner) target = inner;
    const path = resolveElement(target, comps);
    renderOverlay(path, target);

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
