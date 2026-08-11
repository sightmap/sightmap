// Package match implements the NFA multi-query sightmap rule matcher.
// It traverses a sightmap.ComponentNode tree once in O(M) time, matching N
// sightmap component definitions simultaneously via sightmap.Matches.
package match
