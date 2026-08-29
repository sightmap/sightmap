/**
 * extension/resolver.js
 *
 * Pure component matching logic — no Chrome extension APIs, no CDP.
 * Uses el.closest(selector) for fast O(depth × components) hover resolution.
 *
 * This module is the portable core of the overlay; it is designed to be
 * reusable outside the extension context (e.g. in a hosted service worker
 * that has access to a serialized DOM tree).
 */

import { buildFormatted } from "./types.js";
export { buildFormatted };

// ── DOM helpers ───────────────────────────────────────────────────────────────

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

// ── Property extraction ───────────────────────────────────────────────────────

/**
 * First element in el's subtree (document order, excluding el) matched by the
 * component named `name` — found via that component's compound selector, scoped
 * to el by containment. Mirrors the Go matcher's "first descendant matched
 * component named X".
 */
function firstDescendantEl(el, name, components) {
  const def = components.find((c) => c.name === name);
  if (!def || !def.selector) return null;
  try {
    return el.querySelectorAll(def.selector)[0] ?? null;
  } catch {
    return null;
  }
}

/** Walk a dotted component-name path into el's subtree; deepest element or null. */
function resolvePath(el, path, components) {
  let cur = el;
  for (const seg of path.split(".")) {
    if (!seg) return null;
    const next = firstDescendantEl(cur, seg, components);
    if (!next) return null;
    cur = next;
  }
  return cur === el ? null : cur;
}

/**
 * Resolve one SEP-0010 extract directive against el, over the component tree.
 * References descend only, so recursion always terminates.
 */
function resolveExtract(el, extract, components) {
  if (extract === "text") return el.textContent;
  if (typeof extract !== "string") return null;
  if (extract.startsWith("attr=")) return el.getAttribute(extract.slice(5));
  if (extract.startsWith("exists:")) {
    return resolvePath(el, extract.slice(7), components) ? "true" : null;
  }
  // PATH.prop — the descendant component's own extracted property.
  const dot = extract.lastIndexOf(".");
  if (dot <= 0 || dot === extract.length - 1) return null;
  const path = extract.slice(0, dot);
  const target = resolvePath(el, path, components);
  if (!target) return null;
  const def = components.find((c) => c.name === path.split(".").pop());
  if (!def) return null;
  const pd = (def.properties || []).find((p) => p.name === extract.slice(dot + 1));
  if (!pd) return null;
  return resolveExtract(target, pd.extract, components);
}

/**
 * Extract property values for a matched element, resolved over the component
 * tree (SEP-0010): text/attr read the element itself; PATH.prop and exists:PATH
 * reference descendant components. `text` is the element's DOM text content — the
 * extension's implementation-defined accessible text.
 *
 * @param {Element}             el
 * @param {import("./types.js").PropertyDescriptor[]} descriptors
 * @param {import("./types.js").FlatComponent[]}      components
 * @returns {Record<string,string>}
 */
export function extractProperties(el, descriptors, components) {
  if (!descriptors || !descriptors.length) return {};
  const result = {};
  for (const desc of descriptors) {
    try {
      let val = resolveExtract(el, desc.extract, components || []);
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
 * Parent scoping is enforced: a child component is only included if its matched
 * ancestor is inside the immediate parent component's matched element.
 *
 * @param {Element}                              el         - Hovered or clicked element
 * @param {import("./types.js").FlatComponent[]} components - Flat list from CompiledSightmap
 * @returns {import("./types.js").ComponentMatch[]}          Outermost → innermost
 */
export function resolveElement(el, components) {
  if (!el || !components || !components.length) return [];

  /** @type {Map<string, Element>} Maps component name → its matched ancestor element */
  const matchedElements = new Map();
  const matchList = []; // { name, element, depth, properties }

  for (const comp of components) {
    if (!comp.selector) continue;

    let ancestor;
    try {
      ancestor = el.closest(comp.selector);
    } catch {
      continue; // invalid selector
    }
    if (!ancestor) continue;

    // ── Parent scoping ─────────────────────────────────────────────────
    // If this component has a parent chain, verify the matched ancestor
    // is actually inside the immediate parent's matched element.
    if (comp.parentChain && comp.parentChain.length > 0) {
      const parentName = comp.parentChain[comp.parentChain.length - 1];
      const parentEl = matchedElements.get(parentName);
      if (parentEl) {
        // The ancestor must be the parentEl or a descendant of it
        if (parentEl !== ancestor && !parentEl.contains(ancestor)) continue;
      } else {
        // Parent wasn't matched yet — find it from the ancestor's lineage
        // (handles cases where components array ordering isn't depth-first)
        const parentComp = components.find((c) => c.name === parentName);
        if (parentComp) {
          let parentMatch;
          try {
            parentMatch = ancestor.closest(parentComp.selector);
          } catch {
            continue;
          }
          if (!parentMatch) continue;
          matchedElements.set(parentName, parentMatch);
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

  // Sort by depth ascending (shallowest = outermost first)
  matchList.sort((a, b) => a.depth - b.depth);

  return matchList.map((m) => ({
    name: m.name,
    properties: extractProperties(m.element, m.properties, components),
    boundingBox: m.element.getBoundingClientRect(),
  }));
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
  // T1: the element itself (or its direct tag) is the innermost match's element
  const innermost = path[path.length - 1];
  const innermostComp = components.find((c) => c.name === innermost.name);
  if (innermostComp) {
    try {
      if (el.matches(innermostComp.selector)) return 1;
    } catch {}
  }
  return 2;
}
