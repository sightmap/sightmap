// Live-DOM discovery scans used while building a corpus. Depends on
// __smDeepQueryAll from browser/deepquery.js being prepended.

function __smScanCandidates(max) {
  const results = {};
  const selectors = [
    "[data-testid]",
    "[data-component]",
    "[aria-label][role]",
    '[role="navigation"]',
    '[role="search"]',
    '[role="banner"]',
    '[role="main"]',
    '[role="complementary"]',
    '[role="contentinfo"]',
    '[role="dialog"]',
    '[role="alertdialog"]',
    '[role="tablist"]',
    '[role="tab"]',
    '[role="tabpanel"]',
  ];
  for (const sel of selectors) {
    const els = __smDeepQueryAll(document, sel);
    for (const el of els) {
      let candidate = "";
      const dt = el.getAttribute("data-testid");
      const dc = el.getAttribute("data-component");
      const role = el.getAttribute("role") || el.tagName.toLowerCase();
      const tag = el.tagName.toLowerCase();
      const text = el.textContent.trim().replace(/\s+/g, " ").slice(0, 60);
      if (dt) {
        candidate = '[data-testid="' + dt + '"]';
      } else if (dc) {
        const base = dc.replace(/:v\d+\.\d+\.\d+.*$/, "");
        candidate = '[data-component^="' + base + '"]';
      } else {
        continue;
      }
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
