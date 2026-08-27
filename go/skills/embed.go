// Package skills embeds the sightmap skill files so they can be extracted by
// the binary (`sightmap skills install`) without a repo checkout.
//
// The canonical skill corpora live at the repository root (<repo>/skills/) so
// they present as a first-class, standalone skills/plugin directory. Because
// go:embed cannot reach outside the Go module (rooted at <repo>/go) or follow
// symlinks, the directories below are committed copies regenerated from the
// canonical source. Do not edit them by hand — edit <repo>/skills/<name>/ and
// run `go generate ./skills/...` from the go/ directory.
package skills

import "embed"

//go:generate go run ./internal/gen

// FS contains the embedded skill directories (one per top-level directory).
//
//go:embed sightmap-authoring sightmap-browser sightmap-webmcp
var FS embed.FS
