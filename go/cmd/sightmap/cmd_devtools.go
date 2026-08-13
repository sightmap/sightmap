// CLI clients for the daemon's devtools query surface. These are thin HTTP
// clients: the browser start daemon owns the collector and does all filtering;
// these commands just render what it returns. They locate the daemon via the
// session file's ServerPort.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/sightmap"
)

// devtoolsGet fetches path (with query) from the daemon's HTTP server and
// returns the raw body. It resolves the daemon via the session file's
// ServerPort.
func devtoolsGet(sightmapDir, path string, query url.Values) ([]byte, error) {
	info, err := browser.ReadSessionInfo(sightmapDir)
	if err != nil || info.ServerPort == 0 {
		return nil, fmt.Errorf("no running session — start one with 'sightmap browser start'")
	}
	u := fmt.Sprintf("http://localhost:%d%s", info.ServerPort, path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("reach daemon: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("session is still starting (collector not ready) — retry shortly")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned %s: %s", resp.Status, string(body))
	}
	return body, nil
}

// consoleEntry / networkEntry are an observed record plus the names of the
// corpus definitions that classify it. The daemon annotates entries in the
// devtools handler (the collector stays corpus-blind); Matches is empty when no
// corpus is loaded or nothing matched. Embedding promotes the record's own JSON
// fields to the top level, so the wire shape is the record plus "matches".
type consoleEntry struct {
	sightmap.Message
	Matches []string                 `json:"matches,omitempty"`
	Props   []sightmap.PropertyValue `json:"props,omitempty"`
}

type networkEntry struct {
	sightmap.Request
	Matches []string                 `json:"matches,omitempty"`
	Props   []sightmap.PropertyValue `json:"props,omitempty"`
}

type consoleResult struct {
	Entries []consoleEntry `json:"entries"`
	Dropped int            `json:"dropped"`
}

type networkResult struct {
	Entries []networkEntry `json:"entries"`
	Dropped int            `json:"dropped"`
}

// annotateConsole / annotateNetwork project each observed record together with
// the names of the corpus defs that classify it. A nil corpus (none loaded, or a
// load error) yields entries with no matches — devtools stays useful without a
// corpus. Kept pure (corpus passed in, not loaded here) so they're unit-testable
// without a daemon or disk.
func annotateConsole(c *sightmap.Corpus, msgs []sightmap.Message) []consoleEntry {
	out := make([]consoleEntry, len(msgs))
	for i, m := range msgs {
		e := consoleEntry{Message: m}
		if c != nil {
			for _, mm := range c.MessagesForRecord(m) {
				e.Matches = append(e.Matches, mm.Name)
				e.Props = append(e.Props, mm.Properties...)
			}
		}
		out[i] = e
	}
	return out
}

func annotateNetwork(c *sightmap.Corpus, reqs []sightmap.Request) []networkEntry {
	out := make([]networkEntry, len(reqs))
	for i, r := range reqs {
		e := networkEntry{Request: r}
		if c != nil {
			// RequestsForRecord does route+method identity AND resolves each
			// matched def's properties[] against the record's live headers/body
			// (populated by the collector). RequestsForURL gave names only.
			for _, rm := range c.RequestsForRecord(r) {
				e.Matches = append(e.Matches, rm.Name)
				e.Props = append(e.Props, rm.Properties...)
			}
		}
		out[i] = e
	}
	return out
}

// propsSlot renders extracted property values as a trailing `{name=value, …}`
// token for a list line, or "" when none. It follows the record's own payload —
// the classification (matchSlot) leads, the extracted content trails — mirroring
// how a snapshot shows `[Component attr="value"]`.
func propsSlot(props []sightmap.PropertyValue) string {
	if len(props) == 0 {
		return ""
	}
	parts := make([]string, len(props))
	for i, p := range props {
		parts[i] = fmt.Sprintf("%s=%s", p.Name, p.Value)
	}
	return " {" + strings.Join(parts, ", ") + "}"
}

// printProps writes an extracted-property block for a `get` detail view, one
// `name = value` per line under a "Properties:" header. No-op when empty.
func printProps(props []sightmap.PropertyValue) {
	if len(props) == 0 {
		return
	}
	fmt.Println("Properties:")
	for _, p := range props {
		fmt.Printf("  %s = %s\n", p.Name, p.Value)
	}
}

