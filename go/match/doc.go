// Package match implements the NFA multi-query sightmap rule matcher.
// It traverses a comps.ComponentNode tree once in O(M) time, matching N
// sightmap component definitions simultaneously via sel.Matches.
package match
