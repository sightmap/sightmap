package browser

// DeepQueryJS defines shadow-piercing querySelector helpers, prepended to any
// browser-eval that must locate a node the way the offline matcher does: across
// shadow boundaries.
//
// Why this exists: capture is shadow-aware — probe.js walks node.shadowRoot and
// stamps data-sightmap-id on live shadow nodes, and the extracted tree flattens
// shadow subtrees in as ordinary children. So offline matching (and sel-probe's
// offline count, coverage, component queries) matches ACROSS shadow boundaries.
// A live document.querySelector does NOT cross shadow roots, so every live
// lookup that re-finds a matched node (property extraction, interaction,
// bounds, the sel-probe live count) was silently shadow-blind and disagreed
// with the corpus. These helpers close that gap. See schema.md "Selector model
// & shadow DOM".
//
//	__smDeepQueryAll(root, sel) — every element matching sel within root's tree
//	  AND every shadow tree nested beneath it. root may be the document, a
//	  shadow root, or an element; for an element the matches are its descendants
//	  (mirroring Element.querySelectorAll) plus descendants inside its own or any
//	  nested shadow root.
//	__smDeepQuery(root, sel) — the first such element, or null.
//
// Prepend with: browser.DeepQueryJS + "\n" + script. The helpers are function
// declarations, so the script's trailing expression stays the completion value.
const DeepQueryJS = `
function __smDeepQueryAll(root, sel) {
  var out = [];
  function walk(r) {
    try {
      var m = r.querySelectorAll(sel);
      for (var i = 0; i < m.length; i++) out.push(m[i]);
    } catch (e) { return; }
    if (r.shadowRoot) walk(r.shadowRoot);
    var all = r.querySelectorAll('*');
    for (var j = 0; j < all.length; j++) {
      if (all[j].shadowRoot) walk(all[j].shadowRoot);
    }
  }
  walk(root);
  return out;
}
function __smDeepQuery(root, sel) {
  var a = __smDeepQueryAll(root, sel);
  return a.length ? a[0] : null;
}
`
