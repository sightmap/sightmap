package observe

import (
	"context"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// Result is one observation of a live page: the extracted component tree, the
// corpus matches applied to it, the view it resolved to, and the coverage score.
// Props holds live-extracted property values (nodeID → propName → value) and is
// nil unless Options.ExtractProps was set. Matches/View/Coverage are zero when no
// corpus was supplied.
type Result struct {
	Root        *comps.ComponentNode
	URL         string
	Matches     map[*comps.ComponentNode]*match.ComponentMatch
	View        *sightmap.View
	Components  []match.ComponentDef
	GlobalNames map[string]bool
	Props       map[string]map[string]string
	Coverage    coverage.Result

	// CorpusApplied is true when a corpus was supplied and matched against the
	// tree, even if no view matched the URL or no component matched a node. It
	// lets the renderer tell "no corpus" apart from "corpus loaded but nothing
	// matched" — the latter must not be silent.
	CorpusApplied bool

	// TiedViews holds the names of views that matched the URL at equal top
	// specificity (>= 2 means the winner was decided only by declaration order).
	// Empty when there's no ambiguity.
	TiedViews []string
	// ComponentConflicts holds DOM nodes claimed by more than one distinct
	// component name (first-match-wins keeps only one; the rest are dropped).
	ComponentConflicts []match.Conflict
}

// Options controls how a page is observed.
type Options struct {
	// VisibleOnly scores coverage over visible interactive nodes only (the CLI
	// default); false counts hidden/off-screen nodes too.
	VisibleOnly bool
	// ExtractProps runs a live DOM pass to pull component property values. Skip
	// it when only the tree/coverage are needed (e.g. bulk capture).
	ExtractProps bool
}

// Page observes the page currently loaded in conn: it extracts the component
// tree, applies corpus (when non-nil) to produce matches, the view, the merged
// component list, and coverage, and optionally extracts property values.
// Navigation and the browser lifecycle are the caller's responsibility — Page
// only reads the current page state.
func Page(ctx context.Context, conn *browser.CDPConn, corpus *sightmap.Corpus, opts Options) (*Result, error) {
	page, err := conn.DefaultPage()
	if err != nil {
		return nil, err
	}
	root, err := browser.ExtractComponents(ctx, page)
	if err != nil {
		return nil, err
	}
	url, err := browser.GetURL(ctx, conn)
	if err != nil {
		return nil, err
	}

	res := &Result{Root: root, URL: url}
	if corpus == nil {
		return res, nil
	}
	res.CorpusApplied = true

	res.Matches = corpus.MatchTree(root, url)
	res.View = corpus.ViewForURL(url)
	res.Components = corpus.Components(url)
	res.GlobalNames = corpus.GlobalComponentNames()
	res.Coverage = coverage.Score(root, res.Matches, coverage.Options{VisibleOnly: opts.VisibleOnly})

	// Runtime conflicts: ambiguities only visible against this page. TiedViews
	// flags >=2 views matching the URL at equal specificity; ComponentConflicts
	// flags a single node claimed by multiple distinct component names.
	res.TiedViews = corpus.TiedViews(url)
	res.ComponentConflicts = match.FindConflicts(root, res.Components)

	if opts.ExtractProps && len(res.Matches) > 0 && len(res.Components) > 0 {
		compByName := make(map[string]match.ComponentDef, len(res.Components))
		for _, c := range res.Components {
			compByName[c.Name] = c
		}
		res.Props = ExtractProperties(ctx, conn, res.Matches, compByName)
	}
	return res, nil
}
