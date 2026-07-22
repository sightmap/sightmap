// Package skills embeds the sightmap skill files so they can be extracted by
// the binary without a repo checkout.
package skills

import "embed"

// FS contains the embedded skill directories (one per top-level directory).
//
//go:embed sightmap-authoring sightmap-browser
var FS embed.FS
