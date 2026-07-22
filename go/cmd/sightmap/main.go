// Command sightmap is the unified sightmap authoring tool.
//
// Usage:
//
//	sightmap browser <subcommand>   manage the Chrome session
//	sightmap snapshot [flags]       extract and annotate the component tree
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
	case "browser":
		err = runBrowser(args)
	case "inspect":
		err = runInspect(args)
	case "snapshot":
		err = runSnapshot(args)
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
	case "snapshot-novelty", "snapshot_novelty":
		err = runSnapshotNovelty(args)
	case "snapshot-prune", "snapshot_prune":
		err = runSnapshotPrune(args)
	case "search":
		err = runSearch(args)
	case "discover":
		err = runDiscover(args)
	case "serve-sightmap", "serve_sightmap":
		err = runServeSightmap(args)
	case "iterate":
		err = runIterate(args)
	case "report":
		err = runReport(args)

	case "migrate-snapshots", "migrate_snapshots":
		err = runMigrateSnapshots(args)
	case "migrate-probe-urls", "migrate_probe_urls", "migrate-probes":
		err = runMigrateProbeURLs(args)
	case "migrate-visible-config", "migrate_visible_config":
		err = runMigrateVisibleConfig(args)
	case "sel-check", "sel_check":
		err = runSelCheck(args)

	// Hidden aliases kept for backwards compatibility.
	case "dump":
		err = runSnapshot(args)
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
		fmt.Fprintf(os.Stderr, "sightmap %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `sightmap — sightmap authoring toolkit

Commands:
  browser launch [--headless] [--port N] [--url URL]  start Chrome session
  browser stop / status / navigate / eval             session management
  browser click / fill / hover / keypress / scroll     interact with page elements
  browser drag / wait-for / dialog                     more interactions
  browser tabs list/new/select/close/resize            tab management

  report        [--sightmap-dir DIR] [--probe-urls FILE]          per-view coverage health table + T2 quality
  iterate URL  [--sightmap-dir DIR]  navigate+snap+coverage in one step (no tree output)
  migrate-snapshots [--sightmap-dir DIR] [--dry-run]              move legacy .snap files to organized structure
  snapshot       [--url URL] [--out FILE] [--all] [--sightmap-dir DIR]  annotated component tree
  inspect        [--url URL] [--out FILE] [--sightmap-dir DIR]  raw DOM tree for selector authoring
  coverage       [--sightmap-dir DIR] [FILE.snap ...]            offline T1/T2/T3 check
  multi-coverage [--sightmap-dir DIR] [FILE.snap ...]            cross-page coverage matrix
  snapshot-novelty [--sightmap-dir DIR] FILE.snap               does a capture add new components/slots vs its view set?
  snapshot-prune [--dry-run] (<view> | --all)                   drop captures subsumed by the rest of their view set
  suggest        [--sightmap-dir DIR] [--max N] [--exclude-known]  DOM selector candidates
  gap            [--sightmap-dir DIR] [--url URL]                   orphaned interactive nodes
  sel-probe [flags] 'selector'  [--all]                         selector validator (live)
  sel-check 'selector' FILE.snap                                offline selector validator
  validate [--sightmap-dir DIR]                              YAML structural check
  lint     [--sightmap-dir DIR]                              YAML style check
  search   [--field FIELD] PATTERN                           offline YAML content search
  discover [--all]                                           URL pattern discovery
  serve-sightmap [--port N] [--sightmap-dir DIR]             sightmap HTTP server for overlay extension

  skills install [--target DIR]                             install sightmap authoring skill to ~/.agents/skills/
  version                                                    print version and exit

Run 'sightmap <command> --help' for full flag list.
`)
}
