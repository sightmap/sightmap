// Canonical property extractor — mirrored in cmd/sightmap/extension/{content,resolver}.js
// and (transforms only) sightmap/property.go. Depends on __smDeepQuery from
// browser/deepquery.js being prepended.

function __smExtractProperties(specs) {
  // Returns the RAW value; the caller normalizes whitespace, applies the
  // transform, and caps length uniformly.
  function extractValue(el, extract) {
    if (!extract) return null;
    if (extract === 'text') return el.textContent;
    if (extract === 'inner_text') return el.innerText;
    if (extract === 'text_only') {
      const clone = el.cloneNode(true);
      clone.querySelectorAll('img,svg,[alt]').forEach(e => e.remove());
      return clone.textContent;
    }
    if (extract === 'inner_html') return el.innerHTML;
    if (extract.startsWith('attr=')) return el.getAttribute(extract.slice(5));
    if (extract.startsWith('exists:')) {
      return __smDeepQuery(el, extract.slice(7)) ? 'true' : null;
    }
    const sub = __smDeepQuery(el, extract);
    return sub ? (sub.innerText != null ? sub.innerText : sub.textContent) : null;
  }
  function applyTransform(val, transform) {
    if (!transform || !val) return val;
    if (transform.indexOf('match:') === 0) {
      try {
        const m = val.match(new RegExp(transform.slice(6)));
        if (!m) return val;
        return m[1] != null ? m[1] : m[0];
      } catch (e) { return val; }
    }
    const words = val.trim().split(/\s+/);
    switch (transform) {
      case 'first_word': return words[0] || val;
      case 'last_word': return words[words.length - 1] || val;
      case 'first_number': { const m = val.match(/\d[\d,.]*/); return m ? m[0] : val; }
      case 'first_dollar': { const m = val.match(/\$[\d,.]+/); return m ? m[0] : val; }
      case 'number': return val.replace(/[^\d.]/g, '');
      case 'slug': return val.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-]/g, '');
      default: return val;
    }
  }
  const results = {};
  for (const {id, selector, props} of specs) {
    // Anchor to the exact matched element via its sightmap ID attribute (set by
    // probe.js as data-sightmap-id). This ensures child components like
    // BreadcrumbLink get their own text, not the first match on the page.
    const el = (id ? __smDeepQuery(document, '[data-sightmap-id="' + id + '"]') : null)
               || __smDeepQuery(document, selector);
    if (!el) continue;
    const vals = {};
    for (const {name, extract, transform} of props) {
      let val = extractValue(el, extract);
      if (val == null) continue;
      val = String(val).trim().replace(/\s+/g, ' ');
      if (val === '') continue;
      val = applyTransform(val, transform);
      if (val) vals[name] = String(val).slice(0, 120);
    }
    if (Object.keys(vals).length > 0) results[id] = vals;
  }
  return results;
}
