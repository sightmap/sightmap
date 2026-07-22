package browser

import (
	"context"
	"fmt"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/extract"
)

// Page is the minimal interface for component extraction from a live page.
// All methods map 1:1 to CDP commands. The CDPConn adapter implements this
// interface; other consumers can provide their own adapter (e.g. Playwright).
type Page interface {
	// RunProbe executes cdp-probe.js in the page context and returns the
	// structured probe output. isLogicalRoot should be true when running
	// inside a logical root frame (replay). useScrollOffset adds the current
	// scroll position to element coordinates.
	RunProbe(ctx context.Context, isLogicalRoot, useScrollOffset bool) (*extract.ProbeResult, error)

	// GetA11YTree returns the full accessibility tree for the page's main frame.
	GetA11YTree(ctx context.Context) ([]extract.A11YNode, error)

	// GetDOMTree returns the CDP DOM document tree, traversing shadow roots
	// and iframes (pierce=true, depth=-1).
	GetDOMTree(ctx context.Context) (*extract.DOMNode, error)

	// URL returns the current page URL.
	URL() string
}

// ExtractComponents runs the full extraction pipeline on page:
// RunProbe → GetA11YTree → GetDOMTree → extract.BuildTree.
func ExtractComponents(ctx context.Context, page Page) (*comps.ComponentNode, error) {
	probeResult, err := page.RunProbe(ctx, false, false)
	if err != nil {
		return nil, fmt.Errorf("browser: run probe: %w", err)
	}
	if !probeResult.Success {
		return nil, fmt.Errorf("browser: probe failed: %s", probeResult.Error)
	}

	a11y, err := page.GetA11YTree(ctx)
	if err != nil {
		return nil, fmt.Errorf("browser: get a11y tree: %w", err)
	}

	dom, err := page.GetDOMTree(ctx)
	if err != nil {
		return nil, fmt.Errorf("browser: get dom tree: %w", err)
	}

	return extract.BuildTree(probeResult, a11y, dom)
}
