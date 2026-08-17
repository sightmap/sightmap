// Live-DOM helpers for the `bounds` subcommand. __smBoundsBySelector depends
// on __smDeepQueryAll from browser/deepquery.js being prepended.

function __smBoundsBySelector(sel) {
  const els = __smDeepQueryAll(document, sel);
  const out = [];
  for (const el of els) {
    const r = el.getBoundingClientRect();
    let label = (el.getAttribute('aria-label') || el.textContent || '').trim().replace(/\s+/g, ' ');
    if (label.length > 80) label = label.slice(0, 80);
    out.push({x: r.left, y: r.top, width: r.width, height: r.height, label});
  }
  return out;
}

function __smViewportSize() {
  return {w: window.innerWidth, h: window.innerHeight};
}
