// Package authoring holds the live-DOM discovery scans used while building a
// corpus: a selector-candidate scan (behind `suggest`) and an internal-link
// scan plus URL-pattern normalization (behind `discover`). The bespoke
// browser-side JavaScript lives here, in one place, rather than inline in each
// command, so it cannot drift and can be reused by other tooling.
package authoring

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/sightmap/sightmap/go/browser"
)

//go:embed scan.js
var scanJS string

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
	script := browser.DeepQueryJS + scanJS + fmt.Sprintf("\n__smScanCandidates(%d)", max)

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
	script := browser.DeepQueryJS + scanJS + "\n__smScanLinks()"

	raw, err := browser.EvalJSON(ctx, conn, script)
	if err != nil {
		return nil, fmt.Errorf("scan links: eval: %v", err)
	}
	var pathnames []string
	if err := json.Unmarshal(raw, &pathnames); err != nil {
		return nil, fmt.Errorf("scan links: parse result: %v", err)
	}
	return pathnames, nil
}
