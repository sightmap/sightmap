/**
 * extension/types.js
 *
 * Shared type definitions (as JSDoc) and the buildFormatted() utility.
 * No runtime dependencies — safe to import everywhere in the extension.
 */

// ── JSDoc typedefs ────────────────────────────────────────────────────────────

/**
 * The full compiled sightmap served by tools/sightmap-server.js.
 * @typedef {Object} CompiledSightmap
 * @property {string} site      - Hostname, e.g. "vividseats.com"
 * @property {string} version   - Monotonic string; changes whenever YAML files change.
 *                                Extension uses this to skip re-processing unchanged data.
 * @property {ViewEntry[]} views
 * @property {FlatComponent[]} components - All components, fully flattened with parentChain.
 */

/**
 * @typedef {Object} ViewEntry
 * @property {string} name   - e.g. "HomePage"
 * @property {string} route  - e.g. "/"
 */

/**
 * A single sightmap component, flattened from the YAML hierarchy.
 * Corresponds to one entry from flattenComponents() in loader.js.
 * @typedef {Object} FlatComponent
 * @property {string}               name        - e.g. "TicketListingLink"
 * @property {string}               selector    - CSS selector, possibly compound
 * @property {string[]}             parentChain - Ancestor component names, outermost first.
 *                                               e.g. ["TicketListings"]
 * @property {PropertyDescriptor[]} properties  - Extraction descriptors
 */

/**
 * @typedef {Object} PropertyDescriptor
 * @property {string}  name    - Output key, e.g. "price"
 * @property {string}  extract - Tree-closed extract form (SEP-0010):
 *                               "text" | "attr=NAME" | "PATH.prop" | "exists:PATH"
 */

/**
 * A single component match produced by resolver.resolveElement().
 * @typedef {Object} ComponentMatch
 * @property {string}               name        - Component name, e.g. "TicketListingLink"
 * @property {Record<string,string>} properties - Extracted property values
 * @property {DOMRect}              boundingBox - Live bounding rect of the matched element
 */

/**
 * A captured user interaction, enriched with sightmap component context.
 * This is the canonical event structure emitted to the DevTools panel.
 *
 * @typedef {Object} SightmapEvent
 * @property {"click"|"input"|"change"|"focus"} type
 * @property {number}          timestamp   - ms since epoch
 * @property {string}          url         - Full page URL at time of event
 * @property {ComponentMatch[]} path       - Component path, outermost → innermost
 * @property {string}          formatted   - Human-readable event string (see buildFormatted)
 * @property {EventElement}    element     - The leaf interactive node
 */

/**
 * @typedef {Object} EventElement
 * @property {string}    role  - ARIA role or tag name, e.g. "link", "button"
 * @property {string}    name  - Accessible name (text content, aria-label, etc.)
 * @property {1|2|3}     tier  - T1 = has own component, T2 = ancestor scope, T3 = orphan
 */

// ── buildFormatted ────────────────────────────────────────────────────────────

/**
 * Format a ComponentMatch[] path + event type into the canonical event string.
 *
 * Examples:
 *   "click [TicketListings] > [TicketListingLink price=\"$33\" label=\"Lawn…\"]"
 *   "click (orphaned) button \"Add to Cart\""
 *
 * @param {string}           eventType - "click", "input", etc.
 * @param {ComponentMatch[]} path      - outermost → innermost; may be empty
 * @param {EventElement}     [element] - fallback description when path is empty
 * @returns {string}
 */
export function buildFormatted(eventType, path, element) {
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
        const truncated = v.length > 50 ? v.slice(0, 47) + "\u2026" : v;
        // Escape inner quotes
        const escaped = truncated.replace(/"/g, '\\"');
        return `${k}="${escaped}"`;
      })
      .join(" ");
    return propStr ? `[${match.name} ${propStr}]` : `[${match.name}]`;
  });

  return `${eventType} ${parts.join(" > ")}`;
}
