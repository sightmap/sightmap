// Command sightmap is the unified sightmap authoring tool.
//
// Usage:
//
//	sightmap browser <subcommand>   manage the Chrome session
//	sightmap snapshot [flags]       observe: annotated component tree + coverage
//	sightmap capture [flags]        persist a capture into the matched view's set
//	sightmap sel-probe [flags] SEL  validate a CSS selector against the live page
//	sightmap validate [flags]       check sightmap YAML for structural errors
//	sightmap lint [flags]           check sightmap YAML for style issues
//	sightmap coverage [flags]       recompute T1/T2/T3 coverage from saved snap files
//	sightmap multi-coverage [flags] cross-page coverage matrix and promotion candidates
//	sightmap search [flags] PATTERN offline YAML content search with hierarchy context
//	sightmap discover [flags]       URL pattern discovery against the live page
//
// Run 'sightmap <command> --help' for per-command flags.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = runInit(args)
	case "browser":
		err = runBrowser(args)
	case "inspect":
		err = runInspect(args)
	case "snapshot":
		err = runSnapshot(args)
	case "capture":
		err = runCapture(args)
	case "sel-probe", "sel_probe":
		err = runSelProbe(args)
	case "validate":
		err = runValidate(args)
	case "lint":
		err = runLint(args)
	case "suggest":
		err = runSuggest(args)
	case "gap":
		err = runGap(args)
	case "coverage":
		err = runCoverage(args)
	case "multi-coverage", "multi_coverage":
		err = runMultiCoverage(args)
	case "capture-novelty", "capture_novelty":
		err = runCaptureNovelty(args)
	case "capture-prune", "capture_prune":
		err = runCapturePrune(args)
	case "search":
		err = runSearch(args)
	case "discover":
		err = runDiscover(args)
	case "serve-sightmap", "serve_sightmap":
		err = runServeSightmap(args)
	case "report":
		err = runReport(args)
	case "sel-check", "sel_check":
		err = runSelCheck(args)
	case "console":
		err = runConsole(args)
	case "network":
		err = runNetwork(args)
	case "skills":
		err = runSkills(args)
	case "version", "--version", "-v":
		fmt.Printf("sightmap version %s\n", Version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "sightmap: unknown command %q\n\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		// A subcommand's --help/-h parses to flag.ErrHelp; the flag package has
		// already printed its usage, so a help request is success, not an error.
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "sightmap %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `sightmap — sightmap authoring toolkit

Commands:
  browser start [--headless] [--port N] [--url URL]   start Chrome session + sightmap server
  browser stop / status / navigate / eval             session management
  browser click / fill / hover / keypress / scroll     interact with page elements
  browser drag / wait-for / dialog                     more interactions
  browser tabs list/new/close/resize                   tab management

  report        [--sightmap-dir DIR]                              per-view coverage health table + T2 quality
  snapshot       [--url URL] [--coverage] [--out FILE] [--sightmap-dir DIR]  observe: annotated component tree + coverage
  capture        [--url URL] [--all] [--force] [--sightmap-dir DIR]  persist a capture into the matched view's set
  inspect        [--url URL] [--out FILE] [--sightmap-dir DIR]  raw DOM tree for selector authoring
  coverage       [--sightmap-dir DIR] [FILE.snap ...]            offline T1/T2/T3 check
  multi-coverage [--sightmap-dir DIR] [FILE.snap ...]            cross-page coverage matrix
  capture-novelty [--sightmap-dir DIR] FILE.snap               does a capture add new components/slots vs its view set?
  capture-prune [--dry-run] (<view> | --all)                   drop captures subsumed by the rest of their view set
  suggest        [--sightmap-dir DIR] [--max N] [--exclude-known]  DOM selector candidates
  gap            [--sightmap-dir DIR] [--url URL]                   orphaned interactive nodes
  sel-probe [flags] 'selector'  [--all]                         selector validator (live)
  sel-check 'selector' FILE.snap                                offline selector validator
  validate [--sightmap-dir DIR]                              YAML structural check
  lint     [--sightmap-dir DIR]                              YAML style check
  search   [--field FIELD] PATTERN                           offline YAML content search
  discover [--all]                                           URL pattern discovery
  serve-sightmap [--port N] [--sightmap-dir DIR]             sightmap HTTP server for overlay extension

  console  list [--level L] [--tab T] [--limit N]            captured console messages (needs a running session)
  console  get INDEX                                         one console message by index
  network  list [--type T] [--url SUBSTR] [--tab T] [--limit N]  captured network requests
  network  get INDEX [--response-file F] [--request-file F]  one request + body by index

  init     [--sightmap-dir DIR]                             scaffold a schema-correct .sightmap/ corpus
  skills install [--target DIR]                             install sightmap authoring skill to ~/.agents/skills/
  version                                                    print version and exit

Run 'sightmap <command> --help' for full flag list.
`)
}
