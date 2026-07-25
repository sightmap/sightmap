// Package authoring holds the live-DOM discovery scans used while building a
// corpus: a selector-candidate scan (behind `suggest`) and an internal-link
// scan plus URL-pattern normalization (behind `discover`). The bespoke
// browser-side JavaScript lives here, in one place, rather than inline in each
// command, so it cannot drift and can be reused by other tooling.
package authoring

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sightmap/sightmap/go/browser"
)

// Candidate is one selector candidate found by ScanCandidates: a stable
// data-attribute selector, how many elements it matched, and enough context
// (role/tag/sample text, nearest already-captured ancestor) to rank and group it.
type Candidate struct {
	Sel        string `json:"sel"`
	Count      int    `json:"count"`
	Role       string `json:"role"`
	Tag        string `json:"tag"`
	Sample     string `json:"sample"`
	AncestorId string `json:"ancestorId"`
}

// ScanCandidates runs a DOM pass over the live page and returns up to max
// data-testid / data-component selector candidates, most-frequent first. Each
// candidate records the nearest [data-sightmap-id] ancestor so callers can group
// candidates under the component that already covers their region.
func ScanCandidates(ctx context.Context, conn *browser.CDPConn, max int) ([]Candidate, error) {
	script := fmt.Sprintf(`(function(max) {
  const results = {};
  const selectors = [
    '[data-testid]',
    '[data-component]',
    '[aria-label][role]',
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
    const els = document.querySelectorAll(sel);
    for (const el of els) {
      let candidate = '';
      const dt = el.getAttribute('data-testid');
      const dc = el.getAttribute('data-component');
      const role = el.getAttribute('role') || el.tagName.toLowerCase();
      const tag = el.tagName.toLowerCase();
      const text = el.textContent.trim().replace(/\s+/g, ' ').slice(0, 60);
      if (dt) {
        candidate = '[data-testid="' + dt + '"]';
      } else if (dc) {
        const base = dc.replace(/:v\d+\.\d+\.\d+.*$/, '');
        candidate = '[data-component^="' + base + '"]';
      } else {
        continue;
      }
      if (!results[candidate]) {
        const ancestorEl = el.closest('[data-sightmap-id]');
        const ancestorId = ancestorEl ? ancestorEl.getAttribute('data-sightmap-id') : '';
        results[candidate] = {sel: candidate, count: 0, role: role, tag: tag, sample: text, ancestorId: ancestorId};
      }
      results[candidate].count++;
    }
  }
  return Object.values(results).sort((a,b) => b.count - a.count).slice(0, max);
})(%d)`, max)

	raw, err := browser.EvalJSON(ctx, conn, script)
	if err != nil {
		return nil, fmt.Errorf("scan candidates: eval: %v", err)
	}
	var candidates []Candidate
	if err := json.Unmarshal(raw, &candidates); err != nil {
		return nil, fmt.Errorf("scan candidates: parse result: %v", err)
	}
	return candidates, nil
}

// ScanLinks returns the distinct same-host link pathnames on the live page, in
// document order. Callers normalize these into route patterns with NormalizePath.
func ScanLinks(ctx context.Context, conn *browser.CDPConn) ([]string, error) {
	const linkScript = `(function() {
  const host = location.host;
  const seen = new Set();
  const links = [];
  for (const a of document.querySelectorAll('a[href]')) {
    try {
      const u = new URL(a.href);
      if (u.host === host && !seen.has(u.pathname)) {
        seen.add(u.pathname);
        links.push(u.pathname);
      }
    } catch(e) {}
  }
  return links;
})()`

	raw, err := browser.EvalJSON(ctx, conn, linkScript)
	if err != nil {
		return nil, fmt.Errorf("scan links: eval: %v", err)
	}
	var pathnames []string
	if err := json.Unmarshal(raw, &pathnames); err != nil {
		return nil, fmt.Errorf("scan links: parse result: %v", err)
	}
	return pathnames, nil
}
