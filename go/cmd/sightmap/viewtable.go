package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

// The offline per-view tables (report, stats) share one presentation
// convention, kept here so the constants live in exactly one place: a
// "sightmap VERB · SITE · DATE" banner, then a table whose View and Route
// columns are seeded by their labels, widened to the longest value, and capped
// so one pathological route cannot push the numeric columns off the terminal.
const (
	viewNameColMax  = 30
	viewRouteColMax = 35
)

// printTableBanner writes the "sightmap VERB · SITE · DATE" line that opens an
// offline table. The site is the current working directory's last path
// component — the repo the corpus describes, not the corpus directory, so the
// banner reads the same from any --sightmap-dir.
func printTableBanner(out io.Writer, verb string) {
	wd, _ := os.Getwd()
	fmt.Fprintf(out, "sightmap %s · %s · %s\n", verb, lastPathComponent(wd), time.Now().Format("2006-01-02"))
}

// viewColWidths returns the View and Route column widths for a per-view table:
// wide enough for the header labels and the longest value, capped at
// viewNameColMax / viewRouteColMax. Values longer than their width are the
// caller's to truncate.
func viewColWidths(names, routes []string) (nameW, routeW int) {
	nameW = len("View")
	routeW = len("Route")
	for _, n := range names {
		if w := len(n); w > nameW {
			nameW = w
		}
	}
	for _, r := range routes {
		if w := len(r); w > routeW {
			routeW = w
		}
	}
	if nameW > viewNameColMax {
		nameW = viewNameColMax
	}
	if routeW > viewRouteColMax {
		routeW = viewRouteColMax
	}
	return nameW, routeW
}
