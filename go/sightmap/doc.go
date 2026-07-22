// Package sightmap loads .sightmap/ YAML corpus files, flattens the
// component hierarchy into NFA-ready queries, and caches compiled
// MatchQuery slices in a thread-safe Session.
package sightmap
