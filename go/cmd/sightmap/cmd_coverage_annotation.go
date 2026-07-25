package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

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

// displayName truncates s to 60 Unicode code points and wraps it in double
// quotes. Only backslash and double-quote are escaped; all other Unicode is
// preserved as-is so the output stays human-readable.
func displayName(s string) string {
	const maxRunes = 60
	if utf8.RuneCountInString(s) > maxRunes {
		runes := []rune(s)
		s = string(runes[:maxRunes]) + "…"
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
