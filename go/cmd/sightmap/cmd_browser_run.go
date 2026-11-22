// Browser `run`: a step script of browser commands, run against one session, stopping at
// the first step that fails. Agents re-check a journey many times while building an app
// (Plumb S-arm iterations: 212 turns, each re-reading a growing context); a journey as one
// script is one call, and a failed `expect` in it says which step broke and what the page
// showed. Steps are the browser subcommands themselves, one per line, so there is nothing
// new to learn: `navigate /inbox --settle`, `keys j`, `expect InboxRow --count 6`.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/browser"
)

// runSteps are the subcommands a step may use: the ones that act on or read the session's
// page. Session management (start, stop, tabs) stays outside a script.
var runSteps = map[string]func([]string) error{
	"navigate": runNavigate, "click": runClick, "fill": runFill, "hover": runHover,
	"keypress": runKeyPress, "keys": runKeys, "scroll": runScroll, "drag": runDrag,
	"wait-for": runWaitFor, "get": runGet, "expect": runExpect, "eval": runEval,
	"screenshot": runScreenshot, "dialog": runDialog,
}

// step is one line of a script, split into words.
type step struct {
	line  int
	text  string
	words []string
}

func runRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir, passed to every step")
	keepGoing := fs.Bool("keep-going", false, "Run every step and report all failures, instead of stopping at the first")
	quiet := fs.Bool("quiet", false, "Print only failures and the summary")
	files, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	if len(files) != 1 {
		return errors.New("usage: browser run FILE|- [--keep-going] [--quiet]\n" +
			"  runs browser commands, one per line, against this session; stops at the first failing step\n" +
			"  a step is a browser subcommand without the 'sightmap browser' prefix:\n" +
			"    navigate /team/WEB/active --settle      (a path is resolved against the current page)\n" +
			"    keys j s 3\n" +
			"    expect 'IssueRow[identifier=WEB-143]' --prop group='status:in progress'\n" +
			"  # comments and blank lines are skipped; quotes and $VARS work as in sh\n" +
			"  steps: " + strings.Join(sortedStepNames(), " "))
	}
	var in io.Reader = os.Stdin
	if files[0] != "-" {
		f, err := os.Open(files[0])
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	steps, err := parseSteps(in)
	if err != nil {
		return err
	}
	if len(steps) == 0 {
		return errors.New("run: no steps")
	}
	// Check every step names a known subcommand before running any, so a typo on step 9
	// doesn't leave the page half-driven.
	for _, s := range steps {
		if _, ok := runSteps[s.words[0]]; !ok {
			return fmt.Errorf("run: line %d: %q is not a step (steps: %s)", s.line, s.words[0], strings.Join(sortedStepNames(), " "))
		}
	}

	common := []string{"--sightmap-dir", *sightmapDirFlag}
	if *addrFlag != "" {
		common = append(common, "--addr", *addrFlag)
	}
	if *tabFlag != "" {
		common = append(common, "--tab", *tabFlag)
	}
	start := time.Now()
	var failed []string
	for i, s := range steps {
		if !*quiet {
			fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", i+1, len(steps), s.text)
		}
		args := s.words[1:]
		if s.words[0] == "navigate" {
			args = absolutize(args, *addrFlag, *sightmapDirFlag, *tabFlag)
		}
		err := runQuietly(*quiet, func() error { return runSteps[s.words[0]](append(args, withoutGiven(common, args)...)) })
		if err == nil {
			continue
		}
		msg := fmt.Sprintf("step %d (line %d) failed: %s\n  %v", i+1, s.line, s.text, err)
		if where := pageWhere(*addrFlag, *sightmapDirFlag, *tabFlag); where != "" {
			msg += "\n  page: " + where
		}
		if !*keepGoing {
			return fmt.Errorf("%s\n%d of %d steps passed", msg, i, len(steps))
		}
		fmt.Fprintln(os.Stderr, msg)
		failed = append(failed, fmt.Sprintf("%d", i+1))
	}
	elapsed := time.Since(start).Round(100 * time.Millisecond)
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d steps failed (steps %s) in %s", len(failed), len(steps), strings.Join(failed, ", "), elapsed)
	}
	fmt.Fprintf(os.Stderr, "all %d steps passed in %s\n", len(steps), elapsed)
	return nil
}

// runQuietly runs f with stdout and stderr sent to /dev/null when quiet, so --quiet leaves
// only failures and the summary.
func runQuietly(quiet bool, f func() error) error {
	if !quiet {
		return f()
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return f()
	}
	defer devnull.Close()
	out, errw := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = devnull, devnull
	defer func() { os.Stdout, os.Stderr = out, errw }()
	return f()
}

