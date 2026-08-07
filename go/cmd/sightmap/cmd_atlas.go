// atlas searches and installs corpora published at sightmap.org/atlas: `find`
// searches the catalog, `list` browses it, and `add` installs one. The catalog
// schema, the ranking, the cache, the fetch policy and the install live in the
// atlas package; this file is flags in, formatted output out.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/atlas"
)

// maxDetailRunes bounds how much catalog-authored text one result line carries,
// so a verbose or hostile entry cannot flood a terminal or an agent's context.
const maxDetailRunes = 200

func runAtlas(args []string) error {
	return runAtlasOut(args, os.Stdout)
}

func runAtlasOut(args []string, out io.Writer) error {
	if len(args) == 0 {
		atlasUsage()
		return nil
	}
	switch args[0] {
	case "find", "list":
		return runAtlasSearch(args[0], args[1:], out)
	case "add":
		return runAtlasAdd(args[1:], out)
	case "validate":
		return runAtlasValidate(args[1:], out)
	case "help", "--help", "-h":
		atlasUsage()
		return nil
	default:
		atlasUsage()
		return fmt.Errorf("unknown subcommand %q", atlas.SafeText(args[0]))
	}
}

func atlasUsage() {
	fmt.Fprintf(os.Stderr, `sightmap atlas — corpora published at %s

  list [flags]               browse the catalog
  find QUERY [flags]         search by domain, name, category, or description
  add SLUG [--target DIR]    install a corpus into .sightmap/
  validate [FILE|-]          check a catalog before publishing it

Have a URL rather than a slug? Start there:

  sightmap atlas find squareup.com

Run any subcommand with --help for its flags.
`, atlas.AtlasURL)
}

// runAtlasSearch backs both find and list: list is find with an empty query.
func runAtlasSearch(verb string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("atlas "+verb, flag.ContinueOnError)
	indexURL := fs.String("index", atlas.DefaultIndexURL, "Catalog index.json URL, for mirrors and private catalogs")
	category := fs.String("category", "", "Keep only entries in a matching category")
	limit := fs.Int("limit", 10, "Show at most N results (0 shows all)")
	asJSON := fs.Bool("json", false, "Print results as JSON")
	refresh := fs.Bool("refresh", false, "Re-fetch the catalog instead of using the cached copy")
	fs.Usage = func() {
		if verb == "find" {
			fmt.Fprint(os.Stderr, "Usage: sightmap atlas find QUERY [flags]\n\nSearches the community atlas by domain, name, category, or description.\nAn exact domain match ranks first, so a URL is a good query.\n\nFlags:\n")
		} else {
			fmt.Fprint(os.Stderr, "Usage: sightmap atlas list [flags]\n\nLists every corpus published in the community atlas.\n\nFlags:\n")
		}
		fs.PrintDefaults()
	}

	words, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	if verb == "find" && len(words) == 0 {
		fs.Usage()
		return fmt.Errorf("expected a QUERY argument — run 'sightmap atlas list' to browse everything")
	}
	if verb == "list" && len(words) > 0 {
		return fmt.Errorf("unexpected argument %q — use 'sightmap atlas find %s' to search", atlas.SafeText(words[0]), atlas.SafeText(words[0]))
	}
	if *limit < 0 {
		return fmt.Errorf("--limit must be 0 or greater (0 shows all)")
	}
	query := strings.Join(words, " ")

	res, err := atlas.LoadIndex(context.Background(), atlas.IndexOptions{URL: *indexURL, Refresh: *refresh})
	if err != nil {
		return err
	}
	hits := res.Index.Search(atlas.Query{Text: query, Category: *category})
	shown := hits
	if *limit > 0 && len(shown) > *limit {
		shown = shown[:*limit]
	}
	if *asJSON {
		return writeAtlasJSON(out, query, *category, len(hits), shown, res)
	}
	writeAtlasText(out, query, *category, len(hits), shown, res)
	return nil
}

// writeAtlasText prints one block per hit: what it is, what it covers, and the
// command that installs it.
func writeAtlasText(out io.Writer, query, category string, total int, hits []atlas.Hit, res *atlas.IndexResult) {
	if len(hits) == 0 {
		writeNoMatch(out, query, category, res.Source)
		return
	}
	for i, h := range hits {
		if i > 0 {
			fmt.Fprintln(out)
		}
		e := h.Entry
		title := e.Slug
		if e.Name != "" && e.Name != e.Slug {
			title += "  " + e.Name
		}
		fmt.Fprintln(out, title)
		if e.Description != "" {
			fmt.Fprintf(out, "  %s\n", truncate(e.Description, maxDetailRunes))
		}
		fmt.Fprintf(out, "  %s\n", detailLine(e))
		fmt.Fprintf(out, "  sightmap atlas add %s\n", e.Slug)
	}
	fmt.Fprintln(out)
	if total > len(hits) {
		fmt.Fprintf(out, "%d of %d matches. Pass --limit to see more.\n", len(hits), total)
	} else {
		fmt.Fprintf(out, "%d match%s.\n", total, pluralS(total))
	}
	if res.FromCache {
		fmt.Fprintf(out, "Catalog cached %s. Pass --refresh to re-fetch.\n", res.FetchedAt.UTC().Format("2006-01-02 15:04 MST"))
	}
}

