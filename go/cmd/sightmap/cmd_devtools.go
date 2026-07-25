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
	"time"

	"github.com/sightmap/sightmap/go/browser"
)

// devtoolsGet fetches path (with query) from the daemon's HTTP server and
// returns the raw body. It resolves the daemon via the session file's
// ServerPort.
func devtoolsGet(path string, query url.Values) ([]byte, error) {
	info, err := browser.ReadSessionInfo()
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

type consoleResult struct {
	Entries []browser.ConsoleEntry `json:"entries"`
	Dropped int                    `json:"dropped"`
}

type networkResult struct {
	Entries []browser.NetworkEntry `json:"entries"`
	Dropped int                    `json:"dropped"`
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	q := url.Values{}
	setNonEmpty(q, "level", *level)
	setNonEmpty(q, "tab", *tab)
	if *limit > 0 {
		q.Set("limit", strconv.Itoa(*limit))
	}
	body, err := devtoolsGet("/devtools/console", q)
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
				fmt.Printf("[%d] %-9s [%s] %s\n", e.Index, e.Level, shortTab(e.Tab), e.Text)
			} else {
				fmt.Printf("[%d] %-9s %s\n", e.Index, e.Level, e.Text)
			}
		}
	}
	reportDropped(res.Dropped, "console")
	return nil
}

func runConsoleGet(args []string) error {
	fs := flag.NewFlagSet("console get", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	idx, err := singleIndexArg(fs.Args(), "console get")
	if err != nil {
		return err
	}
	body, err := devtoolsGet("/devtools/console", nil)
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
	body, err := devtoolsGet("/devtools/network", q)
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
			fmt.Printf("[%d] %s %s → %s (%s)\n", e.Index, e.Method, e.URL, statusStr(e), e.ResourceType)
		}
	}
	reportDropped(res.Dropped, "network")
	return nil
}

func runNetworkGet(args []string) error {
	fs := flag.NewFlagSet("network get", flag.ContinueOnError)
	respFile := fs.String("response-file", "", "Write the response body to this file")
	reqFile := fs.String("request-file", "", "Write the request body (POST data) to this file")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	idx, err := singleIndexArg(fs.Args(), "network get")
	if err != nil {
		return err
	}

	body, err := devtoolsGet("/devtools/network", nil)
	if err != nil {
		return err
	}
	var res networkResult
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	var entry *browser.NetworkEntry
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
	fmt.Printf("Status: %s\n", statusStr(*entry))
	fmt.Printf("Tab: %s\n", entry.Tab)

	if *reqFile != "" {
		if err := saveBody(idx, "request", *reqFile); err != nil {
			return err
		}
	}
	// Fetch the response body (to a file, or inline preview).
	rbody, err := devtoolsGet("/devtools/network/body", url.Values{"index": {strconv.Itoa(idx)}, "kind": {"response"}})
	if err != nil {
		fmt.Fprintf(os.Stderr, "network get: response body unavailable: %v\n", err)
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

func saveBody(idx int, kind, path string) error {
	b, err := devtoolsGet("/devtools/network/body", url.Values{"index": {strconv.Itoa(idx)}, "kind": {kind}})
	if err != nil {
		return fmt.Errorf("%s body: %w", kind, err)
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

func statusStr(e browser.NetworkEntry) string {
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

func spansMultipleTabs(entries []browser.ConsoleEntry) bool {
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