// parseSteps reads a script: one step per line, `#` comments and blank lines skipped, a
// trailing backslash continuing a line.
func parseSteps(r io.Reader) ([]step, error) {
	var steps []step
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	line, startLine := 0, 0
	var pending strings.Builder
	for sc.Scan() {
		line++
		t := sc.Text()
		if pending.Len() == 0 {
			startLine = line
		}
		if strings.HasSuffix(t, "\\") {
			pending.WriteString(strings.TrimSuffix(t, "\\"))
			pending.WriteByte(' ')
			continue
		}
		pending.WriteString(t)
		text := strings.TrimSpace(pending.String())
		pending.Reset()
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		words, err := shellWords(text)
		if err != nil {
			return nil, fmt.Errorf("run: line %d: %w", startLine, err)
		}
		if len(words) > 0 && (words[0] == "sightmap" || words[0] == "browser") {
			// Tolerate a pasted "sightmap browser click ...".
			for len(words) > 0 && (words[0] == "sightmap" || words[0] == "browser") {
				words = words[1:]
			}
		}
		if len(words) == 0 {
			continue
		}
		steps = append(steps, step{line: startLine, text: text, words: words})
	}
	return steps, sc.Err()
}

// shellWords splits a line the way sh would for the cases a step needs: whitespace
// separates words, single quotes are literal, double quotes allow \" and $VAR, a backslash
// outside quotes escapes the next character, and $VAR / ${VAR} expand from the environment
// outside single quotes. A `#` starting a word ends the line.
func shellWords(s string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	flush := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	expand := func(i int) int { // s[i] == '$'; returns the index after the variable
		j := i + 1
		if j < len(s) && s[j] == '{' {
			end := strings.IndexByte(s[j:], '}')
			if end < 0 {
				cur.WriteByte('$')
				return i + 1
			}
			cur.WriteString(os.Getenv(s[j+1 : j+end]))
			return j + end + 1
		}
		k := j
		for k < len(s) && (s[k] == '_' || s[k] >= 'A' && s[k] <= 'Z' || s[k] >= 'a' && s[k] <= 'z' || k > j && s[k] >= '0' && s[k] <= '9') {
			k++
		}
		if k == j {
			cur.WriteByte('$')
			return j
		}
		cur.WriteString(os.Getenv(s[j:k]))
		return k
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			flush()
			i++
		case c == '#' && !inWord:
			flush()
			return words, nil
		case c == '\'':
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return nil, errors.New("unclosed single quote")
			}
			cur.WriteString(s[i+1 : i+1+end])
			inWord = true
			i += end + 2
		case c == '"':
			inWord = true
			i++
			for {
				if i >= len(s) {
					return nil, errors.New("unclosed double quote")
				}
				if s[i] == '"' {
					i++
					break
				}
				if s[i] == '\\' && i+1 < len(s) && strings.ContainsRune(`"\$`, rune(s[i+1])) {
					cur.WriteByte(s[i+1])
					i += 2
					continue
				}
				if s[i] == '$' {
					i = expand(i)
					continue
				}
				cur.WriteByte(s[i])
				i++
			}
		case c == '\\' && i+1 < len(s):
			cur.WriteByte(s[i+1])
			inWord = true
			i += 2
		case c == '$':
			inWord = true
			i = expand(i)
		default:
			cur.WriteByte(c)
			inWord = true
			i++
		}
	}
	flush()
	return words, nil
}

// withoutGiven drops the session flags a step already sets itself.
func withoutGiven(common, args []string) []string {
	var out []string
	for i := 0; i+1 < len(common); i += 2 {
		given := false
		for _, a := range args {
			if a == common[i] || strings.HasPrefix(a, common[i]+"=") {
				given = true
			}
		}
		if !given {
			out = append(out, common[i], common[i+1])
		}
	}
	return out
}

// absolutize turns a navigate step's path ("/inbox", "/?reset") into a URL on the
// current page's origin, so a script doesn't repeat the host and port on every line.
func absolutize(args []string, addr, sightmapDir, tab string) []string {
	out := append([]string(nil), args...)
	for i, a := range out {
		if strings.HasPrefix(a, "/") && !strings.HasPrefix(a, "//") {
			if origin := pageOrigin(addr, sightmapDir, tab); origin != "" {
				out[i] = origin + a
			}
			break
		}
		if !strings.HasPrefix(a, "-") {
			break
		}
	}
	return out
}

func pageOrigin(addr, sightmapDir, tab string) string {
	u := pageURL(addr, sightmapDir, tab)
	i := strings.Index(u, "://")
	if i < 0 {
		return ""
	}
	if j := strings.IndexByte(u[i+3:], '/'); j >= 0 {
		return u[:i+3+j]
	}
	return u
}

func pageURL(addr, sightmapDir, tab string) string {
	conn, err := browser.Connect(resolveCDPAddr(addr, sightmapDir), tab)
	if err != nil {
		return ""
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	u, _ := browser.GetURL(ctx, conn)
	return u
}

// pageWhere describes the page after a failed step: its URL and title.
func pageWhere(addr, sightmapDir, tab string) string {
	conn, err := browser.Connect(resolveCDPAddr(addr, sightmapDir), tab)
	if err != nil {
		return ""
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := browser.EvalJSON(ctx, conn, `[location.href, document.title]`)
	if err != nil {
		return ""
	}
	var v [2]string
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return fmt.Sprintf("%s (%q)", v[0], v[1])
}

func sortedStepNames() []string {
	return slices.Sorted(maps.Keys(runSteps))
}
