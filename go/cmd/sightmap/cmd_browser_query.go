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
// sightmap, projects the matches' resolved properties, and resolves the query
// to a single node. Everything happens within one extraction, so the returned
// node's bounds are fresh and no id crosses a call boundary — the atomic
// resolve+act that fixes the dynamic-page race.
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

	return compquery.Resolve(root, matches, queryPropertyValues(matches), q)
}

// queryPropertyValues projects each match's resolved properties (populated
// offline by the matcher, SEP-0010) into the node-id → (name → value) map the
// component-query engine consumes. Shared by resolveComponentQuery and
// prepareBoundsQuery.
func queryPropertyValues(
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
) map[string]map[string]string {
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
