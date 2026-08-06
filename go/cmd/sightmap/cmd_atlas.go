// atlas browses and installs corpora published in the community atlas
// (github.com/sightmap/atlas): `find` searches the catalog by domain, name, or
// category, `list` browses it, and `add` installs one. The index schema, the
// ranking, the cache, the fetch policy, and the install live in the atlas
// package; these files are flags in, formatted output out.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/atlas"
)

// runAtlas dispatches the atlas subcommands. It is registered in main.go.
func runAtlas(args []string) error {
	if len(args) == 0 {
		atlasUsage()
		return nil
	}
	switch args[0] {
	case "list":
		return runAtlasList(args[1:])
	case "find":
		return runAtlasFind(args[1:])
	case "add":
		return runAtlasAdd(args[1:])
	case "help", "--help", "-h":
		atlasUsage()
		return nil
	default:
		atlasUsage()
		return fmt.Errorf("unknown subcommand %q", atlas.SafeText(args[0]))
	}
}

func atlasUsage() {
	fmt.Fprint(os.Stderr, `sightmap atlas — corpora published in the community atlas

  list [--category C] [--limit N] [--json]           browse the catalog
  find QUERY [--category C] [--limit N] [--json]     search by domain, name, category, or description
  add SLUG [--target DIR]                            install a corpus into .sightmap/

Have a URL, not a slug? Start there:

  sightmap atlas find squareup.com

Flags:
  --category C   keep only entries in a matching category
  --limit N      show at most N results (default 10; 0 shows all)
  --json         machine-readable results
  --index URL    atlas index.json URL (for mirrors and tests)
  --refresh      re-fetch the index instead of using the cached copy
  --target DIR   where add installs the corpus (default .sightmap)
  --source URL   archive URL template for add, with {slug} substituted
`)
}

// searchFlags are the flags find and list share.
type searchFlags struct {
	fs       *flag.FlagSet
	indexURL *string
	category *string
	limit    *int
	asJSON   *bool
	refresh  *bool
}

func newSearchFlags(name string) *searchFlags {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	f := &searchFlags{
		fs:       fs,
		indexURL: fs.String("index", atlas.DefaultIndexURL, "Atlas index.json URL (for mirrors and tests)"),
		category: fs.String("category", "", "Keep only entries in a matching category"),
		limit:    fs.Int("limit", 10, "Show at most N results (0 shows all)"),
		asJSON:   fs.Bool("json", false, "Print results as JSON"),
		refresh:  fs.Bool("refresh", false, "Re-fetch the index instead of using the cached copy"),
	}
	return f
}

// runAtlasList browses the catalog. It is `find` with an empty query.
func runAtlasList(args []string) error {
	return runAtlasListOut(args, os.Stdout)
}

func runAtlasListOut(args []string, out io.Writer) error {
	f := newSearchFlags("atlas list")
	f.fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sightmap atlas list [--category C] [--limit N] [--json] [--index URL] [--refresh]\n\nLists corpora published in the community atlas.\n\nFlags:\n")
		f.fs.PrintDefaults()
	}
	if err := f.fs.Parse(args); err != nil {
		return err
	}
	if f.fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q — use 'sightmap atlas find %s' to search", atlas.SafeText(f.fs.Arg(0)), atlas.SafeText(f.fs.Arg(0)))
	}
	return searchAtlas(context.Background(), "", f, out)
}

// runAtlasFind searches the catalog.
func runAtlasFind(args []string) error {
	return runAtlasFindOut(args, os.Stdout)
}

func runAtlasFindOut(args []string, out io.Writer) error {
	f := newSearchFlags("atlas find")
	f.fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: sightmap atlas find QUERY [--category C] [--limit N] [--json] [--index URL] [--refresh]\n\nSearches the community atlas by domain, name, category, or description.\nAn exact domain match ranks first, so a URL is a good query.\n\nFlags:\n")
		f.fs.PrintDefaults()
	}
	if err := f.fs.Parse(args); err != nil {
		return err
	}
	rest := f.fs.Args()
	if len(rest) == 0 {
		f.fs.Usage()
		return fmt.Errorf("expected a QUERY argument — run 'sightmap atlas list' to browse everything")
	}
	// Re-parse what follows the query so flags work on either side of it: the
	// flag package stops at the first positional argument.
	var words []string
	for len(rest) > 0 {
		words = append(words, rest[0])
		if err := f.fs.Parse(rest[1:]); err != nil {
			return err
		}
		rest = f.fs.Args()
	}
	return searchAtlas(context.Background(), strings.Join(words, " "), f, out)
}

