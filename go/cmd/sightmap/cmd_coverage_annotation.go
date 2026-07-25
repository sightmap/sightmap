package main

import (
	"fmt"

	"github.com/sightmap/sightmap/go/coverage"
)

// emitContentGaps prints the [Annotation gaps] advisory section for a view: the
// distinct uncaptured named content nodes found across its capture set. It is a
// nudge — it never changes coverage's exit status. Prints nothing when there are
// no gaps. The gap computation itself lives in the coverage package.
func emitContentGaps(viewName string, pres []coverage.ContentGapPresence) {
	if len(pres) == 0 {
		return
	}
	fmt.Println("[Annotation gaps] (named content with no component context)")
	for _, p := range pres {
		loc := ""
		if p.Gap.Selector != "" {
			loc = " inside " + p.Gap.Selector
		}
		name := displayName(p.Gap.Name)
		if p.Captures > 1 {
			fmt.Printf("  %s: %s %s%s \u2014 %d of %d snaps\n",
				viewName, p.Gap.Role, name, loc, p.AppearedIn, p.Captures)
		} else {
			fmt.Printf("  %s: %s %s%s\n", viewName, p.Gap.Role, name, loc)
		}
	}
	fmt.Println()
}
