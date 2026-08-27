// Component-query resolution for the interaction subcommands. Lets click/fill/
// hover/scroll target an element by its sightmap identity (component name +
// extracted properties + descendant chain) instead of an ephemeral probe id.
package main

import (
	"context"
	"fmt"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/compquery"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sightmap"
)

// looksLikeProbeID reports whether arg is a probe id (digits, optionally with
// frame prefixes like "1_5") rather than a component query. Component queries
// always contain a letter or '[', so this cleanly disambiguates the two.
func looksLikeProbeID(arg string) bool {
	if arg == "" {
		return false
	}
	for i := 0; i < len(arg); i++ {
		c := arg[i]
		if (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// resolveTarget resolves an interaction target that may be either a probe id or
// a component query, returning a node with current bounds ready to act on.
func resolveTarget(ctx context.Context, conn *browser.CDPConn, sightmapDir, arg string) (*sightmap.ComponentNode, error) {
	if looksLikeProbeID(arg) {
		return resolveNode(ctx, conn, arg)
	}
	return resolveComponentQuery(ctx, conn, sightmapDir, arg)
}

// resolveComponentQuery extracts the live component tree once, applies the
// sightmap, extracts properties for the queried component names, and resolves
// the query to a single node. Everything happens within one extraction, so the
// returned node's bounds are fresh and no id crosses a call boundary — the
// atomic resolve+act that fixes the dynamic-page race.
func resolveComponentQuery(ctx context.Context, conn *browser.CDPConn, sightmapDir, queryStr string) (*sightmap.ComponentNode, error) {
	q, err := compquery.ParseQuery(queryStr)
	if err != nil {
		return nil, err
	}

	page, err := conn.DefaultPage()
	if err != nil {
		return nil, fmt.Errorf("resolve query: %w", err)
	}
	root, err := browser.ExtractComponents(ctx, page)
	if err != nil {
		return nil, fmt.Errorf("resolve query: extract: %w", err)
	}
	pageURL, err := browser.GetURL(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("resolve query: get URL: %w", err)
	}

	corpus, err := sightmap.Load(sightmapDir)
	if err != nil {
		return nil, fmt.Errorf("resolve query: load corpus: %w", err)
	}
	matcher := match.NewMatcher(corpus)
	matches := matcher.Match(root, pageURL)
	if len(matches) == 0 {
		return nil, fmt.Errorf(
			"resolve query: no sightmap components matched the page (need a corpus in %s)", sightmapDir)
	}

	props := extractQueryProperties(ctx, conn, matcher, matches, pageURL, q)
	return compquery.Resolve(root, matches, props, q)
}

// extractQueryProperties extracts live properties for every node whose matched
// component name is referenced by one of the given queries — keeps the live
// extraction small and avoids the per-snapshot node cap dropping a candidate —
// then projects them into the node-id → (name → value) map the component-query
// engine consumes. Shared by resolveComponentQuery (single query) and
// prepareBoundsQuery (one or more queries).
func extractQueryProperties(
	ctx context.Context,
	conn *browser.CDPConn,
	matcher *match.Matcher,
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	pageURL string,
	queries ...*compquery.Query,
) map[string]map[string]string {
	queryNames := make(map[string]bool)
	for _, q := range queries {
		for _, part := range q.Parts {
			queryNames[part.Name] = true
		}
	}
	relevant := make(map[*sightmap.ComponentNode]*sightmap.ComponentMatch)
	for n, m := range matches {
		if queryNames[m.Name] {
			relevant[n] = m
		}
	}
	components := matcher.Components(pageURL)
	compByName := make(map[string]sightmap.ComponentDef, len(components))
	for _, c := range components {
		compByName[c.Name] = c
	}
	observe.ExtractProperties(ctx, conn, relevant, compByName)

	out := make(map[string]map[string]string)
	for node, m := range matches {
		if m == nil || len(m.Properties) == 0 {
			continue
		}
		pm := make(map[string]string, len(m.Properties))
		for _, pv := range m.Properties {
			pm[pv.Name] = pv.Value
		}
		out[node.Id] = pm
	}
	return out
}
