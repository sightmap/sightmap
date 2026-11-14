package browser

import (
	"context"
	"fmt"
	"github.com/sightmap/sightmap/go/sightmap"

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
// RunProbe → GetA11YTree → GetDOMTree → extract.BuildTree. When page can read
// the frames nested in it (the CDP adapter can), each frame's tree hangs under
// its <iframe> node; see FrameOptions for how they are treated.
func ExtractComponents(ctx context.Context, page Page) (*sightmap.ComponentNode, error) {
	root, _, err := ExtractComponentsNotes(ctx, page)
	return root, err
}

// ExtractComponentsNotes is ExtractComponents that also reports what it left
// out of the frames it descended into: a frame cut at its node cap, frames
// past the frame cap, a frame that could not be read. An agent reading the
// snapshot needs to know the tree is partial there.
func ExtractComponentsNotes(ctx context.Context, page Page) (*sightmap.ComponentNode, []string, error) {
	probeResult, err := page.RunProbe(ctx, false, false)
	if err != nil {
		return nil, nil, fmt.Errorf("browser: run probe: %w", err)
	}
	if !probeResult.Success {
		return nil, nil, fmt.Errorf("browser: probe failed: %s", probeResult.Error)
	}

	// Frames are probed before the top DOM is read: it shows the ids of the
	// frames of its own process only if they are already stamped.
	var fs *frameSet
	fp, framed := page.(framedPage)
	var notes []string
	if framed {
		var ferr error
		if fs, ferr = fp.probeFrames(ctx, probeResult); ferr != nil {
			notes = append(notes, fmt.Sprintf("frames could not be read: %v", ferr))
		}
	}

	a11y, err := page.GetA11YTree(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("browser: get a11y tree: %w", err)
	}

	dom, err := page.GetDOMTree(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("browser: get dom tree: %w", err)
	}

	root, err := extract.BuildTree(probeResult, a11y, dom)
	if err != nil {
		return nil, nil, err
	}
	if fs != nil {
		fp.attachFrames(ctx, fs, dom, root)
		notes = append(notes, fs.notes...)
	}
	return root, notes, nil
}
