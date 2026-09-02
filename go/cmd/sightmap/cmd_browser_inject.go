// Persistent script injection: `browser inject`. Where `eval` runs a script once
// in the current document, `inject --persist` registers it with the session
// daemon's collector, which re-applies it to every tab at the start of every new
// document (CDP Page.addScriptToEvaluateOnNewDocument) — so it survives
// navigations and new tabs for the life of the session. Useful for polyfills,
// overlays, and injecting an instrumentation/experimentation bundle that must
// outlive a multi-page flow instead of being re-evaled after each navigation.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/browser"
)

func runInject(args []string) error {
	fs := flag.NewFlagSet("inject", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID for the immediate one-shot run (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	fileFlag := fs.String("file", "", "Read the script source from this file (instead of an inline arg)")
	persistFlag := fs.Bool("persist", false, "Re-inject on every new document + new tab for the whole session (survives navigations)")
	removeFlag := fs.String("remove", "", "Remove a previously-persisted script by its id")
	listFlag := fs.Bool("list", false, "List the scripts currently persisted for this session")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	// Management modes talk only to the daemon.
	if *listFlag {
		return injectList(*sightmapDirFlag)
	}
	if *removeFlag != "" {
		return injectRemove(*sightmapDirFlag, *removeFlag)
	}

	// Otherwise we need a script source: exactly one of --file or an inline arg.
	source, err := injectSource(*fileFlag, fs.Args())
	if err != nil {
		return err
	}

	if *persistFlag {
		// Register with the daemon so the script outlives this command's transient
		// connection and rides every future navigation.
		id, err := injectPersist(*sightmapDirFlag, source)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "inject: persisted as %s — re-injected on every new document; remove with 'browser inject --remove %s'\n", id, id)
		// addScriptToEvaluateOnNewDocument only affects FUTURE documents, so also
		// run it once now (best-effort) to cover the already-loaded page.
		if err := injectOnce(*addrFlag, *tabFlag, *sightmapDirFlag, source); err != nil {
			fmt.Fprintf(os.Stderr, "inject: note: could not run in the current document (%v) — it will apply on the next navigation\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "inject: also ran once in the current document")
		}
		return nil
	}

	// One-shot: behaves like `eval` but adds --file support.
	if err := injectOnce(*addrFlag, *tabFlag, *sightmapDirFlag, source); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "inject: ran once in the current document (use --persist to re-inject on every navigation)")
	return nil
}

// injectSource returns the script from --file or a single inline positional arg,
// requiring exactly one of the two.
func injectSource(file string, positionals []string) (string, error) {
	switch {
	case file != "" && len(positionals) > 0:
		return "", fmt.Errorf("inject: pass either --file or an inline <script>, not both")
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("inject: read %s: %w", file, err)
		}
		if len(b) == 0 {
			return "", fmt.Errorf("inject: %s is empty", file)
		}
		return string(b), nil
	case len(positionals) == 1:
		return positionals[0], nil
	case len(positionals) == 0:
		return "", fmt.Errorf("usage: browser inject [--file PATH | <script>] [--persist]")
	default:
		return "", fmt.Errorf("inject: too many arguments — pass one inline <script> or --file PATH")
	}
}

// injectOnce evaluates source a single time in the current document over a
// transient CDP connection (the same path as `eval`).
func injectOnce(addr, tab, sightmapDir, source string) error {
	conn, err := browser.Connect(resolveCDPAddr(addr, sightmapDir), tab)
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), evalTimeout)
	defer cancel()
	_, err = browser.EvalJSON(ctx, conn, source)
	return err
}

// daemonBaseURL resolves the session daemon's HTTP base URL from the session
// file, matching devtoolsGet's resolution.
func daemonBaseURL(sightmapDir string) (string, error) {
	info, err := browser.ReadSessionInfo(sightmapDir)
	if err != nil || info.ServerPort == 0 {
		return "", fmt.Errorf("no running session — start one with 'sightmap browser start'")
	}
	return fmt.Sprintf("http://localhost:%d", info.ServerPort), nil
}

func injectPersist(sightmapDir, source string) (string, error) {
	base, err := daemonBaseURL(sightmapDir)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Post(base+"/inject", "application/javascript", bytes.NewReader([]byte(source)))
	if err != nil {
		return "", fmt.Errorf("reach daemon: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", daemonError(resp.StatusCode, body)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse daemon response: %w", err)
	}
	return out.ID, nil
}

func injectRemove(sightmapDir, id string) error {
	base, err := daemonBaseURL(sightmapDir)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodDelete, base+"/inject?"+url.Values{"id": {id}}.Encode(), nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reach daemon: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("no persisted script with id %q (see 'browser inject --list')", id)
	}
	if resp.StatusCode != http.StatusOK {
		return daemonError(resp.StatusCode, body)
	}
	fmt.Fprintf(os.Stderr, "inject: removed %s\n", id)
	return nil
}

func injectList(sightmapDir string) error {
	base, err := daemonBaseURL(sightmapDir)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(base + "/inject")
	if err != nil {
		return fmt.Errorf("reach daemon: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return daemonError(resp.StatusCode, body)
	}
	var out struct {
		Scripts []browser.PersistentScriptInfo `json:"scripts"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("parse daemon response: %w", err)
	}
	if len(out.Scripts) == 0 {
		fmt.Println("No persisted scripts.")
		return nil
	}
	for _, s := range out.Scripts {
		fmt.Printf("%s  (%d bytes, %d tab(s))\n", s.ID, s.Bytes, s.Tabs)
	}
	return nil
}

func daemonError(status int, body []byte) error {
	if status == http.StatusServiceUnavailable {
		return fmt.Errorf("session is still starting (collector not ready) — retry shortly")
	}
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(status)
	}
	return fmt.Errorf("daemon returned %d: %s", status, msg)
}
