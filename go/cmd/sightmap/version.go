package main

// Version is set at build time by goreleaser via -X main.Version=<tag>.
// Falls back to "dev" for local builds.
var Version = "dev"
