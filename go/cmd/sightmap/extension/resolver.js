/**
 * extension/resolver.js
 *
 * Pure component matching logic — no Chrome extension APIs, no CDP.
 * Uses el.closest(selector) for fast O(depth × components) hover resolution.
 *
 * This module is the portable core of the overlay; it is designed to be
 * reusable outside the extension context (e.g. in a hosted service worker
 * that has access to a serialized DOM tree).
 *
 * Everything between the SHARED-WITH-CONTENT markers below is mirrored verbatim
 * into content.js, which is a classic MV3 content script and cannot import an ES
 * module. resolver.test.js fails if the two copies drift — edit here, then run
 * `node scripts/sync-extension-resolver.mjs`.
 */

import { buildFormatted } from "./types.js";
export { buildFormatted };

// ─── SHARED-WITH-CONTENT:START ───────────────────────────────────────────────

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
export function componentAddress(comp) {
  return [...(comp.parentChain ?? []), comp.name].join(ADDRESS_SEP);
}

/** The address of a component's parent, or "" for a root component. */
export function parentAddress(comp) {
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
export function dedupeByAddress(...lists) {
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
export function extractProperties(
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
export function resolveElement(el, components) {
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
export function deepestComponentAt(root, x, y, components) {
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
export function resolveTier(el, path, components) {
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

// ─── SHARED-WITH-CONTENT:END ─────────────────────────────────────────────────
