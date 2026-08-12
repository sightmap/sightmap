package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

// runExport implements `sightmap export [dir]`: load a .sightmap/ corpus and
// emit the canonical Corpus wire — the exact json.Marshal(sightmap.Corpus) shape
// a go-get/library consumer reads, byte-for-byte — to stdout, a file (-o), or an
// HTTP endpoint (--url).
//
// It replaces the hand-rolled Python collector (collect_and_upload_sightmap.py
// in the subtext-sightmap skill), a second, lossy serializer that flattened
// views into compound selectors and dropped routes/requests/messages. Routing
// the upload through the Go loader makes the loader the single source of truth
// and shares the exact sightmap.Corpus type with the server-side reader, so the
// two ends cannot drift.
func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	out := fs.String("o", "", "write to FILE instead of stdout (ignored when --url is set)")
	postURL := fs.String("url", "", "POST the corpus to this URL as application/json (no auth headers; the URL carries its own token)")
	sightmapDir := fs.String("sightmap-dir", "", "path to the .sightmap/ dir (default: the nearest one at or above [dir] or cwd)")
	insecure := fs.Bool("insecure", false, "skip TLS verification when POSTing (auto-enabled for localhost and .test hosts)")

	positional, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return fmt.Errorf("export: expected at most one [dir], got %d", len(positional))
	}

	dir, err := resolveExportDir(*sightmapDir, positional)
	if err != nil {
		return err
	}

	corp, err := sightmap.Load(dir)
	if err != nil {
		return fmt.Errorf("export: load %s: %w", dir, err)
	}

	// The canonical wire: indented for human/file readability; any JSON reader
	// (the server-side sightmap.Corpus reader included) parses it the same.
	data, err := json.MarshalIndent(corp, "", "  ")
	if err != nil {
		return fmt.Errorf("export: marshal corpus: %w", err)
	}

	if *postURL != "" {
		return postJSON(*postURL, data, *insecure)
	}
	return writeOut(*out, func(w io.Writer) error {
		_, werr := w.Write(append(data, '\n'))
		return werr
	})
}

// runPush implements `sightmap push URL [FILE]`: POST FILE (or stdin) to URL as
// application/json. It is the transport half of export split off as a composable
// primitive, so `sightmap export -o corpus.json` then `sightmap push URL
// corpus.json`, or `sightmap export | sightmap push URL`, both work. Both paths
// share postJSON, so the wire contract (no auth headers, local-host TLS skip) is
// defined in exactly one place.
func runPush(args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	insecure := fs.Bool("insecure", false, "skip TLS verification (auto-enabled for localhost and .test hosts)")

	positional, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	if len(positional) < 1 {
		return fmt.Errorf("push: usage: sightmap push URL [FILE]")
	}
	if len(positional) > 2 {
		return fmt.Errorf("push: expected URL [FILE], got %d arguments", len(positional))
	}
	target := positional[0]

	var body []byte
	if len(positional) == 2 && positional[1] != "-" {
		if body, err = os.ReadFile(positional[1]); err != nil {
			return fmt.Errorf("push: read %s: %w", positional[1], err)
		}
	} else {
		if body, err = io.ReadAll(os.Stdin); err != nil {
			return fmt.Errorf("push: read stdin: %w", err)
		}
	}
	return postJSON(target, body, *insecure)
}

// resolveExportDir picks the .sightmap/ directory to load. Precedence:
//  1. an explicit --sightmap-dir, used as-is (consistent with every other command);
//  2. otherwise, the nearest .sightmap/ at or above the positional [dir], or cwd
//     when no [dir] is given — the upward walk the Python collector did.
func resolveExportDir(flagDir string, positional []string) (string, error) {
	if flagDir != "" {
		return flagDir, nil
	}
	start := cwd()
	if len(positional) == 1 {
		start = positional[0]
	}
	if dir := findSightmapDir(start); dir != "" {
		return dir, nil
	}
	return "", fmt.Errorf("export: no .sightmap/ directory found at or above %s", start)
}

// findSightmapDir returns the nearest .sightmap/ directory at or above start, or
// "" if none exists. When start is itself a .sightmap directory it is returned
// directly; otherwise start/.sightmap and each ancestor's .sightmap is tried.
func findSightmapDir(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		abs = start
	}
	if filepath.Base(abs) == ".sightmap" && isDir(abs) {
		return abs
	}
	for d := abs; ; {
		if cand := filepath.Join(d, ".sightmap"); isDir(cand) {
			return cand
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// postJSON POSTs body to rawURL as application/json with NO auth headers: the
// destination URL is expected to carry a single-use token as a query parameter,
// so the tool needs zero knowledge of the consumer. TLS verification is skipped
// when insecure is set or the host is local (localhost, 127.0.0.1, ::1, or a
// *.test / *.localhost name) — the self-signed-cert case for local dev servers,
// matching the old Python uploader.
func postJSON(rawURL string, body []byte, insecure bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https, got %q", u.Scheme)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	if insecure || isLocalHost(u.Hostname()) {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local dev / explicit opt-in only
		}
	}

	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", u.Host, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("POST %s failed (HTTP %d): %s", u.Host, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// Consumer-agnostic: report success and echo any short response body (the
	// destination decides its own confirmation shape) to stderr.
	fmt.Fprintf(os.Stderr, "sightmap: uploaded %d bytes to %s (HTTP %d)\n", len(body), u.Host, resp.StatusCode)
	if trimmed := strings.TrimSpace(string(respBody)); trimmed != "" {
		fmt.Fprintf(os.Stderr, "%s\n", trimmed)
	}
	return nil
}

// isLocalHost reports whether host is a loopback or local development name, for
// which self-signed TLS is skipped automatically.
func isLocalHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return strings.HasSuffix(host, ".test") || strings.HasSuffix(host, ".localhost")
}
