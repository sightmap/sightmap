// Command dumpir prints compiled WebMCP IR as JSON for a tools manifest.
// Used by manual live-verification scripts. Run from go/:
// `go run ./webmcp/internal/dumpir PATH/webmcp.tools.yaml`.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sightmap/sightmap/go/webmcp"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: dumpir PATH/webmcp.tools.yaml")
		os.Exit(2)
	}
	toolsFile, err := filepath.Abs(os.Args[1])
	if err != nil {
		fatal(err)
	}
	manifest, errs, _, err := webmcp.LoadManifest(toolsFile)
	if err != nil {
		fatal(err)
	}
	if len(errs) > 0 {
		fatal(fmt.Errorf("%s", strings.Join(errs, "; ")))
	}
	dir := filepath.Dir(toolsFile)
	corpusDir := webmcp.ManifestSightmapDir(manifest)
	if corpusDir == "" {
		corpusDir = ".sightmap"
	}
	if !filepath.IsAbs(corpusDir) {
		corpusDir = filepath.Join(dir, corpusDir)
	}
	corpus, err := webmcp.LoadCorpus(corpusDir)
	if err != nil {
		fatal(err)
	}
	ir, cerrs, _ := webmcp.Compile(corpus, manifest)
	if len(cerrs) > 0 {
		fatal(fmt.Errorf("%s", strings.Join(cerrs, "; ")))
	}
	fmt.Print(webmcp.StringifyJSON(ir))
	fmt.Print("\n")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "dumpir:", err)
	os.Exit(1)
}
