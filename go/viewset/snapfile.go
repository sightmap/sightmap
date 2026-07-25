package viewset

import (
	"os"
	"strings"
)

// A saved capture's .snap file carries a small text header — a "[View: NAME]"
// line and a "route: /path" line — that the offline readers parse to recover the
// view identity and the route to re-match against without loading the tree JSON.

// ViewNameOf reads the first lines of a .snap file and returns the view name
// from its "[View: NAME]" header, or "" if not found.
func ViewNameOf(snapFile string) string {
	data, err := os.ReadFile(snapFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.SplitN(string(data), "\n", 10) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[View: ") && strings.HasSuffix(line, "]") {
			return strings.TrimSuffix(strings.TrimPrefix(line, "[View: "), "]")
		}
	}
	return ""
}

// RouteOf reads the "route: /path" line from a .snap file header, or "" if not
// found.
func RouteOf(snapFile string) string {
	data, err := os.ReadFile(snapFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.SplitN(string(data), "\n", 10) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "route: ") {
			return strings.TrimPrefix(line, "route: ")
		}
	}
	return ""
}
