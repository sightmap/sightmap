// Live-DOM discovery scans used while building a corpus. Depends on
// __smDeepQueryAll from browser/deepquery.js being prepended.

// __smLooksHashed mirrors coverage.looksHashed (Go): a token is machine-generated
// / per-instance (aura ids, hashed classnames, uuids, emotion/styled suffixes)
// rather than a stable authored hook. Keep the two in sync.
function __smLooksHashed(tok) {
  if (!tok) return true;
  if (/[:;$]/.test(tok)) return true; // aura ids "1:1;a", scoped "$x"
  if (/[0-9]{4,}/.test(tok)) return true; // counters/timestamps
  if (/[0-9a-fA-F]{8,}/.test(tok)) return true; // hashes/uuids
  for (const seg of tok.split(/[-_]/)) {
    // word-<hash>: >=6 chars mixing letters with several digits
    if (
      seg.length >= 6 &&
      /[a-zA-Z]/.test(seg) &&
      (seg.match(/[0-9]/g) || []).length >= 2
    ) {
      return true;
    }
  }
  return false;
}

const __smUtilityClasses = new Set([
  "active",
  "open",
  "closed",
  "hidden",
  "show",
  "hide",
  "selected",
  "disabled",
  "visible",
  "container",
  "row",
  "col",
  "wrapper",
  "content",
  "clearfix",
  "sr-only",
]);

// __smCandidateWorthy limits the scan to elements likely to be a nameable
// component — controls, landmarks, custom elements, or data-attr-bearing nodes —
// so pure layout wrappers don't flood the output.
function __smCandidateWorthy(el) {
  const tag = el.tagName.toLowerCase();
  if (el.hasAttribute("data-testid") || el.hasAttribute("data-component"))
    return true;
  if (tag.includes("-")) return true; // custom element
  if (el.hasAttribute("role")) return true;
  if (el.hasAttribute("aria-label")) return true;
  return ["a", "button", "input", "select", "textarea"].includes(tag);
}

// __smHrefSuffix mirrors coverage.hrefSuffix (Go): the portable, stable
// trailing path segment of an href, "" when the tail is dynamic/hashed.
function __smHrefSuffix(href) {
  let h = href;
  const cut = h.search(/[?#]/);
  if (cut >= 0) h = h.slice(0, cut);
  h = h.replace(/\/+$/, "");
  const slash = h.lastIndexOf("/");
  if (slash < 0) return "";
  const seg = h.slice(slash + 1);
  if (!seg || __smLooksHashed(seg)) return "";
  return "/" + seg;
}

// __smBestHook returns the single strongest stable selector for el, or "".
// A best-effort single pick, not the full ranked list coverage.SelectorCandidates
// (Go) returns — but the same hooks, roughly by the same priority: data-* leads
// but is one input, falling through to custom-element tag, stable id, form name,
// other stable data-*, design-system class, href, then aria-label.
function __smBestHook(el) {
  const tag = el.tagName.toLowerCase();
  const dt = el.getAttribute("data-testid");
  if (dt && !__smLooksHashed(dt)) return '[data-testid="' + dt + '"]';
  const dc = el.getAttribute("data-component");
  if (dc)
    return '[data-component^="' + dc.replace(/:v\d+\.\d+\.\d+.*$/, "") + '"]';
  if (tag.includes("-")) return tag; // custom element — stable and semantic
  const id = el.id;
  if (id && !__smLooksHashed(id) && !/\d+$/.test(id)) return "#" + id;
  const name = el.getAttribute("name");
  if (name && !__smLooksHashed(name)) return tag + '[name="' + name + '"]';
  for (const attr of el.attributes) {
    if (
      attr.name.startsWith("data-") &&
      attr.name !== "data-testid" &&
      attr.name !== "data-component" &&
      attr.value &&
      !__smLooksHashed(attr.value)
    ) {
      return "[" + attr.name + '="' + attr.value + '"]';
    }
  }
  let utility = "";
  for (const cls of el.classList) {
    if (!cls || __smLooksHashed(cls)) continue;
    if (__smUtilityClasses.has(cls)) {
      if (!utility) utility = tag + "." + cls;
      continue;
    }
    return tag + "." + cls; // first specific class wins
  }
  if (utility) return utility;
  const href = el.getAttribute("href");
  if (href) {
    const suf = __smHrefSuffix(href);
    if (suf) return 'a[href$="' + suf + '"]';
  }
  const al = el.getAttribute("aria-label");
  if (al && !__smLooksHashed(al) && al.length <= 40)
    return tag + '[aria-label="' + al + '"]';
  return "";
}

function __smScanCandidates(max) {
  const results = {};
  for (const el of __smDeepQueryAll(document, "*")) {
    if (!__smCandidateWorthy(el)) continue;
    const candidate = __smBestHook(el);
    if (!candidate) continue;
    const role = el.getAttribute("role") || el.tagName.toLowerCase();
    const tag = el.tagName.toLowerCase();
    const text = el.textContent.trim().replace(/\s+/g, " ").slice(0, 60);
    if (!results[candidate]) {
      const ancestorEl = el.closest("[data-sightmap-id]");
      const ancestorId = ancestorEl
        ? ancestorEl.getAttribute("data-sightmap-id")
        : "";
      results[candidate] = {
        sel: candidate,
        count: 0,
        role,
        tag,
        sample: text,
        ancestorId,
      };
    }
    results[candidate].count++;
  }
  return Object.values(results)
    .sort((a, b) => b.count - a.count)
    .slice(0, max);
}

function __smScanLinks() {
  const host = location.host;
  const seen = new Set();
  const links = [];
  for (const a of __smDeepQueryAll(document, "a[href]")) {
    try {
      const u = new URL(a.href);
      if (u.host === host && !seen.has(u.pathname)) {
        seen.add(u.pathname);
        links.push(u.pathname);
      }
    } catch (e) {}
  }
  return links;
}