// writeNoMatch names every constraint that was applied, so a caller can tell a
// category with nothing filed under it from an empty catalog before deciding to
// author a corpus of its own.
func writeNoMatch(out io.Writer, query, category, source string) {
	var applied []string
	if query != "" {
		applied = append(applied, fmt.Sprintf("matches %q", atlas.SafeText(query)))
	}
	if category != "" {
		applied = append(applied, fmt.Sprintf("is in category %q", atlas.SafeText(category)))
	}
	if len(applied) == 0 {
		fmt.Fprintf(out, "The atlas catalog at %s publishes no entries.\n", atlas.SafeText(source))
		return
	}
	fmt.Fprintf(out, "No atlas entry %s.\n", strings.Join(applied, " and "))
	fmt.Fprintf(out, "Run sightmap atlas list to see everything published, browse %s, or map the site yourself with sightmap init.\n", atlas.AtlasURL)
}

// detailLine summarizes what an entry covers: its domains, its categories, how
// much of the site it maps, and when someone last checked it against the live
// site.
func detailLine(e atlas.Entry) string {
	var parts []string
	for _, group := range [][]string{e.Domains, e.Categories, atlas.StatCounts(e.Stats)} {
		if len(group) > 0 {
			parts = append(parts, strings.Join(group, ", "))
		}
	}
	if e.LastVerified != "" {
		parts = append(parts, "verified "+e.LastVerified)
	}
	if len(parts) == 0 {
		return "(the atlas publishes no details for this entry)"
	}
	return truncate(strings.Join(parts, " · "), maxDetailRunes)
}

// atlasJSON is the --json document. Entries are embedded whole, so the fields
// an agent reads are the fields the catalog publishes, plus why the entry
// matched and the command that installs it.
type atlasJSON struct {
	Query    string     `json:"query"`
	Category string     `json:"category,omitempty"`
	Total    int        `json:"total"`
	Shown    int        `json:"shown"`
	Results  []atlasHit `json:"results"`
	Index    struct {
		Source    string `json:"source"`
		FetchedAt string `json:"fetched_at"`
		Cached    bool   `json:"cached"`
	} `json:"index"`
}

type atlasHit struct {
	atlas.Entry
	MatchedOn string `json:"matched_on"`
	Install   string `json:"install"`
}

func writeAtlasJSON(out io.Writer, query, category string, total int, hits []atlas.Hit, res *atlas.IndexResult) error {
	doc := atlasJSON{
		Query:    query,
		Category: category,
		Total:    total,
		Shown:    len(hits),
		Results:  make([]atlasHit, 0, len(hits)),
	}
	doc.Index.Source = res.Source
	doc.Index.FetchedAt = res.FetchedAt.UTC().Format(time.RFC3339)
	doc.Index.Cached = res.FromCache
	for _, h := range hits {
		doc.Results = append(doc.Results, atlasHit{
			Entry:     h.Entry,
			MatchedOn: h.MatchedOn,
			Install:   "sightmap atlas add " + h.Entry.Slug,
		})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// runAtlasValidate checks a catalog before it is published. It reads a file
// rather than a URL because the caller is the atlas repository's CI, checking
// the index.json it is about to merge.
func runAtlasValidate(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("atlas validate", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: sightmap atlas validate [FILE|-]

Checks a catalog index.json for problems every shipped sightmap would hit:
an unreadable schema version, a slug that could not be installed, a duplicate
slug, and display text carrying control characters. Reads stdin for "-" or
when FILE is omitted.

Exits 0 when the catalog is clean, 1 when it is not.
`)
	}
	files, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	if len(files) > 1 {
		return fmt.Errorf("unexpected argument %q after FILE", atlas.SafeText(files[1]))
	}

	data, err := readFileOrStdin(files)
	if err != nil {
		return err
	}
	problems := atlas.Validate(data)
	if len(problems) == 0 {
		fmt.Fprintln(out, "The catalog is valid.")
		return nil
	}
	for _, p := range problems {
		fmt.Fprintf(out, "  %s\n", p)
	}
	return fmt.Errorf("%d problem(s) in the catalog", len(problems))
}

func readFileOrStdin(files []string) ([]byte, error) {
	if len(files) == 0 || files[0] == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(files[0])
}

func runAtlasAdd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("atlas add", flag.ContinueOnError)
	target := fs.String("target", ".sightmap", "Directory to install the corpus into")
	source := fs.String("source", atlas.DefaultArchiveURL, "Archive URL template with {slug} substituted, for mirrors and private corpora")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: sightmap atlas add SLUG [flags]

Installs a corpus published at %s into --target (default ./.sightmap).
A non-empty target is refused; delete it yourself to install over it.

Have a URL rather than a slug? Run: sightmap atlas find <domain>
Install from a private corpus store:

  sightmap atlas add toast-pos --source https://internal.corp/{slug}.tar.gz

Flags:
`, atlas.AtlasURL)
		fs.PrintDefaults()
	}

	words, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(words) == 0:
		fs.Usage()
		return fmt.Errorf("expected a SLUG argument")
	case len(words) > 1:
		return fmt.Errorf("unexpected argument %q after SLUG", atlas.SafeText(words[1]))
	}

	res, err := atlas.Install(context.Background(), words[0], atlas.Options{ArchiveURL: *source, Target: *target})
	if err != nil {
		return err
	}
	for _, rel := range res.Files {
		fmt.Fprintf(out, "  wrote  %s\n", filepath.Join(res.Target, filepath.FromSlash(rel)))
	}
	noun := "files"
	if len(res.Files) == 1 {
		noun = "file"
	}
	fmt.Fprintf(out, "\nInstalled %s: %d %s → %s. Next:\n  sightmap validate\n",
		res.Slug, len(res.Files), noun, res.Target)
	return nil
}
