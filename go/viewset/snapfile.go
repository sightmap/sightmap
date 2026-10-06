package viewset

import (
	"os"
	"strings"
)

// A saved capture's .snap file carries a small text header — a "[View: NAME]"
// line, a "route: /path" line and (since captures record it) a "url: URL"
// line — that the offline readers parse to recover the
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

// URLOf reads the "url: URL" line from a .snap file header: the page URL the
// capture was taken at. "" for captures written before it was recorded.
func URLOf(snapFile string) string {
	data, err := os.ReadFile(snapFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.SplitN(string(data), "\n", 10) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "url: ") {
			return strings.TrimPrefix(line, "url: ")
		}
		if line == "" {
			break // the header ends at the first blank line
		}
	}
	return ""
}
