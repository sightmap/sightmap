package observe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
)

// ExtractProperties runs a single batched JS evaluation against the live DOM
// to extract property values for all matched nodes that have property
// definitions. Returns a map from nodeID to {propName → value}.
// At most 200 nodes are evaluated to avoid JS timeout.
func ExtractProperties(
	ctx context.Context,
	conn *browser.CDPConn,
	matches map[*comps.ComponentNode]*match.ComponentMatch,
	compByName map[string]match.ComponentDef,
) map[string]map[string]string {
	type specProp struct {
		Name      string `json:"name"`
		Extract   string `json:"extract"`
		Transform string `json:"transform"`
	}
	type spec struct {
		ID       string     `json:"id"`
		Selector string     `json:"selector"`
		Props    []specProp `json:"props"`
	}

	var specs []spec
	const maxNodes = 200
	for node, m := range matches {
		if len(specs) >= maxNodes {
			break
		}
		comp, ok := compByName[m.Name]
		if !ok || len(comp.Properties) == 0 || len(comp.Selectors) == 0 {
			continue
		}
		sp := spec{
			ID:       node.Id,
			Selector: comp.Selectors[0],
			Props:    make([]specProp, len(comp.Properties)),
		}
		for i, p := range comp.Properties {
			sp.Props[i] = specProp{Name: p.Name, Extract: p.Extract, Transform: p.Transform}
		}
		specs = append(specs, sp)
	}
	if len(specs) == 0 {
		return nil
	}

	specsJSON, err := json.Marshal(specs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "observe: marshal property specs: %v\n", err)
		return nil
	}

	const jsTemplate = `(function(specs) {
  // Canonical extractor — mirrored in cmd/sightmap/extension/{content,resolver}.js
  // and (transforms only) sightmap/property.go. Returns the RAW value; the caller
  // normalizes whitespace, applies the transform, and caps length uniformly.
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
      return el.querySelector(extract.slice(7)) ? 'true' : null;
    }
    const sub = el.querySelector(extract);
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
    switch(transform) {
      case 'first_word': return words[0] || val;
      case 'last_word':  return words[words.length-1] || val;
      case 'first_number': { const m = val.match(/\d[\d,.]*/); return m ? m[0] : val; }
      case 'first_dollar': { const m = val.match(/\$[\d,.]+/); return m ? m[0] : val; }
      case 'number':     return val.replace(/[^\d.]/g, '');
      case 'slug':       return val.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-]/g, '');
      default: return val;
    }
  }
  const results = {};
  for (const {id, selector, props} of specs) {
    // Anchor to the exact matched element via its sightmap ID attribute (set by
    // probe.js as data-sightmap-id). This ensures child components like
    // BreadcrumbLink get their own text, not the first match on the page.
    const el = (id ? document.querySelector('[data-sightmap-id="' + id + '"]') : null)
               || document.querySelector(selector);
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
})(SPECS_JSON)`

	script := strings.Replace(jsTemplate, "SPECS_JSON", string(specsJSON), 1)

	resultJSON, evalErr := browser.EvalJSON(ctx, conn, script)
	if evalErr != nil {
		fmt.Fprintf(os.Stderr, "observe: property extraction: %v\n", evalErr)
		return nil
	}

	var propValues map[string]map[string]string
	if err := json.Unmarshal(resultJSON, &propValues); err != nil {
		fmt.Fprintf(os.Stderr, "observe: property extraction unmarshal: %v\n", err)
		return nil
	}
	return propValues
}
