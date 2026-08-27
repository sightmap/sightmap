// webmcp generates WebMCP tool bundles (document.modelContext) from a
// sightmap corpus + a webmcp.tools.yaml manifest. The compiler, emitter, and
// embedded browser runtime live in the webmcp package; this file is flags in,
// files out — mirroring the standalone Node CLI at <repo>/webmcp/ flag for
// flag, and producing byte-identical bundles (the webmcp package's golden
// tests enforce that).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sightmap/sightmap/go/webmcp"
)

func runWebmcp(args []string) error {
	if len(args) == 0 {
		webmcpUsage()
		return nil
	}
	switch args[0] {
	case "validate":
		return runWebmcpValidate(args[1:])
	case "generate":
		return runWebmcpGenerate(args[1:])
	case "init":
		return runWebmcpInit(args[1:])
	case "help", "--help", "-h":
		webmcpUsage()
		return nil
	default:
		webmcpUsage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func webmcpUsage() {
	fmt.Fprint(os.Stderr, `sightmap webmcp — generate WebMCP tools (document.modelContext) from a corpus

  validate [--tools FILE] [--sightmap-dir DIR]
  generate [--tools FILE] [--sightmap-dir DIR] [--format snippet|module|userscript|all]
           [--out FILE | --out-dir DIR] [--check]
  init     --site SLUG --base-url URL [--sightmap-dir DIR] [--out FILE]

The tools manifest defaults to ./webmcp.tools.yaml; the corpus defaults to the
manifest's "sightmap:" field (relative to the manifest), then to a .sightmap/
directory next to the manifest. See the sightmap-webmcp skill for the
authoring loop.
`)
}

// resolveWebmcpInputs loads and validates the manifest, then the corpus.
func resolveWebmcpInputs(toolsFlag, dirFlag string) (toolsFile string, manifest any, corpus *webmcp.Corpus, err error) {
	toolsFile, err = filepath.Abs(toolsFlag)
	if err != nil {
		return "", nil, nil, err
	}
	if _, statErr := os.Stat(toolsFile); statErr != nil {
		return "", nil, nil, fmt.Errorf("tools manifest not found: %s (pass --tools, or run \"sightmap webmcp init\" to scaffold one)", toolsFile)
	}
	manifest, errs, warns, err := webmcp.LoadManifest(toolsFile)
	if err != nil {
		return "", nil, nil, err
	}
	for _, w := range warns {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "error: %s\n", e)
		}
		return "", nil, nil, fmt.Errorf("%d manifest error(s)", len(errs))
	}
	var sightmapDir string
	switch {
	case dirFlag != "":
		sightmapDir, err = filepath.Abs(dirFlag)
	case webmcp.ManifestSightmapDir(manifest) != "":
		sightmapDir = filepath.Join(filepath.Dir(toolsFile), webmcp.ManifestSightmapDir(manifest))
	default:
		sightmapDir = filepath.Join(filepath.Dir(toolsFile), ".sightmap")
	}
	if err != nil {
		return "", nil, nil, err
	}
	corpus, err = webmcp.LoadCorpus(sightmapDir)
	if err != nil {
		return "", nil, nil, err
	}
	return toolsFile, manifest, corpus, nil
}

func compileWebmcp(manifest any, corpus *webmcp.Corpus) (*webmcp.OM, error) {
	ir, errs, warns := webmcp.Compile(corpus, manifest)
	for _, w := range warns {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "error: %s\n", e)
		}
		return nil, fmt.Errorf("%d compile error(s)", len(errs))
	}
	return ir, nil
}

