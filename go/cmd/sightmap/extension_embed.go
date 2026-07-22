package main

import "embed"

// extensionFS holds the Sightmap Overlay Chrome extension, embedded at build
// time so the binary is self-contained. browser start extracts it to
// ~/.sightmap/extension/ automatically when no --extensions flag is given.
//
//go:embed extension
var extensionFS embed.FS
