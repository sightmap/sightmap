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
 * Extract property values from a matched DOM element.
 * Mirrors the extraction logic in tools/snapshot.js extractNodeProperties().
 *
 * @param {Element}             el
 * @param {import("./types.js").PropertyDescriptor[]} descriptors
 * @returns {Record<string,string>}
 */
export function extractProperties(el, descriptors) {
  if (!descriptors || !descriptors.length) return {};
  const result = {};

  for (const desc of descriptors) {
    try {
      const { name, extract, transform } = desc;
      let val = null;

      // ── Extraction ──────────────────────────────────────────────────────
      if (extract === "text") {
        val = el.textContent;
      } else if (extract === "inner_text") {
        val = el.innerText;
      } else if (extract === "text_only") {
        const clone = el.cloneNode(true);
        clone.querySelectorAll("img,svg,[alt]").forEach((n) => n.remove());
        val = clone.textContent;
      } else if (extract === "inner_html") {
        val = el.innerHTML;
      } else if (typeof extract === "string" && extract.startsWith("attr=")) {
        val = el.getAttribute(extract.slice(5));
      } else if (typeof extract === "string" && extract.startsWith("exists:")) {
        val = el.querySelector(extract.slice(7)) ? "true" : null;
      } else if (typeof extract === "string" && extract.length > 0) {
        // CSS sub-selector → innerText of first match (falls back to textContent)
        const child = el.querySelector(extract);
        if (child) val = child.innerText ?? child.textContent;
      }

      if (val == null || val === "") continue;
      val = String(val).trim().replace(/\s+/g, " ");

      // ── Transforms ──────────────────────────────────────────────────────
      // Canonical set mirrored in content.js. sightmap/property.go was removed
      // in SEP-0010; the extension keeps its own copy since it has no access
      // to the Go matcher. No match → value passes through unchanged.
      if (transform === "first_word") {
        val = val.split(/\s+/)[0] ?? val;
      } else if (transform === "last_word") {
        val = val.split(/\s+/).pop() ?? val;
      } else if (transform === "first_number") {
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

      if (val) result[name] = val.slice(0, 120); // cap at 120 chars
    } catch {
      // Invalid selector, extraction error — skip this property
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
    properties: extractProperties(m.element, m.properties),
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