// matchSlot renders the leading corpus-match token for a list line: the matched
// def name(s) in brackets, or "[--]" when unmatched. It leads the line (before
// the record's own payload) so an agent reads the classification first, mirroring
// how snapshots foreground a component's [Name]. Multi-match renders every name
// ("[A, B]") so the ambiguity is visible up front. Left-padded to a modest width
// so short tokens align for human scanning; long ones overflow.
func matchSlot(matches []string) string {
	token := "[--]"
	if len(matches) > 0 {
		token = "[" + strings.Join(matches, ", ") + "]"
	}
	return fmt.Sprintf("%-22s", token)
}

// ── console ─────────────────────────────────────────────────────────────────

func runConsole(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: sightmap console <list|get> [flags]")
	}
	switch args[0] {
	case "list":
		return runConsoleList(args[1:])
	case "get":
		return runConsoleGet(args[1:])
	default:
		return fmt.Errorf("console: unknown subcommand %q (want list|get)", args[0])
	}
}

func runConsoleList(args []string) error {
	fs := flag.NewFlagSet("console list", flag.ContinueOnError)
	level := fs.String("level", "", "Filter by level: log, debug, info, warn, error, exception")
	tab := fs.String("tab", "", "Filter by tab ID")
	limit := fs.Int("limit", 0, "Show only the most recent N messages (0 = all)")
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	q := url.Values{}
	setNonEmpty(q, "level", *level)
	setNonEmpty(q, "tab", *tab)
	if *limit > 0 {
		q.Set("limit", strconv.Itoa(*limit))
	}
	body, err := devtoolsGet(*sightmapDir, "/devtools/console", q)
	if err != nil {
		return err
	}
	var res consoleResult
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if len(res.Entries) == 0 {
		fmt.Println("No console messages captured.")
	} else {
		multiTab := spansMultipleTabs(res.Entries)
		for _, e := range res.Entries {
			if multiTab {
				fmt.Printf("[%d] %s %-9s [%s] %s%s\n", e.Index, matchSlot(e.Matches), e.Level, shortTab(e.Tab), e.Text, propsSlot(e.Props))
			} else {
				fmt.Printf("[%d] %s %-9s %s%s\n", e.Index, matchSlot(e.Matches), e.Level, e.Text, propsSlot(e.Props))
			}
		}
	}
	reportDropped(res.Dropped, "console")
	return nil
}

func runConsoleGet(args []string) error {
	fs := flag.NewFlagSet("console get", flag.ContinueOnError)
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	idx, err := singleIndexArg(fs.Args(), "console get")
	if err != nil {
		return err
	}
	body, err := devtoolsGet(*sightmapDir, "/devtools/console", nil)
	if err != nil {
		return err
	}
	var res consoleResult
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	for _, e := range res.Entries {
		if e.Index == idx {
			fmt.Printf("[%d] %s  (tab %s)\n%s\n", e.Index, e.Level, shortTab(e.Tab), e.Text)
			if len(e.Matches) > 0 {
				fmt.Printf("Matches: %s\n", strings.Join(e.Matches, ", "))
			}
			printProps(e.Props)
			return nil
		}
	}
	return fmt.Errorf("console message index %d not found", idx)
}

// ── network ─────────────────────────────────────────────────────────────────

func runNetwork(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: sightmap network <list|get> [flags]")
	}
	switch args[0] {
	case "list":
		return runNetworkList(args[1:])
	case "get":
		return runNetworkGet(args[1:])
	default:
		return fmt.Errorf("network: unknown subcommand %q (want list|get)", args[0])
	}
}

func runNetworkList(args []string) error {
	fs := flag.NewFlagSet("network list", flag.ContinueOnError)
	rtype := fs.String("type", "", "Filter by resource type (Document, XHR, Fetch, Stylesheet, Image, Script, Font, ...)")
	urlSub := fs.String("url", "", "Filter by URL substring")
	tab := fs.String("tab", "", "Filter by tab ID")
	limit := fs.Int("limit", 0, "Show only the most recent N requests (0 = all)")
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	q := url.Values{}
	setNonEmpty(q, "type", *rtype)
	setNonEmpty(q, "url", *urlSub)
	setNonEmpty(q, "tab", *tab)
	if *limit > 0 {
		q.Set("limit", strconv.Itoa(*limit))
	}
	body, err := devtoolsGet(*sightmapDir, "/devtools/network", q)
	if err != nil {
		return err
	}
	var res networkResult
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if len(res.Entries) == 0 {
		fmt.Println("No network requests captured.")
	} else {
		for _, e := range res.Entries {
			fmt.Printf("[%d] %s %s %s → %s (%s)%s\n", e.Index, matchSlot(e.Matches), e.Method, e.URL, statusStr(e.Request), e.ResourceType, propsSlot(e.Props))
		}
	}
	reportDropped(res.Dropped, "network")
	return nil
}