func runWebmcpValidate(args []string) error {
	fs := flag.NewFlagSet("webmcp validate", flag.ContinueOnError)
	tools := fs.String("tools", "webmcp.tools.yaml", "Tool manifest to validate")
	dir := fs.String("sightmap-dir", "", "Corpus directory (default: manifest's sightmap: field, else .sightmap next to it)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, manifest, corpus, err := resolveWebmcpInputs(*tools, *dir)
	if err != nil {
		return err
	}
	ir, err := compileWebmcp(manifest, corpus)
	if err != nil {
		return err
	}
	fmt.Printf("✓ %d tool(s) compile against %d corpus file(s): %s\n",
		len(webmcp.ToolNames(ir)), len(corpus.Files), joinNames(webmcp.ToolNames(ir)))
	return nil
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

func runWebmcpGenerate(args []string) error {
	fs := flag.NewFlagSet("webmcp generate", flag.ContinueOnError)
	tools := fs.String("tools", "webmcp.tools.yaml", "Tool manifest to compile")
	dir := fs.String("sightmap-dir", "", "Corpus directory (default: manifest's sightmap: field, else .sightmap next to it)")
	format := fs.String("format", "snippet", "Output format: snippet|module|userscript|all")
	out := fs.String("out", "", "Output file (single format only)")
	outDir := fs.String("out-dir", "", "Output directory (default: the manifest's directory)")
	check := fs.Bool("check", false, "Compare against existing output files; exit 2 on drift, write nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	toolsFile, manifest, corpus, err := resolveWebmcpInputs(*tools, *dir)
	if err != nil {
		return err
	}
	ir, err := compileWebmcp(manifest, corpus)
	if err != nil {
		return err
	}

	formats := []string{*format}
	if *format == "all" {
		formats = webmcp.Formats
	}
	for _, f := range formats {
		ok := false
		for _, known := range webmcp.Formats {
			if f == known {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("unknown format %q (snippet|module|userscript|all)", f)
		}
	}
	if *out != "" && len(formats) > 1 {
		return fmt.Errorf("--out only works with a single --format; use --out-dir for several")
	}
	baseDir := filepath.Dir(toolsFile)
	if *outDir != "" {
		baseDir, err = filepath.Abs(*outDir)
		if err != nil {
			return err
		}
	} else if *out != "" {
		abs, err := filepath.Abs(*out)
		if err != nil {
			return err
		}
		baseDir = filepath.Dir(abs)
	}

	hash, err := webmcp.CorpusHash(corpus.Dir, corpus.Files)
	if err != nil {
		return err
	}
	site := webmcp.SiteOf(ir)
	drift := false
	for _, f := range formats {
		outFile := filepath.Join(baseDir, webmcp.DefaultFileName(site, f))
		if *out != "" {
			outFile, _ = filepath.Abs(*out)
		}
		manifestRel, _ := filepath.Rel(filepath.Dir(outFile), toolsFile)
		if manifestRel == "" {
			manifestRel = filepath.Base(toolsFile)
		}
		corpusRel, _ := filepath.Rel(filepath.Dir(outFile), corpus.Dir)
		content, err := webmcp.Emit(ir, f, webmcp.Provenance{
			GeneratorVersion: webmcp.GeneratorVersion,
			Manifest:         manifestRel,
			Corpus:           corpusRel,
			CorpusFiles:      len(corpus.Files),
			CorpusHash:       hash,
		})
		if err != nil {
			return err
		}
		if *check {
			existing, readErr := os.ReadFile(outFile)
			if readErr != nil || string(existing) != content {
				fmt.Fprintf(os.Stderr, "drift: %s is stale — regenerate with sightmap webmcp generate\n", outFile)
				drift = true
			} else {
				fmt.Printf("✓ %s is up to date\n", outFile)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(outFile), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(outFile, []byte(content), 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s (%d bytes, %d tools)\n", outFile, len(content), len(webmcp.ToolNames(ir)))
		}
	}
	if drift {
		os.Exit(2)
	}
	return nil
}

func runWebmcpInit(args []string) error {
	fs := flag.NewFlagSet("webmcp init", flag.ContinueOnError)
	site := fs.String("site", "", "Site slug for the manifest (required)")
	baseURL := fs.String("base-url", "", "Site base URL (required)")
	dir := fs.String("sightmap-dir", ".sightmap", "Corpus directory to scaffold from")
	out := fs.String("out", "webmcp.tools.yaml", "Output manifest path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *site == "" || *baseURL == "" {
		return fmt.Errorf("init needs --site SLUG and --base-url URL")
	}
	corpus, err := webmcp.LoadCorpus(*dir)
	if err != nil {
		return err
	}
	outFile, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(outFile); statErr == nil {
		return fmt.Errorf("%s already exists — delete it first if you meant to re-scaffold", outFile)
	}
	content, err := webmcp.Scaffold(corpus, *site, *baseURL)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outFile, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s — a draft; see the sightmap-webmcp skill for the authoring loop\n", outFile)
	return nil
}