// searchAtlas is the one code path behind both verbs.
func searchAtlas(ctx context.Context, query string, f *searchFlags, out io.Writer) error {
	res, err := atlas.LoadIndex(ctx, atlas.IndexOptions{
		URL:     *f.indexURL,
		Refresh: *f.refresh,
	})
	if err != nil {
		return err
	}
	hits := res.Index.Search(atlas.Query{Text: query, Category: *f.category})
	shown := hits
	if *f.limit > 0 && len(shown) > *f.limit {
		shown = shown[:*f.limit]
	}
	if *f.asJSON {
		return writeAtlasJSON(out, query, *f.category, len(hits), shown, res)
	}
	writeAtlasText(out, query, len(hits), shown, res)
	return nil
}

// writeAtlasText prints one block per hit: what it is, what it covers, and the
// command that installs it. An empty result is a successful search, so it
// prints where to look next and exits 0.
func writeAtlasText(out io.Writer, query string, total int, hits []atlas.Hit, res *atlas.IndexResult) {
	if len(hits) == 0 {
		if query == "" {
			fmt.Fprintf(out, "The atlas index at %s publishes no entries.\n", atlas.SafeText(res.Source))
			return
		}
		fmt.Fprintf(out, "No atlas entry matches %q.\n", atlas.SafeText(query))
		fmt.Fprintf(out, "Try the product name or a category, browse %s, or map the site yourself with sightmap init.\n", atlas.AtlasURL)
		return
	}
	for i, h := range hits {
		if i > 0 {
			fmt.Fprintln(out)
		}
		e := h.Entry
		fmt.Fprintf(out, "%s", atlas.SafeText(e.Slug))
		if name := atlas.SafeText(e.Name); name != "" && name != atlas.SafeText(e.Slug) {
			fmt.Fprintf(out, "  %s", name)
		}
		fmt.Fprintln(out)
		if d := atlas.SafeText(e.Description); d != "" {
			fmt.Fprintf(out, "  %s\n", d)
		}
		fmt.Fprintf(out, "  %s\n", e.Detail())
		fmt.Fprintf(out, "  sightmap atlas add %s\n", atlas.SafeText(e.Slug))
	}
	fmt.Fprintln(out)
	if total > len(hits) {
		fmt.Fprintf(out, "%d of %d matches. Pass --limit to see more.\n", len(hits), total)
	} else {
		fmt.Fprintf(out, "%s.\n", plural(total, "match", "matches"))
	}
	if res.FromCache {
		fmt.Fprintf(out, "Index cached %s. Pass --refresh to re-fetch.\n", res.FetchedAt.UTC().Format("2006-01-02 15:04 MST"))
	}
}

// atlasJSON is the --json document: one flat object per hit, install command
// included, so an agent needs no second lookup to act on a result.
type atlasJSON struct {
	Query    string          `json:"query"`
	Category string          `json:"category,omitempty"`
	Total    int             `json:"total"`
	Shown    int             `json:"shown"`
	Results  []atlasJSONHit  `json:"results"`
	Index    atlasJSONSource `json:"index"`
}

type atlasJSONHit struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name,omitempty"`
	Description  string   `json:"description,omitempty"`
	Domains      []string `json:"domains,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	Views        int      `json:"views"`
	Components   int      `json:"components"`
	LastVerified string   `json:"last_verified,omitempty"`
	MatchedOn    string   `json:"matched_on"`
	Install      string   `json:"install"`
}

type atlasJSONSource struct {
	Source    string `json:"source"`
	FetchedAt string `json:"fetched_at"`
	Cached    bool   `json:"cached"`
}

func writeAtlasJSON(out io.Writer, query, category string, total int, hits []atlas.Hit, res *atlas.IndexResult) error {
	doc := atlasJSON{
		Query:    query,
		Category: category,
		Total:    total,
		Shown:    len(hits),
		Results:  make([]atlasJSONHit, 0, len(hits)),
		Index: atlasJSONSource{
			Source:    atlas.SafeText(res.Source),
			FetchedAt: res.FetchedAt.UTC().Format(time.RFC3339),
			Cached:    res.FromCache,
		},
	}
	for _, h := range hits {
		e := h.Entry
		doc.Results = append(doc.Results, atlasJSONHit{
			Slug:         atlas.SafeText(e.Slug),
			Name:         atlas.SafeText(e.Name),
			Description:  atlas.SafeText(e.Description),
			Domains:      safeStrings(e.Domains),
			Categories:   safeStrings(e.Categories),
			Views:        e.Stats.Views,
			Components:   e.Stats.Components,
			LastVerified: atlas.SafeText(e.LastVerified),
			MatchedOn:    h.Rank.String(),
			Install:      "sightmap atlas add " + atlas.SafeText(e.Slug),
		})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func safeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, atlas.SafeText(v))
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