func runNetworkGet(args []string) error {
	fs := flag.NewFlagSet("network get", flag.ContinueOnError)
	respFile := fs.String("response-file", "", "Write the response body to this file")
	reqFile := fs.String("request-file", "", "Write the request body (POST data) to this file")
	sightmapDir := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	idx, err := singleIndexArg(fs.Args(), "network get")
	if err != nil {
		return err
	}

	body, err := devtoolsGet(*sightmapDir, "/devtools/network", nil)
	if err != nil {
		return err
	}
	var res networkResult
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	var entry *networkEntry
	for i := range res.Entries {
		if res.Entries[i].Index == idx {
			entry = &res.Entries[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("network request index %d not found", idx)
	}

	fmt.Printf("Method: %s\n", entry.Method)
	fmt.Printf("URL: %s\n", entry.URL)
	fmt.Printf("Resource Type: %s\n", entry.ResourceType)
	fmt.Printf("Status: %s\n", statusStr(entry.Request))
	fmt.Printf("Tab: %s\n", entry.Tab)
	if len(entry.Matches) > 0 {
		fmt.Printf("Matches: %s\n", strings.Join(entry.Matches, ", "))
	}
	printProps(entry.Props)

	if *reqFile != "" {
		if err := saveBody(*sightmapDir, idx, "request", *reqFile); err != nil {
			return err
		}
	}
	// Fetch the response body (to a file, or inline preview).
	rbody, err := devtoolsGet(*sightmapDir, "/devtools/network/body", url.Values{"index": {strconv.Itoa(idx)}, "kind": {"response"}})
	if err != nil {
		fmt.Fprintf(os.Stderr, "network get: %s\n", friendlyBodyErr(err))
		return nil
	}
	if *respFile != "" {
		if err := os.WriteFile(*respFile, rbody, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", *respFile, err)
		}
		fmt.Fprintf(os.Stderr, "response body saved: %s (%d bytes)\n", *respFile, len(rbody))
		return nil
	}
	printBodyPreview(rbody)
	return nil
}

// friendlyBodyErr rewrites the raw daemon/CDP error from a network-body fetch
// into an actionable message. Response bodies are only retained briefly, so the
// common failure is the body having been evicted from the network cache.
func friendlyBodyErr(err error) string {
	s := err.Error()
	if strings.Contains(s, "No resource with given identifier") || strings.Contains(s, "No data found") {
		return "response body no longer available — it was evicted from the network cache (bodies are retained only briefly after the response arrives)"
	}
	return "response body unavailable: " + s
}

func saveBody(sightmapDir string, idx int, kind, path string) error {
	b, err := devtoolsGet(sightmapDir, "/devtools/network/body", url.Values{"index": {strconv.Itoa(idx)}, "kind": {kind}})
	if err != nil {
		return fmt.Errorf("%s body: %s", kind, friendlyBodyErr(err))
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "%s body saved: %s (%d bytes)\n", kind, path, len(b))
	return nil
}

// ── shared helpers ──────────────────────────────────────────────────────────

const bodyPreviewLimit = 4096

func printBodyPreview(b []byte) {
	if len(b) == 0 {
		fmt.Println("Response Body: (empty)")
		return
	}
	if len(b) > bodyPreviewLimit {
		fmt.Printf("Response Body (first %d of %d bytes; use --response-file for the full body):\n%s\n",
			bodyPreviewLimit, len(b), string(b[:bodyPreviewLimit]))
		return
	}
	fmt.Printf("Response Body:\n%s\n", string(b))
}

func statusStr(e sightmap.Request) string {
	if e.Status == 0 {
		return "pending"
	}
	return fmt.Sprintf("%d %s", e.Status, e.StatusText)
}

func reportDropped(n int, stream string) {
	if n > 0 {
		fmt.Printf("(%d earlier %s entries dropped — buffer overflowed)\n", n, stream)
	}
}

func setNonEmpty(q url.Values, key, val string) {
	if val != "" {
		q.Set(key, val)
	}
}

func singleIndexArg(args []string, cmd string) (int, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("usage: sightmap %s <index>", cmd)
	}
	idx, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, fmt.Errorf("%s: index must be a number, got %q", cmd, args[0])
	}
	return idx, nil
}

func spansMultipleTabs(entries []consoleEntry) bool {
	seen := ""
	for _, e := range entries {
		if seen == "" {
			seen = e.Tab
		} else if e.Tab != seen {
			return true
		}
	}
	return false
}

func shortTab(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
